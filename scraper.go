package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/jinzhu/copier"
)

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	if _, err := os.Stat(path); err == nil {
		return true
	}
	return false
}

func findChromiumBinary() string {
	if custom := os.Getenv("CHROMIUM_BIN"); fileExists(custom) {
		return custom
	}

	candidates := []string{
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
		"/usr/bin/google-chrome",
		"/opt/homebrew/bin/chromium",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	}

	for _, candidate := range candidates {
		if fileExists(candidate) {
			return candidate
		}
	}

	return ""
}

func resolveRelativeURL(base *url.URL, raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if parsed.IsAbs() {
		return parsed.String()
	}
	if base == nil {
		return parsed.String()
	}
	return base.ResolveReference(parsed).String()
}

func extractImageURL(doc *goquery.Document, pageURL string) string {
	base, err := url.Parse(pageURL)
	if err != nil {
		base = nil
	}

	type candidate struct {
		selector string
		attr     string
	}

	selectors := []candidate{
		{selector: "meta[property='og:image']", attr: "content"},
		{selector: "meta[name='twitter:image']", attr: "content"},
		{selector: "link[rel='apple-touch-icon']", attr: "href"},
		{selector: "link[rel='icon']", attr: "href"},
	}

	for _, item := range selectors {
		selection := doc.Find(item.selector)
		if selection.Length() == 0 {
			continue
		}
		found := ""
		selection.EachWithBreak(func(i int, s *goquery.Selection) bool {
			val, exists := s.Attr(item.attr)
			if !exists {
				return true
			}
			val = strings.TrimSpace(val)
			if val == "" {
				return true
			}
			resolved := resolveRelativeURL(base, val)
			if resolved != "" {
				found = resolved
				return false
			}
			return true
		})
		if found != "" {
			return found
		}
	}

	if base != nil {
		return base.ResolveReference(&url.URL{Path: "/favicon.ico"}).String()
	}

	return ""
}

func getRecipe(pageURL string) (Recipe, string, error) {
	launch := launcher.New()
	bin := findChromiumBinary()
	if bin == "" {
		log.Println("No Chromium/Chrome binary found; set CHROMIUM_BIN or install chromium")
		return Recipe{}, "", errors.New("no Chromium/Chrome binary found; set CHROMIUM_BIN or install chromium")
	}
	launch = launch.Bin(bin)

	u, err := launch.Launch()
	if err != nil {
		return Recipe{}, "", fmt.Errorf("launch browser: %w", err)
	}

	browser := rod.New().ControlURL(u)
	if err := browser.Connect(); err != nil {
		return Recipe{}, "", fmt.Errorf("connect browser: %w", err)
	}
	defer browser.MustClose()

	page := browser.MustPage()

	// Set 60-second timeout for page navigation and load
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	err = rod.Try(func() {
		page.MustNavigate(pageURL).MustWaitLoad()
	})
	if err != nil {
		return Recipe{}, "", fmt.Errorf("page navigation timeout: %w", err)
	}

	// Check if context was cancelled due to timeout
	select {
	case <-ctx.Done():
		return Recipe{}, "", fmt.Errorf("browser operation timed out after 60 seconds")
	default:
	}

	content := page.MustHTML()

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		return Recipe{}, "", err
	}

	doc.Find("script, style").Remove()
	cleanedText := strings.TrimSpace(doc.Text())

	prompt := fmt.Sprintf("Extract the recipe details from the provided text, including name/title, description, instructions, ingredients, original_url, featuredImage, and category. Category is either breakfast, dinner or baking. Ensure all steps and ingredients are fully covered. %v", cleanedText)
	system := "You assist in extracting recipe data from web pages and output in json format."
	maxTokens := 16384
	format := "text"
	before := time.Now()
	openaiKey := os.Getenv("OPENAI_KEY")
	ai := NewClient(openaiKey, "gpt-5-mini", format, false)
	response, err := ai.RecipePrompt(prompt, system, maxTokens)
	if err != nil {
		log.Println(err.Error())
		return Recipe{}, "", fmt.Errorf("ai recipe prompt failed: %w", err)
	}
	if response == nil {
		return Recipe{}, "", fmt.Errorf("ai recipe prompt returned nil response")
	}
	spew.Dump(response)

	responseRecipe := Recipe{}
	if err := copier.Copy(&responseRecipe, &response); err != nil {
		return Recipe{}, "", fmt.Errorf("copy ai response: %w", err)
	}
	log.Println("Time to call getting recipe AI: ", time.Since(before).String())
	log.Println(response.Category)

	title := response.Title
	slug := strings.ToLower(strings.ReplaceAll(title, " ", "-"))
	log.Printf("Slug for recipe: %s", slug)

	storedImage := ""
	metadataImage := extractImageURL(doc, pageURL)
	if metadataImage != "" {
		url, err := storeImageFromURL(metadataImage, slug)
		if err != nil {
			log.Printf("Failed to store metadata image: %v", err)
		} else {
			storedImage = url
		}
	}

	if storedImage == "" {
		promptText := fmt.Sprintf("High quality food photography of %s, plated, natural lighting", title)
		imageURL, err := ai.GenerateImage(promptText)
		if err != nil {
			log.Printf("Error generating image: %v", err)
		} else {
			log.Printf("Image URL: %s", imageURL)
			url, err := storeImageFromURL(imageURL, slug)
			if err != nil {
				log.Printf("Failed to store generated image: %v", err)
			} else {
				storedImage = url
			}
		}
	}

	if storedImage != "" {
		responseRecipe.Image = storedImage
	}

	responseRecipe.OriginalURL = pageURL
	return responseRecipe, slug, nil
}

func storeImageFromURL(imageURL, slug string) (string, error) {
	if strings.TrimSpace(imageURL) == "" {
		return "", errors.New("image url is empty")
	}

	// Create HTTP client with 60-second timeout
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	resp, err := client.Get(imageURL)
	if err != nil {
		return "", fmt.Errorf("download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read image: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}

	ext := extensionForContentType(contentType)
	if ext == "" {
		ext = filepath.Ext(imageURL)
	}
	if ext == "" {
		ext = ".jpg"
	}

	key := fmt.Sprintf("images/%s-%d%s", slug, time.Now().Unix(), ext)

	s3Client, err := NewCloudflareS3()
	if err != nil {
		return "", fmt.Errorf("initialize S3 client: %w", err)
	}

	if err := s3Client.UploadImage(key, contentType, data); err != nil {
		return "", fmt.Errorf("upload image: %w", err)
	}

	return fmt.Sprintf("https://cookingimage.bronson.dev/%s", key), nil
}

func extensionForContentType(contentType string) string {
	switch strings.ToLower(contentType) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ""
	}
}

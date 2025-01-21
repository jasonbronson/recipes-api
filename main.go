package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/davecgh/go-spew/spew"
	"github.com/gin-gonic/gin"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/jinzhu/copier"
	"github.com/joho/godotenv"
	"github.com/patrickmn/go-cache"
	ginprometheus "github.com/zsais/go-gin-prometheus"
)

// Add cache as a global variable
var recipeCache *cache.Cache
var recipesCache *cache.Cache

func listRecipes(s3Client *CloudflareS3) ([]Recipe, error) {
	// Try to get from cache first
	if cachedRecipes, found := recipesCache.Get("all_recipes"); found {
		allRecipes, ok := cachedRecipes.([]Recipe)
		if !ok {
			log.Println("Cache data type mismatch")
			return nil, fmt.Errorf("cache data invalid")
		}
		log.Println("Cache hit for all recipes")
		return allRecipes, nil
	}

	// If not in cache, get from S3 bucket
	recipes, err := s3Client.ListRecipes()
	if err != nil {
		return nil, err
	}

	// Create slice to hold all recipe data
	allRecipes := make([]Recipe, 0, len(recipes))

	// Read each recipe file and parse it
	for _, filename := range recipes {
		content, err := s3Client.GetRecipe(filename)
		if err != nil {
			log.Printf("Error reading recipe %s: %v", filename, err)
			continue
		}

		var recipe Recipe
		if err := json.Unmarshal(content, &recipe); err != nil {
			log.Printf("Error parsing recipe JSON %s: %v", filename, err)
			continue
		}

		allRecipes = append(allRecipes, recipe)
	}

	// Store in cache for 1 hour
	recipesCache.Set("all_recipes", allRecipes, 1*time.Hour)

	return allRecipes, nil
}

func main() {
	versionFlag := flag.Bool("version", false, "Print the version of the application")
	flag.Parse()

	// Get version of app
	versionFile, err := os.Open("latest-version.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer versionFile.Close()
	versionBytes, err := io.ReadAll(versionFile)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Version:", string(versionBytes))

	// Check if the version flag is set
	if *versionFlag {
		fmt.Println(string(versionBytes))
		return
	}

	// Initialize both caches
	recipeCache = cache.New(30*24*time.Hour, 1*time.Hour)
	recipesCache = cache.New(1*time.Hour, 10*time.Minute)

	if err := godotenv.Load(); err != nil {
		log.Println("Error loading .env file:", err)
	}

	router := gin.Default()

	// Add CORS middleware
	router.Use(func(c *gin.Context) {
		if c.Request.URL.Path == "/metrics" {
			c.Next()
			return
		}
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	p := ginprometheus.NewPrometheus("gin")
	p.Use(router)

	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "Pong"})
	})

	router.POST("/save-recipe", func(c *gin.Context) {
		var request struct {
			URL string `json:"url" binding:"required"`
		}

		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "URL is required"})
			return
		}

		go func(url string) {
			recipe, filename := getRecipe(url)
			recipe.Link = fmt.Sprintf("/recipes/%s/%s", recipe.Category, strings.Replace(filename, ".json", "", 1))

			jsonContent, err := json.Marshal(recipe)
			if err != nil {
				log.Printf("Failed to marshal recipe: %v", err)
				return
			}

			s3Client, err := NewCloudflareS3()
			if err != nil {
				log.Printf("Failed to initialize S3 client: %v", err)
				return
			}

			if err := s3Client.UploadRecipe(filename, jsonContent); err != nil {
				log.Printf("Failed to upload recipe to R2: %v", err)
				return
			}

			recipeCache.Delete(filename)
			recipesCache.Delete("all_recipes")
		}(request.URL)

		c.JSON(http.StatusOK, gin.H{"message": "Recipe saved successfully"})
	})

	router.GET("/get-recipe/:name", func(c *gin.Context) {
		recipeName := c.Param("name")
		filename := recipeName + ".json"

		// Try to get from cache first
		if cachedContent, found := recipeCache.Get(filename); found {
			log.Println("Cache hit", filename)
			c.Data(http.StatusOK, "application/json", cachedContent.([]byte))
			return
		}

		// If not in cache, get from S3
		s3Client, err := NewCloudflareS3()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to initialize S3 client"})
			return
		}

		content, err := s3Client.GetRecipe(filename)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch recipe"})
			return
		}

		// Store in cache
		recipeCache.Set(filename, content, cache.DefaultExpiration)

		c.Data(http.StatusOK, "application/json", content)
	})

	router.GET("/get-recipes", func(c *gin.Context) {
		// Get the category from query parameters
		category := c.Query("category")

		// Use the new listRecipes function
		s3Client, err := NewCloudflareS3()
		if err != nil {
			log.Println("Error listing recipes:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list recipes"})
			return
		}

		allRecipes, err := listRecipes(s3Client)
		if err != nil {
			log.Println("Error listing recipes:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list recipes"})
			return
		}

		// Filter recipes by category if specified
		if category != "" {
			filteredRecipes := make([]Recipe, 0)
			for _, recipe := range allRecipes {
				if recipe.Category == category {
					filteredRecipes = append(filteredRecipes, recipe)
				}
			}
			c.JSON(http.StatusOK, filteredRecipes)
			return
		}

		c.JSON(http.StatusOK, allRecipes)
	})

	router.GET("/search-recipes", func(c *gin.Context) {
		// Get the search term from query parameters
		searchTerm := c.Query("q")

		// Use the new listRecipes function
		s3Client, err := NewCloudflareS3()
		if err != nil {
			log.Println("Error listing recipes:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list recipes"})
			return
		}

		allRecipes, err := listRecipes(s3Client)
		if err != nil {
			log.Println("Error listing recipes:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list recipes"})
			return
		}

		// Filter recipes by title based on the search term
		filteredRecipes := make([]Recipe, 0)
		for _, recipe := range allRecipes {
			if strings.Contains(strings.ToLower(recipe.Title), strings.ToLower(searchTerm)) {
				filteredRecipes = append(filteredRecipes, recipe)
			}
		}

		c.JSON(http.StatusOK, filteredRecipes)
	})

	router.Run(":8080")
}

func getRecipe(url string) (Recipe, string) {
	binPath := "/usr/bin/chromium"
	if os.Getenv("LOCAL") == "true" {
		binPath = "/opt/homebrew/bin/chromium"
	}
	u := launcher.New().Bin(binPath).MustLaunch()

	// Connect to the browser
	browser := rod.New().ControlURL(u).MustConnect()
	defer browser.MustClose()

	// Create a new page
	page := browser.MustPage()

	// Navigate to the URL
	page.MustNavigate(url)

	// Wait for the page to load fully
	page.MustWaitLoad()

	// Get the page content
	content := page.MustHTML()

	// Load the HTML document
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		log.Fatal(err)
	}

	// Initialize Cloudflare S3 client
	s3Client, err := NewCloudflareS3()
	if err != nil {
		log.Fatal("Failed to initialize S3 client:", err)
	}

	// Extract and print the text from the entire document
	doc.Find("script, style").Remove() // Remove script and style tags
	text := doc.Text()
	cleanedText := strings.TrimSpace(text)

	prompt := fmt.Sprintf("Extract the recipe details from the provided text, including name/title, description, instructions, ingredients, original_url, featuredImage, and category. Category is either breakfast, dinner or baking. Ensure all steps and ingredients are fully covered. %v", cleanedText)
	system := "You assist in extracting recipe data from web pages and output in json format."
	maxTokens := 16384
	format := "text"
	before := time.Now()
	openaiKey := os.Getenv("OPENAI_KEY")
	ai := NewClient(openaiKey, "gpt-4o-mini-2024-07-18", format, false)
	response, err := ai.RecipePrompt(prompt, system, maxTokens)
	if err != nil {
		log.Println(err.Error())
	}
	spew.Dump(response)

	responseRecipe := Recipe{}
	copier.Copy(&responseRecipe, &response)
	after := time.Now()
	diff := after.Sub(before)
	log.Println("Time to call getting recipe AI: ", diff.String())
	log.Println(response.Category)

	title := response.Title

	// Create filename for S3 upload
	filename := strings.ToLower(strings.ReplaceAll(title, " ", "-")) + ".json"
	log.Printf("Filename for json: %s", filename)
	fixedTitle := strings.ToLower(strings.ReplaceAll(title, " ", "-"))
	imageFilename := fmt.Sprintf("images/%s.jpg", fixedTitle)
	tempImageFilename := fmt.Sprintf("autogenerated_%v_%v.jpg", time.Now().Unix(), fixedTitle)

	r, err := ai.GenerateEnhancedFoodPrompt(title, 2000)
	if err != nil {
		log.Fatal(err)
	}

	p := r.EnhancedPrompt[:999]

	// Generate image using AI
	imageURL, err := ai.GenerateImage(p)
	if err != nil {
		log.Printf("Error generating image: %v", err)
	} else {
		log.Printf("Image URL: %s", imageURL)
		// Download image
		err := DownloadImage(imageURL, tempImageFilename)
		if err == nil {
			log.Printf("Image downloaded to %s", tempImageFilename)
			image, err := os.ReadFile(tempImageFilename)
			if err != nil {
				log.Println("Error reading image:", err)
			} else {
				// Upload the image to S3
				if err := s3Client.UploadImage(imageFilename, "image/jpeg", image); err != nil {
					log.Fatal("Error uploading image to S3:", err)
				}
				responseRecipe.Image = fmt.Sprintf("https://cookingimage.bronson.dev/%s", imageFilename)
				log.Printf("image uploaded to s3 %s", imageFilename)
			}
		}
	}
	responseRecipe.OriginalURL = url
	return responseRecipe, filename
}

func DownloadImage(url, filename string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	return nil
}

type Recipe struct {
	Category     string   `json:"category"`
	CookTime     int      `json:"cookTime"`
	Date         string   `json:"date"`
	Image        string   `json:"image"`
	Ingredients  []string `json:"ingredients"`
	Instructions []string `json:"instructions"`
	PrepTime     int      `json:"prepTime"`
	Servings     int      `json:"servings"`
	Title        string   `json:"title"`
	TotalTime    int      `json:"totalTime"`
	Link         string   `json:"link"`
	OriginalURL  string   `json:"originalURL"`
}

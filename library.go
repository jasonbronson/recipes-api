package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/davecgh/go-spew/spew"
)

func matchImage(title string, image byte) bool {

	prompt := fmt.Sprintf("Does this image match this title '%s'?", title)
	system := "You assist in matching food images to recipe titles and output an answer only with 'yes' or 'no'."
	openaiKey := os.Getenv("OPENAI_KEY")
	maxTokens := 16384
	format := "text"
	ai := NewClient(openaiKey, "gpt-4o-mini-2024-07-18", format, false)
	response, err := ai.Prompt(prompt, system, maxTokens)
	if err != nil {
		log.Println(err.Error())
	}
	spew.Dump(response)

	return false

}

func downloadImages(images []string) []byte {
	imageList := make([]byte, 0)

	// Download the images
	for _, imageContent := range images {
		resp, err := http.Get(imageContent)
		if err != nil {
			log.Println("Error downloading image:", err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Printf("Failed to download image: %s \n", resp.Status)
			continue
		}

		// Read the image data
		imageData, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatal("Error reading image data:", err)
			continue
		}
		imageList = append(imageList, imageData...)
	}

	return imageList
}

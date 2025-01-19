package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

func matchImage(title string, imageData []byte) bool {

	openaiKey := os.Getenv("OPENAI_KEY")
	format := "text"
	ai := NewClient(openaiKey, "gpt-4o", format, false)

	// Encode the image data to base64
	imageBase64 := base64.StdEncoding.EncodeToString(imageData)
	promptWithImage := fmt.Sprintf(" Image Data (base64): %s ", imageBase64)
	response, err := ai.ValidateImage(title, promptWithImage)
	if err != nil {
		log.Println(err.Error())
	}

	if response {
		log.Println(imageBase64)
		log.Println("Image matches:", title)
	}

	return response
}

func downloadImages(images []string) [][]byte {
	imageList := make([][]byte, 0)

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
		imageList = append(imageList, imageData)
	}

	return imageList
}

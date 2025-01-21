package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/davecgh/go-spew/spew"
	"github.com/sashabaranov/go-openai"
)

const defaultEngine = "gpt-4o-mini"

type Client struct {
	client *openai.Client
	engine string
	debug  bool
	format string
	schema map[string]interface{}
}

func NewClient(apiKey, engine, format string, debug bool) *Client {
	if engine == "" {
		engine = defaultEngine
	}

	data, err := os.ReadFile("schema.json")
	if err != nil {
		log.Fatal(err)
	}
	var schema map[string]interface{}
	if err := json.Unmarshal(data, &schema); err != nil {
		log.Fatal(err)
	}

	return &Client{
		client: openai.NewClient(apiKey),
		engine: engine,
		debug:  debug,
		format: format,
		schema: schema,
	}
}

func (c *Client) RecipePrompt(prompt, systemPrompt string, maxTokens int) (*Response, error) {
	ctx := context.Background()

	schemaJSON, err := json.Marshal(c.schema["schema"])
	if err != nil {
		return nil, fmt.Errorf("failed to marshal schema: %w", err)
	}

	req := openai.ChatCompletionRequest{
		Model: c.engine,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemPrompt,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		MaxTokens:   maxTokens,
		Temperature: 0,
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONSchema,
			JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
				Name:   "recipe_response",
				Schema: json.RawMessage(schemaJSON),
				Strict: true,
			},
		},
	}

	if c.debug {
		log.Printf("Request: %+v\n", req)
	}

	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, err
	}

	if c.debug {
		log.Printf("Response: %+v\n", resp)
	}

	var response Response
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &response); err != nil {
		return nil, err
	}

	response.ID = resp.ID
	response.Object = resp.Object
	response.Created = resp.Created
	response.Model = resp.Model
	response.SystemFingerprint = resp.SystemFingerprint
	response.Usage = Usage{
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
	}

	return &response, nil
}

func (c *Client) ValidateImage(title, image string) (bool, error) {
	ctx := context.Background()

	// Define the JSON schema for enforcing a boolean response with additionalProperties set to false
	schemaJSON := `{
		"type": "object",
		"properties": {
			"matches": {
				"type": "boolean"
			}
		},
		"required": ["matches"],
		"additionalProperties": false
	}`

	req := openai.ChatCompletionRequest{
		Model: c.engine,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: "You are an assistant validating if an image title matches its content. Respond only with a JSON object containing a boolean field 'matches'.",
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: fmt.Sprintf(`{"title": %q, "image": %q}`, title, image),
			},
		},
		MaxTokens:   16000,
		Temperature: 0,
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONSchema,
			JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
				Name:   "image_validation",
				Schema: json.RawMessage(schemaJSON),
				Strict: true,
			},
		},
	}

	// Debug logging
	//if c.debug {
	//	log.Printf("Request: %+v\n", req)
	//}

	// Send the request
	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return false, err
	}

	if c.debug {
		log.Printf("Response: %+v\n", resp)
	}

	// Parse the response into a struct
	var result struct {
		Matches bool `json:"matches"`
	}

	if len(resp.Choices) > 0 {
		if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &result); err != nil {
			return false, fmt.Errorf("failed to parse response: %w", err)
		}
		spew.Dump(resp.Choices)
	}

	return result.Matches, nil
}

func (c *Client) GenerateImage(prompt string) (string, error) {
	ctx := context.Background()

	req := openai.ImageRequest{
		Prompt:         prompt,
		Size:           openai.CreateImageSize1024x1024,
		N:              1,
		ResponseFormat: openai.CreateImageResponseFormatURL,
	}

	resp, err := c.client.CreateImage(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to generate image: %w", err)
	}

	if len(resp.Data) == 0 {
		return "", fmt.Errorf("no image URL returned")
	}

	return resp.Data[0].URL, nil
}

func (c *Client) GenerateEnhancedFoodPrompt(foodItem string, maxTokens int) (*BasicResponse, error) {
	ctx := context.Background()

	// Define the system prompt for generating detailed and visually rich descriptions
	systemPrompt := "You are a food stylist and photographer specializing in creating vivid, visually appealing descriptions for food items. Your job is to generate enhanced and detailed prompts suitable for creating high-quality images."

	// User prompt for the specific food item and context
	userPrompt := fmt.Sprintf("Create a visually appealing description for '%s'. Include details about texture, color, lighting, setting, and arrangement. Max characters can not exceed 1000 chars.", foodItem)

	req := openai.ChatCompletionRequest{
		Model: c.engine,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemPrompt,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: userPrompt,
			},
		},
		MaxTokens:   maxTokens,
		Temperature: 0.9,
	}

	if c.debug {
		log.Printf("Request: %+v\n", req)
	}

	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate enhanced prompt: %w", err)
	}

	if c.debug {
		log.Printf("foodResponse: %+v\n", resp)
	}

	var basicResponse BasicResponse
	basicResponse.ID = resp.ID
	basicResponse.Object = resp.Object
	basicResponse.Created = resp.Created
	basicResponse.Model = resp.Model
	basicResponse.Usage = Usage{
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
	}
	basicResponse.EnhancedPrompt = resp.Choices[0].Message.Content

	return &basicResponse, nil
}

type BasicResponse struct {
	ID                string `json:"id"`                 // ID of the response
	Object            string `json:"object"`             // Object type (e.g., "text_completion")
	Created           int64  `json:"created"`            // Timestamp of creation
	Model             string `json:"model"`              // Model used for the request
	SystemFingerprint string `json:"system_fingerprint"` // Optional system fingerprint for debugging
	Usage             Usage  `json:"usage"`              // Token usage details
	EnhancedPrompt    string `json:"enhanced_prompt"`    // The generated detailed prompt
}

type ImageValidationRequest struct {
	Title string `json:"title"`
	Image string `json:"image"`
}

type Response struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"`
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	SystemFingerprint string   `json:"system_fingerprint"`
	Choices           []Choice `json:"choices"`
	Usage             Usage    `json:"usage"`
	Title             string   `json:"title"`
	Date              string   `json:"date"`
	Image             string   `json:"image"`
	PrepTime          int      `json:"prepTime"`
	CookTime          int      `json:"cookTime"`
	TotalTime         int      `json:"totalTime"`
	Servings          int      `json:"servings"`
	Category          string   `json:"category"`
	Ingredients       []string `json:"ingredients"`
	Instructions      []string `json:"instructions"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	Logprobs     *string `json:"logprobs"`
	FinishReason string  `json:"finish_reason"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/penguinpowernz/aichat/config"
)

type OpenAIClient struct {
	config     *config.Config
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
}

func NewOpenAIClient(cfg *config.Config) (*OpenAIClient, error) {
	// API key optional for local models (Ollama)
	if cfg.APIKey == "" && cfg.Provider != "ollama" && cfg.Provider != "custom" {
		return nil, fmt.Errorf("API key is required for provider: %s", cfg.Provider)
	}

	return &OpenAIClient{
		config:     cfg,
		httpClient: &http.Client{},
		baseURL:    strings.TrimSuffix(cfg.BaseURL, "/"),
		apiKey:     cfg.APIKey,
		model:      cfg.Model,
	}, nil
}

func (c *OpenAIClient) SendMessage(ctx context.Context, messages []Message) (*Response, error) {
	// Prepend system prompt if it exists
	allMessages := c.prepareMessages(messages)

	reqBody := openAIRequest{
		Model:       c.model,
		Messages:    allMessages,
		MaxTokens:   c.config.MaxTokens,
		Temperature: c.config.Temperature,
		Stream:      false,
	}

	respBody, err := c.makeRequest(ctx, reqBody)
	if err != nil {
		return nil, err
	}

	return &Response{
		Content:      respBody.Choices[0].Message.Content,
		TokensUsed:   respBody.Usage.TotalTokens,
		FinishReason: respBody.Choices[0].FinishReason,
	}, nil
}

func (c *OpenAIClient) StreamMessage(ctx context.Context, messages []Message) (<-chan string, error) {
	// Prepend system prompt if it exists
	allMessages := c.prepareMessages(messages)

	reqBody := openAIRequest{
		Model:       c.model,
		Messages:    allMessages,
		MaxTokens:   c.config.MaxTokens,
		Temperature: c.config.Temperature,
		Stream:      true,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	streamChan := make(chan string, 10)

	go func() {
		defer close(streamChan)
		defer resp.Body.Close()

		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err != io.EOF {
					if c.config.Verbose {
						fmt.Printf("Stream read error: %v\n", err)
					}
				}
				return
			}

			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}

			// SSE format: "data: {...}"
			if !bytes.HasPrefix(line, []byte("data: ")) {
				continue
			}

			data := bytes.TrimPrefix(line, []byte("data: "))

			// Check for end of stream
			if string(data) == "[DONE]" {
				return
			}

			var chunk openAIStreamChunk
			if err := json.Unmarshal(data, &chunk); err != nil {
				if c.config.Verbose {
					fmt.Printf("Failed to parse chunk: %v\n", err)
				}
				continue
			}

			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				select {
				case streamChan <- chunk.Choices[0].Delta.Content:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return streamChan, nil
}

func (c *OpenAIClient) GetModelInfo() ModelInfo {
	return ModelInfo{
		Name:              c.model,
		Provider:          c.config.Provider,
		MaxTokens:         c.config.MaxTokens,
		SupportsStreaming: true,
	}
}

func (c *OpenAIClient) makeRequest(ctx context.Context, reqBody openAIRequest) (*openAIResponse, error) {
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

func convertToOpenAIMessages(messages []Message) []openAIMessage {
	result := make([]openAIMessage, len(messages))
	for i, msg := range messages {
		result[i] = openAIMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}
	return result
}

// prepareMessages prepends system prompt if it exists
func (c *OpenAIClient) prepareMessages(messages []Message) []openAIMessage {
	var allMessages []Message

	// Add system prompt if it exists
	if c.config.SystemPrompt != "" {
		allMessages = append(allMessages, Message{
			Role:    "system",
			Content: c.config.SystemPrompt,
		})
	}

	allMessages = append(allMessages, messages...)
	return convertToOpenAIMessages(allMessages)
}

// OpenAI API types
type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	Stream      bool            `json:"stream"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []openAIChoice `json:"choices"`
	Usage   openAIUsage    `json:"usage"`
}

type openAIChoice struct {
	Index        int           `json:"index"`
	Message      openAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openAIStreamChunk struct {
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Model   string               `json:"model"`
	Choices []openAIStreamChoice `json:"choices"`
}

type openAIStreamChoice struct {
	Index        int               `json:"index"`
	Delta        openAIStreamDelta `json:"delta"`
	FinishReason string            `json:"finish_reason"`
}

type openAIStreamDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

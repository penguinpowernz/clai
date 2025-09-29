package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/penguinpowernz/aichat/config"
)

type AnthropicClient struct {
	config     *config.Config
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
}

func NewAnthropicClient(cfg *config.Config) (*AnthropicClient, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("anthropic API key is required")
	}

	return &AnthropicClient{
		config:     cfg,
		httpClient: &http.Client{},
		baseURL:    cfg.BaseURL,
		apiKey:     cfg.APIKey,
		model:      cfg.Model,
	}, nil
}

func (c *AnthropicClient) SendMessage(ctx context.Context, messages []Message) (*Response, error) {
	reqBody := anthropicRequest{
		Model:     c.model,
		Messages:  convertToAnthropicMessages(messages),
		MaxTokens: c.config.MaxTokens,
		Stream:    false,
		System:    c.config.SystemPrompt, // Add system prompt
	}

	respBody, err := c.makeRequest(ctx, reqBody)
	if err != nil {
		return nil, err
	}

	return &Response{
		Content:      respBody.Content[0].Text,
		TokensUsed:   respBody.Usage.InputTokens + respBody.Usage.OutputTokens,
		FinishReason: respBody.StopReason,
	}, nil
}

func (c *AnthropicClient) StreamMessage(ctx context.Context, messages []Message) (<-chan string, error) {
	reqBody := anthropicRequest{
		Model:     c.model,
		Messages:  convertToAnthropicMessages(messages),
		MaxTokens: c.config.MaxTokens,
		Stream:    true,
		System:    c.config.SystemPrompt, // Add system prompt
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

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

		decoder := json.NewDecoder(resp.Body)
		for {
			var event anthropicStreamEvent
			if err := decoder.Decode(&event); err != nil {
				if err != io.EOF {
					// Send error through channel if needed
					if c.config.Verbose {
						fmt.Printf("Stream decode error: %v\n", err)
					}
				}
				return
			}

			if event.Type == "content_block_delta" && event.Delta.Type == "text_delta" {
				select {
				case streamChan <- event.Delta.Text:
				case <-ctx.Done():
					return
				}
			}

			if event.Type == "message_stop" {
				return
			}
		}
	}()

	return streamChan, nil
}

func (c *AnthropicClient) GetModelInfo() ModelInfo {
	return ModelInfo{
		Name:              c.model,
		Provider:          "anthropic",
		MaxTokens:         200000, // Claude's context window
		SupportsStreaming: true,
	}
}

func (c *AnthropicClient) makeRequest(ctx context.Context, reqBody anthropicRequest) (*anthropicResponse, error) {
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

func convertToAnthropicMessages(messages []Message) []anthropicMessage {
	result := make([]anthropicMessage, 0, len(messages))
	for _, msg := range messages {
		if msg.Role != "system" { // Anthropic handles system messages differently
			result = append(result, anthropicMessage{
				Role: msg.Role,
				Content: []anthropicContent{
					{Type: "text", Text: msg.Content},
				},
			})
		}
	}
	return result
}

// Anthropic API types
type anthropicRequest struct {
	Model     string             `json:"model"`
	Messages  []anthropicMessage `json:"messages"`
	MaxTokens int                `json:"max_tokens"`
	Stream    bool               `json:"stream"`
	System    string             `json:"system,omitempty"` // System prompt
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
}

type anthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicResponse struct {
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	Role       string             `json:"role"`
	Content    []anthropicContent `json:"content"`
	StopReason string             `json:"stop_reason"`
	Usage      anthropicUsage     `json:"usage"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicStreamEvent struct {
	Type  string               `json:"type"`
	Delta anthropicStreamDelta `json:"delta,omitempty"`
}

type anthropicStreamDelta struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// internal/ai/types.go
package ai

import "context"

// Message represents a single message in the conversation
type Message struct {
	Role    string `json:"role"`    // "user", "assistant", or "system"
	Content string `json:"content"` // The message content
}

// Response represents an AI response
type Response struct {
	Content      string
	TokensUsed   int
	FinishReason string // "stop", "length", "content_filter", etc.
}

// AIProvider is the interface that all AI clients must implement
type AIProvider interface {
	// SendMessage sends a message and waits for complete response
	SendMessage(ctx context.Context, messages []Message) (*Response, error)

	// StreamMessage sends a message and streams the response
	StreamMessage(ctx context.Context, messages []Message) (<-chan string, error)

	// GetModelInfo returns information about the current model
	GetModelInfo() ModelInfo
}

// ModelInfo contains metadata about the AI model
type ModelInfo struct {
	Name              string
	Provider          string
	MaxTokens         int
	SupportsStreaming bool
}

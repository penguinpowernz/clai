// internal/ai/types.go
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/penguinpowernz/clai/internal/tools"
)

// Message represents a single message in the conversation
type Message struct {
	Role       string   `json:"role"`                   // "user", "assistant", or "system"
	Content    string   `json:"content"`                // The message content
	ToolCallID string   `json:"tool_call_id,omitempty"` // For tool result messages
	ToolCall   *ToolUse `json:"tool_call,omitempty"`    // When assistant uses a tool
}

// ToolUse represents a tool invocation by the AI
type ToolUse struct {
	ID    string      `json:"id"`
	Name  string      `json:"name"`
	Input interface{} `json:"input"`
}

// Response represents an AI response
type Response struct {
	Content      string
	TokensUsed   int
	FinishReason string    // "stop", "length", "content_filter", "tool_use", etc.
	ToolUses     []ToolUse // Tools the AI wants to use
}

// Provider is the interface that all AI clients must implement
type Provider interface {
	// SendMessage sends a message and waits for complete response
	SendMessage(ctx context.Context, messages []Message) (*Response, error)

	// StreamMessage sends a message and streams the response
	StreamMessage(ctx context.Context, messages []Message) (<-chan MessageChunk, error)

	// GetModelInfo returns information about the current model
	GetModelInfo() ModelInfo

	// SetTools sets the tools available to the AI
	SetTools(tools []tools.Tool)
}

// ModelInfo contains metadata about the AI model
type ModelInfo struct {
	Name              string
	Provider          string
	MaxTokens         int
	SupportsStreaming bool
}

type MessageChunk struct {
	typ      string
	Content  string
	ToolCall *ToolCall
}

func (m MessageChunk) Type() string {
	return m.typ
}

func (m MessageChunk) String() string {
	if m.IsToolCall() {
		return fmt.Sprintf("The AI wishes to call the tool: %s with args: %+v", m.ToolCall.Name)
	}

	return m.Content
}

func (m MessageChunk) IsToolCall() bool {
	return m.ToolCall != nil
}

type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

const (
	ChunkMessage  = "message"
	ChunkToolCall = "tool_call"
	ChunkThink    = "think"
)

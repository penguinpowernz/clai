package tools

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/penguinpowernz/clai/config"
)

var DefaultTools = []Tool{}

// Tool is one entry in the `tools` array that you send to /chat/completions.
type Tool struct {
	Type     string          `json:"type"` // "function" (currently the only supported value)
	Function *FunctionSchema `json:"function,omitempty"`
	exec     toolExecutor
}

type Tools []Tool

func (ts Tools) find(name string) (*Tool, bool) {
	for _, t := range ts {
		if t.Function.Name == name {
			return &t, true
		}
	}
	return nil, false
}

type FunctionSchema struct {
	Name        string      `json:"name"`        // e.g. "book_flight"
	Description string      `json:"description"` // human‑readable docstring
	Parameters  *JSONSchema `json:"parameters,omitempty"`
}

type JSONSchema struct {
	Type       string              `json:"type"` // usually "object"
	Properties map[string]Property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
	// You can add more JSON‑Schema fields here if you need them
	// e.g. `Enum`, `Format`, `Items`, etc.
}

type Property struct {
	Type        string `json:"type"` // usually "string"
	Description string `json:"description"`
}

// ToolUse represents when the AI wants to use a tool
type ToolUse struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// ToolResult represents the result of executing a tool
type ToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

// GetAvailableTools returns all tools the AI can use
func GetAvailableTools() []Tool {
	return DefaultTools
}

type toolExecutor func(cfg config.Config, toolUse json.RawMessage, workingDir string) (string, error)

// ExecuteTool executes a tool and returns the result
func ExecuteTool(cfg *config.Config, toolCall ToolUse, workingDir string) ToolResult {
	result := ToolResult{
		ToolUseID: toolCall.ID,
	}

	var tool toolExecutor

	x, found := Tools(DefaultTools).find(toolCall.Name)
	if !found {
		result.Content = fmt.Sprintf("Unknown tool: %s", toolCall.Name)
		result.IsError = true
		return result
	}
	tool = x.exec

	if tool == nil {
		result.Content = fmt.Sprintf("cannot execute tool: %s", toolCall.Name)
		result.IsError = true
		return result
	}

	content, err := tool(*cfg, toolCall.Input, workingDir)
	if err != nil {
		result.Content = fmt.Sprintf("Error: %v", err)
		result.IsError = true
	} else {
		result.Content = content
	}

	log.Println("[tools] tool output:", result.Content)

	return result
}

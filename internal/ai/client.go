package ai

import (
	"fmt"

	"github.com/penguinpowernz/clai/config"
)

// NewClient creates a new AI client based on the provider configuration
func NewClient(cfg *config.Config) (Provider, error) {
	switch cfg.Provider {
	case "anthropic":
		return NewAnthropicClient(cfg)
	case "openai":
		return NewOpenAIClient(cfg)
	case "ollama":
		// Ollama uses OpenAI-compatible API
		return NewOpenAIClient(cfg)
	case "custom":
		// Custom providers assumed to be OpenAI-compatible
		return NewOpenAIClient(cfg)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}

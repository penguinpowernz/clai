package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

// Config holds all application configuration
type Config struct {
	// AI Provider settings
	Provider string `mapstructure:"provider"` // "anthropic", "openai", "ollama", "custom"
	Model    string `mapstructure:"model"`
	APIKey   string `mapstructure:"api_key"`
	BaseURL  string `mapstructure:"base_url"` // Custom API endpoint (for ollama, local models, etc.)

	// Behavior settings
	AutoApply    bool    `mapstructure:"auto_apply"`    // Auto-apply code changes
	AutoCommit   bool    `mapstructure:"auto_commit"`   // Auto-commit changes
	Stream       bool    `mapstructure:"stream"`        // Stream responses
	ContextFiles int     `mapstructure:"context_files"` // Max files to include
	MaxTokens    int     `mapstructure:"max_tokens"`    // Max tokens per request
	Temperature  float64 `mapstructure:"temperature"`   // Model temperature

	// Git settings
	UseGit       bool `mapstructure:"use_git"`        // Enable git integration
	GitAutoStage bool `mapstructure:"git_auto_stage"` // Auto-stage changes

	// UI settings
	NoColor bool   `mapstructure:"no_color"` // Disable colored output
	Verbose bool   `mapstructure:"verbose"`  // Verbose logging
	Editor  string `mapstructure:"editor"`   // Preferred editor

	// File handling
	ExcludePatterns []string `mapstructure:"exclude_patterns"` // Files/dirs to exclude
	IncludeHidden   bool     `mapstructure:"include_hidden"`   // Include hidden files
	MaxFileSize     int64    `mapstructure:"max_file_size"`    // Max file size in bytes

	// Session settings
	SessionDir     string `mapstructure:"session_dir"`      // Where to store sessions
	SaveHistory    bool   `mapstructure:"save_history"`     // Save conversation history
	MaxHistorySize int    `mapstructure:"max_history_size"` // Max messages to keep
}

// Load loads the configuration from file and environment
func Load() (*Config, error) {
	cfg := &Config{
		// Defaults
		Provider:     "anthropic",
		Model:        "claude-sonnet-4-20250514",
		APIKey:       "",
		BaseURL:      "", // Will be set based on provider if empty
		AutoApply:    false,
		AutoCommit:   false,
		Stream:       true,
		ContextFiles: 5,
		MaxTokens:    4096,
		Temperature:  0.7,
		UseGit:       true,
		GitAutoStage: false,
		NoColor:      false,
		Verbose:      false,
		Editor:       getDefaultEditor(),
		ExcludePatterns: []string{
			"node_modules/",
			".git/",
			"*.log",
			"*.tmp",
			"vendor/",
			"dist/",
			"build/",
		},
		IncludeHidden:  false,
		MaxFileSize:    1024 * 1024, // 1MB
		SessionDir:     ".aichat",
		SaveHistory:    true,
		MaxHistorySize: 100,
	}

	// Unmarshal viper config into struct
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Load API key from environment if not in config
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("ANTHROPIC_API_KEY")
		if cfg.APIKey == "" && cfg.Provider == "anthropic" {
			cfg.APIKey = os.Getenv("CLAUDE_API_KEY")
		}
		if cfg.APIKey == "" && cfg.Provider == "openai" {
			cfg.APIKey = os.Getenv("OPENAI_API_KEY")
		}
	}

	// Set default base URLs if not specified
	if cfg.BaseURL == "" {
		cfg.BaseURL = getDefaultBaseURL(cfg.Provider)
	}

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// API key not required for local models like Ollama
	if c.APIKey == "" && c.Provider != "ollama" && c.Provider != "custom" {
		return fmt.Errorf("API key not found. Set ANTHROPIC_API_KEY or OPENAI_API_KEY environment variable")
	}

	if c.Provider != "anthropic" && c.Provider != "openai" && c.Provider != "ollama" && c.Provider != "custom" {
		return fmt.Errorf("invalid provider: %s (must be 'anthropic', 'openai', 'ollama', or 'custom')", c.Provider)
	}

	if c.Model == "" {
		return fmt.Errorf("model not specified")
	}

	if c.BaseURL == "" {
		return fmt.Errorf("base_url not specified")
	}

	if c.ContextFiles < 0 {
		return fmt.Errorf("context_files must be >= 0")
	}

	if c.MaxTokens < 1 {
		return fmt.Errorf("max_tokens must be > 0")
	}

	return nil
}

// Initialize creates a default config file
func Initialize() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configPath := home + "/.aichat.yaml"

	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("config file already exists at %s", configPath)
	}

	// Create default config
	defaultConfig := `# AI Code Assistant Configuration

# AI Provider (anthropic, openai, ollama, or custom)
provider: anthropic
model: claude-sonnet-4-20250514

# Base URL for API endpoint (optional - defaults set per provider)
# For Ollama: http://localhost:11434/v1
# For custom OpenAI-compatible APIs
# base_url: http://localhost:11434/v1

# API Key (or use environment variable)
# Not required for Ollama or local models
# api_key: your-api-key-here

# Behavior
auto_apply: false      # Automatically apply code changes
auto_commit: false     # Automatically commit changes
stream: true           # Stream responses
context_files: 5       # Max files to include in context
max_tokens: 4096       # Max tokens per request
temperature: 0.7       # Model temperature (0.0 - 1.0)

# Git
use_git: true          # Enable git integration
git_auto_stage: false  # Auto-stage changes

# UI
no_color: false        # Disable colored output
verbose: false         # Verbose logging
editor: vim            # Preferred editor

# File handling
exclude_patterns:
  - node_modules/
  - .git/
  - "*.log"
  - "*.tmp"
  - vendor/
  - dist/
  - build/

include_hidden: false  # Include hidden files
max_file_size: 1048576 # Max file size in bytes (1MB)

# Session
session_dir: .aichat   # Where to store session data
save_history: true     # Save conversation history
max_history_size: 100  # Max messages to keep in history
`

	if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Printf("Created config file at %s\n", configPath)
	fmt.Println("Don't forget to set your API key!")

	return nil
}

// Display shows the current configuration
func Display(cfg *Config) error {
	fmt.Println("Current Configuration:")
	fmt.Printf("  Provider: %s\n", cfg.Provider)
	fmt.Printf("  Model: %s\n", cfg.Model)
	fmt.Printf("  Base URL: %s\n", cfg.BaseURL)
	if cfg.APIKey != "" {
		fmt.Printf("  API Key: %s\n", maskAPIKey(cfg.APIKey))
	} else {
		fmt.Printf("  API Key: (not set)\n")
	}
	fmt.Printf("  Auto Apply: %t\n", cfg.AutoApply)
	fmt.Printf("  Auto Commit: %t\n", cfg.AutoCommit)
	fmt.Printf("  Stream: %t\n", cfg.Stream)
	fmt.Printf("  Context Files: %d\n", cfg.ContextFiles)
	fmt.Printf("  Use Git: %t\n", cfg.UseGit)
	return nil
}

// Set updates a configuration value
func Set(key, value string) error {
	viper.Set(key, value)
	return viper.WriteConfig()
}

func getDefaultEditor() string {
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}
	return "vim"
}

func maskAPIKey(key string) string {
	if len(key) < 8 {
		return "****"
	}
	return key[:4] + "..." + key[len(key)-4:]
}

// getDefaultBaseURL returns the default base URL for a provider
func getDefaultBaseURL(provider string) string {
	switch provider {
	case "anthropic":
		return "https://api.anthropic.com"
	case "openai":
		return "https://api.openai.com/v1"
	case "ollama":
		return "http://localhost:11434/v1"
	case "custom":
		return "" // Must be set by user
	default:
		return ""
	}
}

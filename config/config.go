package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/spf13/viper"
)

// Config holds all application configuration
type Config struct {
	// AI Provider settings
	Provider string `mapstructure:"provider"` // "openai", "ollama", "custom"
	Model    string `mapstructure:"model"`
	APIKey   string `mapstructure:"api_key"`
	BaseURL  string `mapstructure:"base_url"` // Custom API endpoint (for ollama, local models, etc.)

	// Prompt settings
	SystemPrompt string `mapstructure:"system_prompt"` // Custom system prompt

	// Behavior settings
	AutoApply    bool    `mapstructure:"auto_apply"`    // Auto-apply code changes
	ShowThinking bool    `mapstructure:"show_thinking"` // Show thinking indicator
	ContextFiles int     `mapstructure:"context_files"` // Max files to include
	MaxTokens    int     `mapstructure:"max_tokens"`    // Max tokens per request
	Temperature  float64 `mapstructure:"temperature"`   // Model temperature

	// UI settings
	Verbose bool   `mapstructure:"verbose"` // Verbose logging
	Editor  string `mapstructure:"editor"`  // Preferred editor

	// File handling
	ExcludePatterns []string `mapstructure:"exclude_patterns"` // Files/dirs to exclude
	IncludeHidden   bool     `mapstructure:"include_hidde n"`  // Include hidden files
	MaxFileSize     int64    `mapstructure:"max_file_size"`    // Max file size in bytes
	PermittedTools  []string `mapstructure:"permitted_tools"`  // Tools to allow

	// Session settings
	SessionDir     string `mapstructure:"session_dir"`      // Where to store sessions
	SaveHistory    bool   `mapstructure:"save_history"`     // Save conversation history
	MaxHistorySize int    `mapstructure:"max_history_size"` // Max messages to keep

	PluginDir string `mapstructure:"plugin_dir"`
}

func Default() *Config {
	return &Config{
		// Defaults
		Provider:     "ollama",
		Model:        "gpt-oss:latest",
		APIKey:       "",
		BaseURL:      "", // Will be set based on provider if empty
		SystemPrompt: getDefaultSystemPrompt(),
		AutoApply:    false,
		ContextFiles: 5,
		MaxTokens:    4096,
		Temperature:  0.7,
		Verbose:      false,
		ShowThinking: true,
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
		SessionDir:     "~/.clai",
		SaveHistory:    true,
		MaxHistorySize: 100,
		PermittedTools: []string{"list_files", "search_file"},
		PluginDir:      "~/.clai/plugins",
	}
}

// Load loads the configuration from file and environment
func Load() (*Config, error) {
	cfg := Default()

	// Unmarshal viper config into struct
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Replace ~ with home directory
	cfg.SessionDir = strings.Replace(cfg.SessionDir, "~", os.Getenv("HOME"), 1)

	// Load API key from environment if not in config
	if cfg.APIKey == "" {
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
		return fmt.Errorf("API key not found. Set OPENAI_API_KEY environment variable")
	}

	if c.Provider != "openai" && c.Provider != "ollama" && c.Provider != "custom" {
		return fmt.Errorf("invalid provider: %s (must be 'openai', 'ollama', or 'custom')", c.Provider)
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

func (c *Config) String() string {
	data, _ := yaml.Marshal(c)
	return string(data)
}

func (c *Config) Get(v string) interface{} {
	return viper.Get(v)
}

func (c *Config) Set(v string, value interface{}) {
	viper.Set(v, value)
}

// Initialize creates a default config file
func Initialize() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configPath := home + "/.clai.yaml"

	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("config file already exists at %s", configPath)
	}

	// Create default config
	defaultConfig := `# AI Code Assistant Configuration

# AI Provider (openai, ollama, or custom)
provider: ollama
model: gpt-oss:latest

# Base URL for API endpoint (optional - defaults set per provider)
# For Ollama: http://localhost:11434/
# For custom OpenAI-compatible APIs
# base_url: http://localhost:11434/

# API Key (or use environment variable)
# Not required for Ollama or local models
# api_key: your-api-key-here

# System Prompt (optional - uses default if not set)
# Customize the AI's behavior and personality
# system_prompt: |
#   You are an expert coding assistant...

# Behavior
auto_apply: false      # Automatically apply code changes
show_thinking: true    # Show thinking animation
context_files: 5       # Max files to include in context
max_tokens: 4096       # Max tokens per request
temperature: 0.7       # Model temperature (0.0 - 1.0)


# UI
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
permitted_tools: # Permitted tools
	- list_files
	- search_file
session_dir: .clai   # Where to store session data
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
	fmt.Printf("  Context Files: %d\n", cfg.ContextFiles)
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

// getDefaultSystemPrompt returns the default system prompt
func getDefaultSystemPrompt() string {
	return `You are an expert coding assistant helping developers write, debug, and improve code.

Key responsibilities:
- Write clean, efficient, and well-documented code
- Explain technical concepts clearly
- Suggest best practices and design patterns
- Debug issues and propose fixes
- Refactor code for better maintainability
- Answer questions about programming concepts

When modifying code:
- Preserve existing code style and conventions
- Add comments for complex logic
- Consider edge cases and error handling
- Write code that is production-ready

Always be concise but thorough in your explanations.`
}

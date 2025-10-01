package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/penguinpowernz/clai/config"
	"github.com/penguinpowernz/clai/internal/ai"
	"github.com/penguinpowernz/clai/internal/chat"
	"github.com/penguinpowernz/clai/internal/history"
	"github.com/penguinpowernz/clai/internal/ui"
)

var (
	version = "dev"
	cfgFile string
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Setup context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupts
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt signal, shutting down...")
		cancel()
	}()

	rootCmd := newRootCommand(ctx)
	return rootCmd.Execute()
}

func newRootCommand(ctx context.Context) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "clai [message]",
		Short: "AI-powered coding assistant",
		Long: `A CLI tool for AI-assisted coding using Claude or other LLMs.
Helps you write, refactor, and debug code through conversational AI.

Run without arguments to enter interactive mode, or provide a message to send immediately.`,
		Version: version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if err := os.MkdirAll(cfg.SessionDir, 0755); err != nil {
				return fmt.Errorf("failed to create session directory: %w", err)
			}

			f, err := os.OpenFile(filepath.Join(cfg.SessionDir, "clai.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Errorf("failed to open log file: %w", err)
			}
			defer f.Close()
			log.SetOutput(f)

			aiClient, err := ai.NewClient(cfg)
			if err != nil {
				return fmt.Errorf("failed to create AI client: %w", err)
			}

			sessionID := generateSessionID()
			history.SetSessionID(sessionID)
			history.SetConfig(*cfg)

			cm := ui.NewChatModel(ctx, *cfg)
			session := chat.NewSession(cfg, aiClient, sessionID)
			session.AddObserver(cm)
			cm.AddObserver(session)

			// Enter interactive mode
			go session.InteractiveMode(ctx)
			p := tea.NewProgram(cm, tea.WithAltScreen())
			if _, err := p.Run(); err != nil {
				return fmt.Errorf("error running interactive mode: %w", err)
			}

			fmt.Println("Ended chat session", sessionID)

			return nil
		},
	}

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.clai.yml)")
	rootCmd.PersistentFlags().String("model", "", "AI model to use (e.g., gpt-oss:latest)")
	rootCmd.PersistentFlags().String("provider", "", "AI provider (ollama, openai)")
	rootCmd.PersistentFlags().String("session", "", "The session ID to load history from")
	rootCmd.PersistentFlags().Bool("verbose", false, "verbose output")

	// Chat-specific flags
	rootCmd.Flags().StringSliceP("files", "f", []string{}, "files to include in context")

	// Bind flags to viper
	viper.BindPFlag("model", rootCmd.PersistentFlags().Lookup("model"))
	viper.BindPFlag("provider", rootCmd.PersistentFlags().Lookup("provider"))
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	viper.BindPFlag("no_color", rootCmd.PersistentFlags().Lookup("no-color"))

	return rootCmd
}

func initConfig() error {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}

		// Search for config in home directory
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".clai")

		// Also check XDG config directory
		configDir, err := os.UserConfigDir()
		if err == nil {
			viper.AddConfigPath(configDir + "/clai")
		}
	}

	// Read environment variables
	viper.SetEnvPrefix("CLAI")
	viper.AutomaticEnv()

	// Read config file (ignore not found errors)
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return err
		}
	}

	return nil
}

func generateSessionID() string {
	return uuid.New().String()[:6]
}

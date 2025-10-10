package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/penguinpowernz/clai/config"
	"github.com/penguinpowernz/clai/internal/ai"
	"github.com/penguinpowernz/clai/internal/files"
	"github.com/pkoukk/tiktoken-go"
)

var (
	DefaultRegistry *Registry
)

func init() {
	DefaultRegistry = NewRegistry()
}

type Session interface {
	GetClient() ai.Provider
	ClearMessages()
	Context() (any, []any, []any)
}

// Command represents a slash command
type Command struct {
	Name        string
	Aliases     []string
	Description string
	Usage       string
	Handler     HandlerFunc
}

// HandlerFunc is the function signature for command handlers
type HandlerFunc func(ctx context.Context, args []string, env *Environment) (*Result, error)

// Environment provides context for command execution
type Environment struct {
	Session    Session
	Config     *config.Config
	WorkingDir string
	Files      *files.Context
}

// Result represents the outcome of a command
type Result struct {
	Message      string // Message to display to user
	ShouldExit   bool   // Whether to exit the application
	ClearInput   bool   // Whether to clear the input field
	AddToHistory bool   // Whether to add to conversation history
}

// Registry manages all available commands
type Registry struct {
	commands map[string]*Command
}

// NewRegistry creates a new command registry with default commands
func NewRegistry() *Registry {
	r := &Registry{
		commands: make(map[string]*Command),
	}

	// Register built-in commands
	r.Register(&Command{
		Name:        "help",
		Aliases:     []string{"h", "?"},
		Description: "Show available commands",
		Usage:       "/help [command]",
		Handler:     helpHandler,
	})

	r.Register(&Command{
		Name:        "clear",
		Aliases:     []string{"c"},
		Description: "Clear conversation history",
		Usage:       "/clear",
		Handler:     clearHandler,
	})

	r.Register(&Command{
		Name:        "exit",
		Aliases:     []string{"quit", "q"},
		Description: "Exit the application",
		Usage:       "/exit",
		Handler:     exitHandler,
	})

	// r.Register(&Command{
	// 	Name:        "add",
	// 	Aliases:     []string{"load"},
	// 	Description: "Add file(s) to context",
	// 	Usage:       "/add <file1> [file2] ...",
	// 	Handler:     addFileHandler,
	// })

	// r.Register(&Command{
	// 	Name:        "remove",
	// 	Aliases:     []string{"rm"},
	// 	Description: "Remove file(s) from context",
	// 	Usage:       "/remove <file1> [file2] ...",
	// 	Handler:     removeFileHandler,
	// })

	// r.Register(&Command{
	// 	Name:        "files",
	// 	Aliases:     []string{"ls"},
	// 	Description: "List files in context",
	// 	Usage:       "/files",
	// 	Handler:     listFilesHandler,
	// })

	r.Register(&Command{
		Name:        "model",
		Aliases:     []string{"m"},
		Description: "Show or change the AI model",
		Usage:       "/model [model-name]",
		Handler:     modelHandler,
	})

	r.Register(&Command{
		Name:        "models",
		Description: "Show available AI models",
		Usage:       "/models",
		Handler:     modelsHandler,
	})

	r.Register(&Command{
		Name:        "tokens",
		Aliases:     []string{"t"},
		Description: "Show token usage statistics",
		Usage:       "/tokens",
		Handler:     tokensHandler,
	})

	r.Register(&Command{
		Name:        "system",
		Aliases:     []string{"sys"},
		Description: "Show or update system prompt",
		Usage:       "/system [new prompt]",
		Handler:     systemPromptHandler,
	})

	return r
}

// Register adds a command to the registry
func (r *Registry) Register(cmd *Command) {
	r.commands[cmd.Name] = cmd
	for _, alias := range cmd.Aliases {
		r.commands[alias] = cmd
	}
}

// Get retrieves a command by name or alias
func (r *Registry) Get(name string) (*Command, bool) {
	cmd, ok := r.commands[name]
	return cmd, ok
}

// List returns all unique commands (no aliases)
func (r *Registry) List() []*Command {
	seen := make(map[string]bool)
	var result []*Command

	for _, cmd := range r.commands {
		if !seen[cmd.Name] {
			seen[cmd.Name] = true
			result = append(result, cmd)
		}
	}

	return result
}

// Parse parses a message and determines if it's a command
func (r *Registry) Parse(message string) (isCommand bool, cmdName string, args []string) {
	trimmed := strings.TrimSpace(message)
	if !strings.HasPrefix(trimmed, "/") {
		return false, "", nil
	}

	// Remove leading slash
	trimmed = strings.TrimPrefix(trimmed, "/")

	// Split into parts
	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return false, "", nil
	}

	return true, parts[0], parts[1:]
}

// Execute runs a command
func (r *Registry) Execute(ctx context.Context, message string, env *Environment) (*Result, error) {
	isCommand, cmdName, args := r.Parse(message)
	if !isCommand {
		return nil, fmt.Errorf("not a command")
	}

	cmd, ok := r.Get(cmdName)
	if !ok {
		return &Result{
			Message:    fmt.Sprintf("Unknown command: /%s\nType /help for available commands", cmdName),
			ClearInput: true,
		}, nil
	}

	return cmd.Handler(ctx, args, env)
}

// -------------------------------------------------------------------
// Command Handlers
// -------------------------------------------------------------------

func helpHandler(ctx context.Context, args []string, env *Environment) (*Result, error) {
	// // If specific command requested
	// if len(args) > 0 {
	// 	cmd, ok := env.Session.Commands.Get(args[0])
	// 	if !ok {
	// 		return &Result{
	// 			Message:    fmt.Sprintf("Unknown command: /%s", args[0]),
	// 			ClearInput: true,
	// 		}, nil
	// 	}

	// 	var aliases string
	// 	if len(cmd.Aliases) > 0 {
	// 		aliases = fmt.Sprintf(" (aliases: %s)", strings.Join(cmd.Aliases, ", "))
	// 	}

	// 	return &Result{
	// 		Message: fmt.Sprintf("/%s%s\n%s\nUsage: %s",
	// 			cmd.Name, aliases, cmd.Description, cmd.Usage),
	// 		ClearInput: true,
	// 	}, nil
	// }

	// // List all commands
	var sb strings.Builder
	sb.WriteString("Available Commands:\n\n")

	for _, cmd := range DefaultRegistry.List() {
		sb.WriteString(fmt.Sprintf("  %-12s %s\n", "/"+cmd.Name, cmd.Description))
	}

	// sb.WriteString("\nType /help <command> for more details")

	return &Result{
		Message:    sb.String(),
		ClearInput: true,
	}, nil
}

func clearHandler(ctx context.Context, args []string, env *Environment) (*Result, error) {
	env.Session.ClearMessages()
	return &Result{
		Message:    "Conversation history cleared",
		ClearInput: true,
	}, nil
}

func exitHandler(ctx context.Context, args []string, env *Environment) (*Result, error) {
	return &Result{
		Message:    "Goodbye!",
		ShouldExit: true,
	}, nil
}

func addFileHandler(ctx context.Context, args []string, env *Environment) (*Result, error) {
	if len(args) == 0 {
		return &Result{
			Message:    "Usage: /add <file1> [file2] ...",
			ClearInput: true,
		}, nil
	}

	var added []string
	var failed []string

	for _, path := range args {
		if err := env.Files.AddFile(path); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", path, err))
		} else {
			added = append(added, path)
		}
	}

	var message strings.Builder
	if len(added) > 0 {
		message.WriteString(fmt.Sprintf("Added %d file(s) to context:\n", len(added)))
		for _, f := range added {
			message.WriteString(fmt.Sprintf("  • %s\n", f))
		}
	}

	if len(failed) > 0 {
		message.WriteString(fmt.Sprintf("\nFailed to add %d file(s):\n", len(failed)))
		for _, f := range failed {
			message.WriteString(fmt.Sprintf("  • %s\n", f))
		}
	}

	return &Result{
		Message:    message.String(),
		ClearInput: true,
	}, nil
}

func removeFileHandler(ctx context.Context, args []string, env *Environment) (*Result, error) {
	if len(args) == 0 {
		return &Result{
			Message:    "Usage: /remove <file1> [file2] ...",
			ClearInput: true,
		}, nil
	}

	for _, path := range args {
		env.Files.RemoveFile(path)
	}

	return &Result{
		Message:    fmt.Sprintf("Removed %d file(s) from context", len(args)),
		ClearInput: true,
	}, nil
}

func listFilesHandler(ctx context.Context, args []string, env *Environment) (*Result, error) {
	files := env.Files.GetFiles()

	if len(files) == 0 {
		return &Result{
			Message:    "No files in context",
			ClearInput: true,
		}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Files in context (%d):\n\n", len(files)))

	for _, file := range files {
		sb.WriteString(fmt.Sprintf("  • %s (%s, %d bytes)\n",
			file.Path, file.Language, file.Size))
	}

	sb.WriteString(fmt.Sprintf("\nTotal: %d bytes", env.Files.GetTotalSize()))

	return &Result{
		Message:    sb.String(),
		ClearInput: true,
	}, nil
}

func modelHandler(ctx context.Context, args []string, env *Environment) (*Result, error) {
	// Show current model
	if len(args) == 0 {
		info := env.Session.GetClient().GetModelInfo()
		return &Result{
			Message: fmt.Sprintf("Current model: %s (%s)\nMax tokens: %d\nUse /model <model> to change models",
				info.Name, info.Provider, info.MaxTokens),
			ClearInput: true,
		}, nil
	}

	if len(args) > 0 {
		model := args[0]
		env.Config.Model = model
		return &Result{
			Message:    fmt.Sprintf("Model changed to %s for this session", model),
			ClearInput: true,
		}, nil
	}

	// Change model (would need implementation)
	return &Result{
		Message:    "Changing models not yet implemented",
		ClearInput: true,
	}, nil
}

func modelsHandler(ctx context.Context, args []string, env *Environment) (*Result, error) {
	// List available models

	models := env.Session.GetClient().ListModels()

	var sb strings.Builder
	sb.WriteString("Available models:\n")
	for _, model := range models {
		sb.WriteString(fmt.Sprintf("  - %s\n", model))
	}

	return &Result{
		Message:    sb.String(),
		ClearInput: true,
	}, nil
}

func tokensHandler(ctx context.Context, args []string, env *Environment) (*Result, error) {
	// This would track token usage across the session
	// For now, just show a placeholder

	enc, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		return nil, err
	}

	dump := func(v any) string { d, _ := json.Marshal(v); return string(d) }

	sys, in, out := env.Session.Context()

	system := len(enc.Encode(dump(sys), nil, nil))
	input := len(enc.Encode(dump(in), nil, nil))
	output := len(enc.Encode(dump(out), nil, nil))
	total := system + input + output

	style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))

	return &Result{
		Message: fmt.Sprintf(`  %s: %5d tokens
  %s:  %5d tokens
  %s: %5d tokens
  %s:  %5d tokens
	`,
			style.Render("System"), system,
			style.Render("Input"), input,
			style.Render("Output"), output,
			style.Render("Total"), total,
		),
		ClearInput: true,
	}, nil
}

func systemPromptHandler(ctx context.Context, args []string, env *Environment) (*Result, error) {
	// Show current system prompt
	if len(args) == 0 {
		prompt := env.Config.SystemPrompt
		if prompt == "" {
			prompt = "(no system prompt set)"
		}
		return &Result{
			Message:    fmt.Sprintf("System Prompt:\n\n%s", prompt),
			ClearInput: true,
		}, nil
	}

	// Update system prompt
	newPrompt := strings.Join(args, " ")
	env.Config.SystemPrompt = newPrompt

	return &Result{
		Message:    "System prompt updated for this session",
		ClearInput: true,
	}, nil
}

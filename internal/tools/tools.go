package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/penguinpowernz/clai/config"
)

// Tool represents a function the AI can call
type Tool struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	InputSchema ToolSchema `json:"input_schema"`
}

type ToolSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required"`
}

type Property struct {
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Items       *Property `json:"items,omitempty"`
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
	return []Tool{
		{
			Name:        "list_files",
			Description: "List files and directories in a given path. Returns file names, types (file/directory), and sizes.",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]Property{
					"path": {
						Type:        "string",
						Description: "The directory path to list. Use '.' for current directory.",
					},
					"recursive": {
						Type:        "boolean",
						Description: "Whether to list files recursively in subdirectories.",
					},
				},
				Required: []string{"path"},
			},
		},
		{
			Name:        "read_file",
			Description: "Read the contents of a file. Returns the file content as a string.",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]Property{
					"path": {
						Type:        "string",
						Description: "The path to the file to read.",
					},
				},
				Required: []string{"path"},
			},
		},
		{
			Name:        "write_file",
			Description: "Write content to a file. Creates the file if it doesn't exist, overwrites if it does.",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]Property{
					"path": {
						Type:        "string",
						Description: "The path to the file to write.",
					},
					"content": {
						Type:        "string",
						Description: "The content to write to the file.",
					},
				},
				Required: []string{"path", "content"},
			},
		},
		{
			Name:        "search_files",
			Description: "Search for files matching a pattern (glob) in a directory.",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]Property{
					"pattern": {
						Type:        "string",
						Description: "Glob pattern to match (e.g., '*.go', 'src/**/*.js')",
					},
					"path": {
						Type:        "string",
						Description: "Directory to search in. Defaults to current directory.",
					},
				},
				Required: []string{"pattern"},
			},
		},
	}
}

type toolExecutor func(cfg config.Config, toolUse json.RawMessage, workingDir string) (string, error)

// ExecuteTool executes a tool and returns the result
func ExecuteTool(cfg *config.Config, toolUse ToolUse, workingDir string) ToolResult {
	result := ToolResult{
		ToolUseID: toolUse.ID,
	}

	var tool toolExecutor
	switch toolUse.Name {
	case "list_files":
		tool = listFiles
	case "read_file":
		tool = readFile
	case "write_file":
		tool = writeFile
	case "search_files":
		tool = searchFiles
	default:
		result.Content = fmt.Sprintf("Unknown tool: %s", toolUse.Name)
		result.IsError = true
		return result
	}

	if tool == nil {
		result.Content = fmt.Sprintf("Unknown tool: %s", toolUse.Name)
		result.IsError = true
		return result
	}

	content, err := tool(*cfg, toolUse.Input, workingDir)
	if err != nil {
		result.Content = fmt.Sprintf("Error: %v", err)
		result.IsError = true
	} else {
		result.Content = content
	}

	return result
}

// Tool implementation functions

func listFiles(cfg config.Config, input json.RawMessage, workingDir string) (string, error) {
	var params struct {
		Path      string `json:"path"`
		Recursive bool   `json:"recursive"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", err
	}

	targetPath := filepath.Join(workingDir, params.Path)

	var files []string
	if params.Recursive {
		err := filepath.Walk(targetPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			relPath, _ := filepath.Rel(workingDir, path)
			fileType := "file"
			if info.IsDir() {
				fileType = "directory"
			}
			files = append(files, fmt.Sprintf("%s (%s, %d bytes)", relPath, fileType, info.Size()))
			return nil
		})
		if err != nil {
			return "", err
		}
	} else {
		entries, err := os.ReadDir(targetPath)
		if err != nil {
			return "", err
		}
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			fileType := "file"
			if entry.IsDir() {
				fileType = "directory"
			}
			files = append(files, fmt.Sprintf("%s (%s, %d bytes)", entry.Name(), fileType, info.Size()))
		}
	}

	return strings.Join(files, "\n"), nil
}

func readFile(cfg config.Config, input json.RawMessage, workingDir string) (string, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", err
	}

	targetPath := filepath.Join(workingDir, params.Path)

	// Security check: ensure path is within working directory
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return "", err
	}
	absWorking, err := filepath.Abs(workingDir)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(absTarget, absWorking) {
		return "", fmt.Errorf("access denied: path outside working directory")
	}

	// check the file is not excluded
	for _, pattern := range cfg.ExcludePatterns {
		matched, _ := filepath.Match(pattern, filepath.Base(targetPath))
		if matched {
			return "", fmt.Errorf("file matches exclude pattern")
		}
	}

	content, err := os.ReadFile(targetPath)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

func writeFile(cfg config.Config, input json.RawMessage, workingDir string) (string, error) {
	var params struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", err
	}

	targetPath := filepath.Join(workingDir, params.Path)

	// Security check: ensure path is within working directory
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return "", err
	}
	absWorking, err := filepath.Abs(workingDir)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(absTarget, absWorking) {
		return "", fmt.Errorf("access denied: path outside working directory")
	}

	// Create parent directories if needed
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return "", err
	}

	if err := os.WriteFile(targetPath, []byte(params.Content), 0644); err != nil {
		return "", err
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(params.Content), params.Path), nil
}

func searchFiles(cfg config.Config, input json.RawMessage, workingDir string) (string, error) {
	var params struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", err
	}

	if params.Path == "" {
		params.Path = "."
	}

	targetPath := filepath.Join(workingDir, params.Path)
	pattern := filepath.Join(targetPath, params.Pattern)

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	}

	var results []string
	for _, match := range matches {
		relPath, _ := filepath.Rel(workingDir, match)
		info, err := os.Stat(match)
		if err != nil {
			continue
		}
		results = append(results, fmt.Sprintf("%s (%d bytes)", relPath, info.Size()))
	}

	if len(results) == 0 {
		return "No files found matching pattern", nil
	}

	return strings.Join(results, "\n"), nil
}

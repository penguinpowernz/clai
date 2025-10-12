package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/penguinpowernz/clai/config"
)

func init() { DefaultTools = append(DefaultTools, _writeFile) }

var _writeFile = Tool{
	exec: writeFile,
	Type: "function",
	Function: &FunctionSchema{
		Name:        "write_file",
		Description: "Write content to a file. Creates the file if it doesn't exist, overwrites if it does.",
		Parameters: &JSONSchema{
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

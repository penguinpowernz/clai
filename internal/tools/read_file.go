package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/penguinpowernz/clai/config"
)

func init() { DefaultTools = append(DefaultTools, _readFile) }

var _readFile = Tool{
	exec: readFile,
	Type: "function",
	Function: &FunctionSchema{
		Name:        "read_file",
		Description: "Read the contents of a file. Returns the file content as a string.",
		Parameters: &JSONSchema{
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

	targetPath = strings.Replace(targetPath, workingDir, "", 1)
	out := "// " + targetPath + "\n" + string(content)
	return out, nil
}

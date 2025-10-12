package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/penguinpowernz/clai/config"
)

func init() { DefaultTools = append(DefaultTools, _searchFile) }

var _searchFile = Tool{
	Type: "function",
	Function: &FunctionSchema{
		Name:        "search_files",
		Description: "Search for files matching a pattern (glob) in a directory.",
		Parameters: &JSONSchema{
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
	exec: searchFile,
}

func searchFile(cfg config.Config, input json.RawMessage, workingDir string) (string, error) {
	var params struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", err
	}

	// SEC: strip path traversal
	absTarget, err := filepath.Abs(params.Path)
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

	targetPath := filepath.Join(absWorking, params.Path)

	// check the file is not excluded
	for _, pattern := range cfg.ExcludePatterns {
		matched, _ := filepath.Match(pattern, filepath.Base(targetPath))
		if matched {
			return "", fmt.Errorf("file matches exclude pattern")
		}
	}
	// check the file is not excluded
	for _, pattern := range cfg.ExcludePatterns {
		matched, _ := filepath.Match(pattern, filepath.Base(params.Path))
		if matched {
			return "", fmt.Errorf("file matches exclude pattern")
		}
	}

	cmd := exec.Command("grep", params.Pattern, targetPath)
	buf := &bytes.Buffer{}
	cmd.Stderr = buf
	cmd.Stdout = buf
	cmd.Run()

	return buf.String(), nil
}

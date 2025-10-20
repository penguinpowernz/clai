package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/penguinpowernz/clai/config"
)

func init() { DefaultTools = append(DefaultTools, _searchFiles) }

var _searchFiles = Tool{
	exec: searchFiles,
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

	if IsExcluded(cfg, params.Path) {
		return "ERROR: the requested path does not exist", nil
	}

	if !Exists(params.Path) {
		return "ERROR: the requested path does not exist", nil
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

		if IsExcluded(cfg, match) {
			continue
		}

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

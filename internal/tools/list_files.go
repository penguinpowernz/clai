package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/penguinpowernz/clai/config"
)

func init() { DefaultTools = append(DefaultTools, _listFiles) }

var _listFiles = Tool{
	exec: listFiles,
	Type: "function",
	Function: &FunctionSchema{
		Name:        "list_files",
		Description: "List files and directories in a given path. Returns file names, types (file/directory), and sizes.",
		Parameters: &JSONSchema{
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
}

func listFiles(cfg config.Config, input json.RawMessage, workingDir string) (string, error) {
	var params struct {
		Path      string `json:"path"`
		Recursive bool   `json:"recursive"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", err
	}

	targetPath := filepath.Join(workingDir, params.Path)

	if IsExcluded(cfg, targetPath) {
		return "ERROR: the requested path does not exist", nil
	}

	var files []string
	if params.Recursive {
		err := filepath.Walk(targetPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			relPath, _ := filepath.Rel(workingDir, path)

			fileType := "file"
			if info.IsDir() {
				if IsExcluded(cfg, path) {
					return filepath.SkipDir
				}

				fileType = "directory"
			}

			if IsExcluded(cfg, path) {
				return nil
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

			if IsExcluded(cfg, entry.Name()) {
				continue
			}

			files = append(files, fmt.Sprintf("%s (%s, %d bytes)", entry.Name(), fileType, info.Size()))
		}
	}

	return strings.Join(files, "\n"), nil
}

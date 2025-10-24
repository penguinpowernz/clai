package tools

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/penguinpowernz/clai/config"
)

func init() { DefaultTools = append(DefaultTools, _mkdir) }

var _mkdir = Tool{
	exec: mkdir,
	Type: "function",
	Function: &FunctionSchema{
		Name:        "mkdir",
		Description: "Create a directory, -p is used by default",
		Parameters: &JSONSchema{
			Type: "object",
			Properties: map[string]Property{
				"path": {
					Type:        "string",
					Description: "The path to the directory to create.",
				},
			},
			Required: []string{"path"},
		},
	},
}

func mkdir(cfg config.Config, input json.RawMessage, workingDir string) (string, error) {
	d := struct {
		Path string `json:"path"`
	}{}
	if err := json.Unmarshal(input, &d); err != nil {
		return "", err
	}

	d.Path = Sanitize(d.Path)

	if IsExcluded(cfg, d.Path) {
		return "ERROR: the requested path does not exist", nil
	}

	targetPath := filepath.Join(workingDir, d.Path)
	err := os.MkdirAll(targetPath, 0755)

	reply := "directory was created"
	if err != nil {
		reply = "ERROR: " + err.Error()
	}

	return reply, nil
}

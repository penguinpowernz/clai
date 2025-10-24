package tools

import (
	"bytes"
	"encoding/json"
	"os/exec"

	"github.com/penguinpowernz/clai/config"
)

func init() { DefaultTools = append(DefaultTools, _filetype) }

var _filetype = Tool{
	exec: filetype,
	Type: "function",
	Function: &FunctionSchema{
		Name:        "filetype",
		Description: "Returns the file type of a file using the linux `file` tool",
		Parameters: &JSONSchema{
			Type: "object",
			Properties: map[string]Property{
				"path": {
					Type:        "string",
					Description: "The path to the file",
				},
			},
			Required: []string{"path"},
		},
	},
}

func filetype(cfg config.Config, input json.RawMessage, workingDir string) (string, error) {
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

	cmd := exec.Command("file", d.Path)
	var sout, serr bytes.Buffer
	cmd.Stdout = &sout
	cmd.Stderr = &serr

	err := cmd.Run()
	if err != nil {
		return "", err
	}

	return jsonDump(map[string]any{
		"stdout":     sout.String(),
		"stderr":     serr.String(),
		"exitstatus": cmd.ProcessState.ExitCode(),
	}), nil
}

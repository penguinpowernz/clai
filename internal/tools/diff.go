package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/penguinpowernz/clai/config"
)

func init() { DefaultTools = append(DefaultTools, _diff) }

var _diff = Tool{
	exec: diff,
	Type: "function",
	Function: &FunctionSchema{
		Name:        "diff",
		Description: "Diff two files",
		Parameters: &JSONSchema{
			Type: "object",
			Properties: map[string]Property{
				"file1": {
					Type:        "string",
					Description: "The path to the first file",
				},
				"file2": {
					Type:        "string",
					Description: "The path to the second file",
				},
				"args": {
					Type:        "string",
					Description: "The arguments to pass to diff",
				},
			},
			Required: []string{"file1", "file2"},
		},
	},
}

func diff(cfg config.Config, input json.RawMessage, workingDir string) (string, error) {
	d := struct {
		File1 string `json:"file1"`
		File2 string `json:"file2"`
		Args  string `json:"args"`
	}{}
	if err := json.Unmarshal(input, &d); err != nil {
		return "", err
	}

	d.File1 = Sanitize(d.File1)
	d.File2 = Sanitize(d.File2)

	if IsExcluded(cfg, d.File1) {
		return "ERROR: the requested path for file1 does not exist", nil
	}

	if IsExcluded(cfg, d.File2) {
		return "ERROR: the requested path for file2 does not exist", nil
	}

	cmdStr := fmt.Sprintf("diff %s %s %s", d.Args, d.File1, d.File2)
	cmd := exec.Command("sh", "-c", cmdStr)
	var sout, serr bytes.Buffer
	cmd.Stdout = &sout
	cmd.Stderr = &serr

	err := cmd.Run()
	if err != nil {
		return "", err
	}

	return jsonDump(map[string]any{
		"cmd":        cmdStr,
		"stdout":     sout.String(),
		"stderr":     serr.String(),
		"exitstatus": cmd.ProcessState.ExitCode(),
	}), nil
}

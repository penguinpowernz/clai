package tools

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"

	"github.com/penguinpowernz/clai/config"
)

func init() { DefaultTools = append(DefaultTools, _grep) }

var _grep = Tool{
	exec: grep,
	Type: "function",
	Function: &FunctionSchema{
		Name:        "grep",
		Description: "Find content inside of a file or files",
		Parameters: &JSONSchema{
			Type: "object",
			Properties: map[string]Property{
				"pattern": {
					Type:        "string",
					Description: "The pattern to match",
				},
				"path": {
					Type:        "string",
					Description: "The path to search",
				},
				"regex": {
					Type:        "boolean",
					Description: "Whether to treat the pattern as PCRE",
				},
				"recursive": {
					Type:        "boolean",
					Description: "Whether to search recursively (default: false)",
				},
				"case_insensitive": {
					Type:        "boolean",
					Description: "Whether to match case sensitively (default: false)",
				},
			},
		},
	},
}

func grep(cfg config.Config, input json.RawMessage, workingDir string) (string, error) {

	var params struct {
		Pattern         string `json:"pattern"`
		Path            string `json:"path"`
		Regex           bool   `json:"regex"`
		Recursive       bool   `json:"recursive"`
		CaseInsensitive bool   `json:"case_insensitive"`
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

	cmd := exec.Command("grep")
	bufo := bytes.NewBuffer(nil)
	bufe := bytes.NewBuffer(nil)
	cmd.Stdout = bufo
	cmd.Stderr = bufe

	cmd.Args = append(cmd.Args, params.Pattern)

	if params.Recursive {
		cmd.Args = append(cmd.Args, "-R")
	}
	if params.CaseInsensitive {
		cmd.Args = append(cmd.Args, "-i")
	}
	if params.Regex {
		cmd.Args = append(cmd.Args, "-P")
	}

	cmd.Args = append(cmd.Args, targetPath)

	_ = cmd.Run()

	return jsonDump(map[string]any{
		"stdout":     bufo.String(),
		"stderr":     bufe.String(),
		"exitstatus": cmd.ProcessState.ExitCode(),
	}), nil
}

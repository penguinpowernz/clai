package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/penguinpowernz/clai/config"
)

func init() { DefaultTools = append(DefaultTools, _find) }

var _find = Tool{
	exec: find,
	Type: "function",
	Function: &FunctionSchema{
		Name:        "find",
		Description: "Find files using the linux find command",
		Parameters: &JSONSchema{
			Type: "object",
			Properties: map[string]Property{
				"path": {
					Type:        "string",
					Description: "The path to search in",
				},
				"raw_args": {
					Type:        "string",
					Description: "The raw arguments to pass to find, do not include the path, exec is not allowed",
				},
			},
			Required: []string{"path"},
		},
	},
}

func find(cfg config.Config, input json.RawMessage, workingDir string) (string, error) {
	d := struct {
		Path    string `json:"path"`
		RawArgs string `json:"raw_args"`
	}{}
	if err := json.Unmarshal(input, &d); err != nil {
		return "", err
	}

	if IsExcluded(cfg, d.Path) {
		return "ERROR: the requested path does not exist", nil
	}

	cmdStr := fmt.Sprintf("find %s %s", d.Path, d.RawArgs)
	if strings.Contains(cmdStr, "-exec") {
		return "ERROR: exec is not allowed", nil
	}

	cmd := exec.Command("sh", "-c", cmdStr)

	var sout, serr bytes.Buffer

	cmd.Stdout = &sout
	cmd.Stderr = &serr

	if err := cmd.Run(); err != nil {
		return "", err
	}

	okLines := []string{}
	for _, line := range strings.Split(sout.String(), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !IsExcluded(cfg, line) {
			okLines = append(okLines, line)
		}
	}

	return jsonDump(map[string]any{
		"cmd":        cmdStr,
		"stdout":     strings.Join(okLines, "\n"),
		"stderr":     serr.String(),
		"exitstatus": cmd.ProcessState.ExitCode(),
	}), nil
}

package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/penguinpowernz/clai/config"
)

func IsValid(tools []Tool, toolName string) bool {
	for _, tool := range tools {
		if tool.Function.Name == toolName {
			return true
		}
	}
	return false
}

func GetNames(tools []Tool) []string {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Function.Name
	}
	return names
}

func Exists(fn string) bool {
	_, err := os.Stat(fn)
	return !os.IsNotExist(err)
}

func PrepareFilePath(cfg config.Config, workingDir string, fn string) (string, error) {
	fn = Sanitize(fn)
	if IsExcluded(cfg, fn) {
		return "", fmt.Errorf("file matches exclude pattern")
	}

	path := filepath.Join(workingDir, fn)
	if !Exists(path) {
		return "", fmt.Errorf("file not found")
	}

	return path, nil
}

func jsonDump(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

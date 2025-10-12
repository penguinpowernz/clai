package tools

import (
	"path/filepath"
	"strings"

	"github.com/penguinpowernz/clai/config"
)

func Sanitize(path string) string {
	path = strings.TrimPrefix(path, "/")
	path = strings.ReplaceAll(path, "../", "")
	return path
}

func IsExcluded(cfg config.Config, path string) bool {
	if path[0] == '/' {
		return true
	}

	for _, pattern := range cfg.ExcludePatterns {
		matched, _ := filepath.Match(pattern, filepath.Base(path))
		if matched {
			return true
		}
		// Also check if path contains pattern (for directories)
		if strings.Contains(path, strings.TrimSuffix(pattern, "/")) {
			return true
		}
	}
	return false
}

package tools

import (
	"path/filepath"
	"strings"

	"github.com/penguinpowernz/clai/config"
)

func IsExcluded(cfg config.Config, path string) bool {
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

package tools

import (
	"testing"

	"github.com/penguinpowernz/clai/config"
	"github.com/stretchr/testify/assert"
)

func TestIsExcluded(t *testing.T) {
	cfg := config.Default()
	cfg.ExcludePatterns = []string{
		"node_modules/",
		".git/",
		"*.log",
		"*.tmp",
		"vendor/",
		"dist/",
		"build/",
	}

	assert.False(t, IsExcluded(*cfg, "internal/tool/tools.go"))
	assert.False(t, IsExcluded(*cfg, "main.go"))
	assert.True(t, IsExcluded(*cfg, "vendor/modules.txt"))
	assert.True(t, IsExcluded(*cfg, "test.log"))
	assert.True(t, IsExcluded(*cfg, "logs/test.log"))
	assert.True(t, IsExcluded(*cfg, "/etc/passwd"))
}

func TestSanitize(t *testing.T) {
	assert.Equal(t, "etc/passwd", Sanitize("/etc/passwd"))
	assert.Equal(t, "etc/passwd", Sanitize("../etc/passwd"))
	assert.Equal(t, "etc/passwd", Sanitize("/../etc/passwd"))
	assert.Equal(t, "etc/passwd", Sanitize("/../../../etc/passwd"))
	assert.Equal(t, "./etc/passwd", Sanitize("./etc/passwd"))
}

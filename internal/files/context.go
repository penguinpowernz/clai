package files

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/penguinpowernz/clai/config"
)

// Context manages file context for AI requests
type Context struct {
	config     *config.Config
	workingDir string
	files      map[string]*File
	gitRepo    string
	totalSize  int64
}

// File represents a single file in context
type File struct {
	Path         string
	Content      string
	Size         int64
	Language     string
	LastModified int64
}

// NewContext creates a new file context manager
func NewContext(cfg *config.Config) *Context {
	wd, _ := os.Getwd()

	return &Context{
		config:     cfg,
		workingDir: wd,
		files:      make(map[string]*File),
	}
}

// AddFile adds a file to the context
func (c *Context) AddFile(path string) error {
	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Check if file exists
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}

	// Check size limit
	if info.Size() > c.config.MaxFileSize {
		return fmt.Errorf("file too large: %d bytes (max: %d)", info.Size(), c.config.MaxFileSize)
	}

	// Check if excluded
	if c.isExcluded(absPath) {
		return fmt.Errorf("file matches exclude pattern")
	}

	// Read file
	content, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Detect language
	lang := detectLanguage(absPath)

	c.files[absPath] = &File{
		Path:         absPath,
		Content:      string(content),
		Size:         info.Size(),
		Language:     lang,
		LastModified: info.ModTime().Unix(),
	}

	c.totalSize += info.Size()

	return nil
}

// RemoveFile removes a file from context
func (c *Context) RemoveFile(path string) {
	absPath, _ := filepath.Abs(path)
	if file, exists := c.files[absPath]; exists {
		c.totalSize -= file.Size
		delete(c.files, absPath)
	}
}

// GetFiles returns all files in context
func (c *Context) GetFiles() []*File {
	files := make([]*File, 0, len(c.files))
	for _, file := range c.files {
		files = append(files, file)
	}
	return files
}

// BuildPrompt builds a prompt with file context
func (c *Context) BuildPrompt(userMessage string) string {
	var sb strings.Builder

	// Add file context
	if len(c.files) > 0 {
		sb.WriteString("Here are the relevant files:\n\n")

		for _, file := range c.files {
			relPath, _ := filepath.Rel(c.workingDir, file.Path)
			sb.WriteString(fmt.Sprintf("--- %s ---\n", relPath))
			sb.WriteString(fmt.Sprintf("```%s\n", file.Language))
			sb.WriteString(file.Content)
			sb.WriteString("\n```\n\n")
		}
	}

	// Add user message
	sb.WriteString(userMessage)

	return sb.String()
}

// Clear removes all files from context
func (c *Context) Clear() {
	c.files = make(map[string]*File)
	c.totalSize = 0
}

// GetTotalSize returns total size of all files in context
func (c *Context) GetTotalSize() int64 {
	return c.totalSize
}

// GetFileCount returns number of files in context
func (c *Context) GetFileCount() int {
	return len(c.files)
}

// isExcluded checks if a path matches exclude patterns
func (c *Context) isExcluded(path string) bool {
	for _, pattern := range c.config.ExcludePatterns {
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

// detectLanguage detects the programming language from file extension
func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))

	langMap := map[string]string{
		".go":   "go",
		".py":   "python",
		".js":   "javascript",
		".ts":   "typescript",
		".jsx":  "javascript",
		".tsx":  "typescript",
		".java": "java",
		".c":    "c",
		".cpp":  "cpp",
		".h":    "c",
		".hpp":  "cpp",
		".rs":   "rust",
		".rb":   "ruby",
		".php":  "php",
		".sh":   "bash",
		".yml":  "yaml",
		".yaml": "yaml",
		".json": "json",
		".xml":  "xml",
		".html": "html",
		".css":  "css",
		".sql":  "sql",
		".md":   "markdown",
	}

	if lang, exists := langMap[ext]; exists {
		return lang
	}

	return "text"
}

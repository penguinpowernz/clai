package ui

import (
	"regexp"
	"strings"
)

var reThinkBlock = regexp.MustCompile("(?s)<think>.*?</think>")

func stripThinkBlock(content string) string {
	out := reThinkBlock.ReplaceAllString(content, "")
	return strings.TrimSpace(out)
}

package ui

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
	"github.com/penguinpowernz/clai/internal/ai"
)

func createToolPermissionList() list.Model {
	items := []list.Item{
		list.Item(permissionItem{title: optAllowToolThisTime, desc: ""}),
		list.Item(permissionItem{title: optAllowToolThisSession, desc: ""}),
		list.Item(permissionItem{title: optDisallowTool, desc: ""}),
	}

	// Create a simple delegate for single-line items
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetHeight(1)
	delegate.Styles.SelectedTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("200"))

	l := list.New(items, delegate, 0, 0)
	l.Title = "Tool Permission"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.SetShowTitle(true)
	return l
}

type permissionItem struct {
	title, desc string
}

func (i permissionItem) FilterValue() string { return i.title }
func (i permissionItem) Title() string       { return i.title }
func (i permissionItem) Description() string { return i.desc }

func (m ChatModel) renderToolPermissionOptions() string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Tool: %s\n\n", m.pendingToolCall.Name))

	for i, option := range m.toolPermissionOptions {
		cursor := " "
		if i == m.selectedOption {
			cursor = ">"
		}
		b.WriteString(fmt.Sprintf("%s %s\n", cursor, option))
	}

	return b.String()
}

func (m ChatModel) onRunningTool(msg EventRunningTool) {
	m.runningTool = true
	m.typing = false
	m.thinking = false

	m.addMessage("system", fmt.Sprintf("Running tool: %s with args: %s", msg.Name, msg.Input))

	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
}

var reToolCallCheck = regexp.MustCompile(`^Request to use tool: .* with args: .*`)
var reToolParse = regexp.MustCompile("^Request to use tool: `(.*)` with args: (.*)$")

func actuallyAToolCall(finalContent string) (EventToolCall, bool) {
	if !reToolCallCheck.MatchString(finalContent) {
		return EventToolCall{}, false
	}

	matches := reToolParse.FindStringSubmatch(finalContent)

	tool := matches[1]
	args := matches[2]

	return EventToolCall{Name: tool, Input: json.RawMessage(args)}, false
}

func (m *ChatModel) onToolOutput(output string) {
	if lines := strings.Split(output, "\n"); len(lines) > 3 {
		lines = lines[:3]
		for i := range lines {
			lines[i] = "> " + lines[i]
		}
		output = strings.Join(lines[:3], "\n") + "\n> [...]"
	}

	// Add tool output to chat messages
	m.addMessage("tool", "Tool output:\n"+output)
}

func (m *ChatModel) OnToolCallReceived(toolCall EventToolCall) {
	m.thinking = false
	m.typing = false

	// Log tool call
	log.Println("[ui] Tool call received in UI:", toolCall.Name)

	// Set pending tool call and switch to tool permission mode
	x := ai.ToolCall(toolCall)
	m.pendingToolCall = &x
	m.selectedOption = 0 // Reset to first option

	// Blur textarea to remove focus
	m.textarea.Blur()

	// Format tool arguments for display
	argsStr := ""
	if len(toolCall.Input) > 0 {
		argsStr = fmt.Sprintf(" with args: %s", toolCall.Input)
	}

	// Add a system message about the tool call
	m.addMessage("assistant", fmt.Sprintf("I need to use the tool \"%s\" with args %s", toolCall.Name, argsStr))

	// Update viewport
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
}

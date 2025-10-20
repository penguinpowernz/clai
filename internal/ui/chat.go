package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"github.com/penguinpowernz/clai/config"
	"github.com/penguinpowernz/clai/internal/ai"
	"github.com/penguinpowernz/clai/internal/commands"
	"github.com/penguinpowernz/clai/internal/history"
)

const (
	optAllowToolThisTime    = "Allow to run this time only"
	optAllowToolThisSession = "Allow, and don't ask again this session"
	optDisallowTool         = "Don't allow to run the tool, give the prompt back"

	maxLineLength = 120
)

type EventSlashCommand commands.Result
type EventExit struct{}
type EventCancelStream struct{}
type EventStreamCancelled struct{}
type EventStreamStarted string
type EventStreamThink string
type EventStreamEnded string
type EventStreamChunk string
type EventToolCall ai.ToolCall
type EventPermitToolUse ai.ToolCall
type EventPermitToolUseThisSession ai.ToolCall
type EventCancelToolUse ai.ToolCall
type EventSystemMsg string
type EventUserPrompt string
type EventStreamErr error
type EventAssistantMessage string
type EventRunningTool ai.ToolCall
type EventRunningToolDone string
type EventToolOutput string

type UIObserver interface {
	Observe(chan any)
}

// ChatModel is the bubbletea model for the REPL
type ChatModel struct {
	ctx           context.Context
	cfg           config.Config
	viewport      viewport.Model
	textarea      textarea.Model
	spinner       spinner.Model
	messages      []ai.Message
	typing        bool
	runningTool   bool
	thinking      bool
	inThinkBlock  bool
	err           error
	width         int
	height        int
	currentStream *strings.Builder
	in, out       chan any

	// Tool permission selection
	pendingToolCall       *ai.ToolCall
	toolPermissionList    list.Model
	toolPermissionOptions []string
	selectedOption        int
}

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

func NewChatModel(ctx context.Context, cfg config.Config) *ChatModel {
	ta := textarea.New()
	ta.Placeholder = "Type your message... (Ctrl+D to send, Ctrl+C to quit)"
	ta.Focus()
	ta.Prompt = "│ "
	ta.CharLimit = 0
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.FocusedStyle.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("34"))
	ta.BlurredStyle.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("34"))

	vp := viewport.New(80, 20)
	vp.SetContent(welcomeMessage())

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	model := ChatModel{
		ctx:                   ctx,
		cfg:                   cfg,
		textarea:              ta,
		viewport:              vp,
		spinner:               sp,
		messages:              make([]ai.Message, 0),
		currentStream:         &strings.Builder{},
		in:                    make(chan any),
		out:                   make(chan any),
		toolPermissionList:    createToolPermissionList(),
		toolPermissionOptions: []string{optAllowToolThisTime, optAllowToolThisSession, optDisallowTool},
		selectedOption:        0,
	}

	return &model
}

func (m *ChatModel) addMessage(role, msg string) {
	m.messages = append(m.messages, ai.Message{
		Role:    role,
		Content: msg,
	})

	if m.cfg.SaveHistory == true {
		if err := history.SaveHistory("ui", m.messages); err != nil {
			log.Println("[ui] Error saving history:", err)
		}
	}
}

func (m *ChatModel) AddObserver(observer UIObserver) {
	observer.Observe(m.out)
}

func (m *ChatModel) Observe(events chan any) {
	m.in = events
}

func (m *ChatModel) onSystemMessage(msg string) {
	// Add system message to chat messages
	m.addMessage("system", msg)

	// Update viewport
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
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

func (m *ChatModel) onStreamStarted() {
	log.Println("[ui] STREAM STARTED")
	m.typing = false
	m.currentStream.Reset()

	m.thinking = true
	m.addMessage("thinking", m.currentStream.String())

	// Update viewport
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
}

func (m *ChatModel) onStreamThink(chunk string) {
	m.currentStream.WriteString(chunk)

	// Update the last streaming message
	if len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == "thinking" {
		m.messages[len(m.messages)-1].Content = m.currentStream.String()
	}

	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
}

func (m *ChatModel) onStreamChunk(chunk string) {
	if chunk == "<think>" {
		m.inThinkBlock = true
		m.onStreamThink(chunk)
		return
	}

	if m.inThinkBlock {
		m.onStreamThink(chunk)
		if chunk == "</think>" {
			m.inThinkBlock = false
			return
		}
		return
	}

	if m.thinking {
		m.currentStream.Reset()
		// Add a streaming assistant message
		m.addMessage("assistant-streaming", "")
		m.thinking = false
		m.typing = true
	}

	m.currentStream.WriteString(chunk)

	// Update the last streaming message
	if len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == "assistant-streaming" {
		m.messages[len(m.messages)-1].Content = m.currentStream.String()
	}

	// Update viewport
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
}

func (m *ChatModel) onStreamEnded(finalContent string) {
	m.typing = false
	m.thinking = false

	finalContent = stripThinkBlock(finalContent)

	// Finalize the streaming message
	if len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == "assistant-streaming" {
		m.messages[len(m.messages)-1].Role = "assistant"
		m.messages[len(m.messages)-1].Content = finalContent
	}

	// sometimes the agent will put the tool call inside the chat
	if ev, yes := actuallyAToolCall(finalContent); yes {
		m.in <- ev
		return
	}

	// Reset current stream
	m.currentStream.Reset()

	// Update viewport
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	log.Println("[ui] we ended! final was ", finalContent)
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

func (m *ChatModel) onAssistantMessage(msg string) {
	// Add assistant message to chat messages
	m.addMessage("assistant", msg)

	// Update viewport
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
}

func (m ChatModel) Init() tea.Cmd {
	// No need to manually set system message handler anymore
	return textarea.Blink
}

func listen(m ChatModel) tea.Cmd {
	return func() tea.Msg {
		return <-m.in
	}
}

func (m ChatModel) handleToolCallResponse() (tea.Model, tea.Cmd) {
	selectedOption := m.toolPermissionOptions[m.selectedOption]
	switch selectedOption {
	case optAllowToolThisTime:
		log.Println("[ui] allowing tool use for this time")
		m.out <- EventPermitToolUse(*m.pendingToolCall)
		m.runningTool = true
		// TODO: Execute the tool with the provided arguments
		// The tool name is: m.pendingToolCall.Name
		// The tool args are: m.pendingToolCall.Args

	case optAllowToolThisSession:
		log.Println("[ui] allowing tool use for this session")
		m.out <- EventPermitToolUseThisSession(*m.pendingToolCall)
		m.runningTool = true
		// TODO: Add this tool to permanently allowed tools list
		// TODO: Execute the tool with the provided arguments
		// The tool name is: m.pendingToolCall.Name
		// The tool args are: m.pendingToolCall.Args

	case optDisallowTool:
		log.Println("[ui] cancelling tool use")
		m.out <- EventCancelToolUse(*m.pendingToolCall)
		// TODO: Send cancellation message back to the LLM
		// Let the LLM know that tool use was cancelled by user
	}

	// Reset tool call mode and restore textarea focus
	m.pendingToolCall = nil
	m.selectedOption = 0
	m.textarea.Focus()

	// Update viewport
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m, listen(m)
}

func (m ChatModel) handleSubmit() (tea.Model, tea.Cmd) {
	// If in tool permission mode, handle list selection ONLY
	if m.pendingToolCall != nil {
		return m.handleToolCallResponse()
	}

	// Regular message sending (only when NOT in tool permission mode)
	if m.typing || m.thinking || m.inThinkBlock {
		return m, nil
	}

	userMsg := strings.TrimSpace(m.textarea.Value())
	if userMsg == "" {
		return m, nil
	}

	// Add user message
	m.addMessage("user", userMsg)

	// Clear textarea
	m.textarea.Reset()

	if userMsg[0] != '/' {
		m.thinking = true
	}

	m.currentStream.Reset()

	// Update viewport
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()

	return m, tea.Batch(
		m.spinner.Tick,
		func() tea.Msg { m.out <- EventUserPrompt(userMsg); return nil },
		listen(m),
	)
}

func (m ChatModel) handleSlashCommand(ev EventSlashCommand) (tea.Model, tea.Cmd) {
	res := commands.Result(ev)

	if res.ShouldExit {
		return m, tea.Quit
	}

	m.addMessage("slashcmd", res.Message)
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()

	return m, nil
}

func (m ChatModel) onStreamCancelled() {
	log.Println("[ui] STREAM CANCELLED")
	m.typing = false
	m.thinking = false
	m.inThinkBlock = false
}

func (m ChatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		taCmd   tea.Cmd
		vpCmd   tea.Cmd
		spCmd   tea.Cmd
		listCmd tea.Cmd
	)

	// Only update textarea if we're not in tool permission mode
	if m.pendingToolCall == nil {
		m.textarea, taCmd = m.textarea.Update(msg)
	}
	m.viewport, vpCmd = m.viewport.Update(msg)
	m.spinner, spCmd = m.spinner.Update(msg)

	// Don't update the old list component when in tool permission mode
	if m.pendingToolCall == nil {
		m.toolPermissionList, listCmd = m.toolPermissionList.Update(msg)
	}

	if evt := fmt.Sprintf("%T", msg); evt[0:8] == "ui.Event" {
		log.Println("[ui.event]", evt)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Resize components
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 6 // Leave room for textarea and borders
		m.textarea.SetWidth(msg.Width - 4)

		// Re-render messages with new width
		if m.pendingToolCall != nil {
			m.toolPermissionList.SetWidth(msg.Width - 4)
			m.toolPermissionList.SetHeight(5)
		}
		m.viewport.SetContent(m.renderMessages() + "\n" + m.toolPermissionList.View())
		m.viewport.GotoBottom()

	case tea.KeyMsg:
		// Handle arrow key navigation in tool permission mode
		if m.pendingToolCall != nil {
			switch msg.Type {
			case tea.KeyUp:
				if m.selectedOption > 0 {
					m.selectedOption--
				}
				return m, nil
			case tea.KeyDown:
				if m.selectedOption < len(m.toolPermissionOptions)-1 {
					m.selectedOption++
				}
				return m, nil
			}
		}

		switch msg.String() {
		case "q", "d", "u", "j", "k":
			// Ignore these keys
			return m, nil
		}

		switch msg.Type {
		case tea.KeyEsc:
			log.Println("[ui] Cancel pushed...")

			return m, func() tea.Msg {
				if m.thinking || m.inThinkBlock || m.typing {
					log.Println("[ui] Canceling stream...")
					m.out <- EventCancelStream{}
					log.Println("[ui] Cancelled stream...")
				}
				return nil
			}

		case tea.KeyCtrlC:
			return m, tea.Quit

		case tea.KeyEnter:
			if m.pendingToolCall != nil {
				return m.handleToolCallResponse()
			}

			if msg := strings.TrimSpace(m.textarea.Value()); msg != "" && msg[0] == '/' {
				return m.handleSubmit()
			}

			return m, nil

		case tea.KeyCtrlD:
			return m.handleSubmit()

		case tea.KeyCtrlL:
			// Clear screen (only when NOT in tool permission mode)
			if m.pendingToolCall == nil {
				m.messages = make([]ai.Message, 0)
				m.viewport.SetContent(welcomeMessage())
			}
		}

	case EventSlashCommand:
		return m.handleSlashCommand(msg)

	case EventExit:
		return m, tea.Quit

	case EventStreamChunk:
		m.onStreamChunk(string(msg))
		return m, listen(m)

	case EventSystemMsg:
		m.onSystemMessage(string(msg))

	case EventStreamEnded:
		m.onStreamEnded(string(msg))
		return m, listen(m)

	case EventStreamThink:
		m.onStreamThink(string(msg))
		return m, listen(m)

	case EventStreamCancelled:
		m.onStreamCancelled()
		return m, listen(m)

	case EventStreamStarted:
		m.onStreamStarted()
		return m, listen(m)

	case EventToolCall:
		m.OnToolCallReceived(msg)
		return m, listen(m)

	case EventAssistantMessage:
		m.onAssistantMessage(string(msg))
		return m, listen(m)

	case EventRunningTool:
		m.onRunningTool(msg)

		return m, tea.Batch(
			listen(m),
			m.spinner.Tick,
		)

	case EventRunningToolDone:
		m.runningTool = false
		m.typing = false
		m.thinking = true
		return m, tea.Batch(
			listen(m),
			m.spinner.Tick,
		)

	case EventToolOutput:
		m.onToolOutput(string(msg))
		return m, listen(m)

	}

	return m, tea.Batch(taCmd, vpCmd, spCmd, listCmd)
}

func (m ChatModel) onRunningTool(msg EventRunningTool) {
	m.runningTool = true
	m.typing = false
	m.thinking = false

	m.addMessage("system", fmt.Sprintf("Running tool: %s with args: %s", msg.Name, msg.Input))

	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
}

func (m ChatModel) View() string {
	if m.err != nil {
		return errorStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}

	var status string
	switch {
	case m.typing:
		m.spinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("201"))
		status = fmt.Sprintf("%s Typing...", m.spinner.View())
	case m.thinking:
		m.spinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("40"))
		status = fmt.Sprintf("%s Thinking...", m.spinner.View())
	case m.runningTool:
		m.spinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("21"))
		status = fmt.Sprintf("%s Running tool...", m.spinner.View())
	default:
		status = "👍 Ready"
	}

	var help string
	var inputArea string
	var viewportContent string

	// If we have a pending tool call, show the permission list instead of textarea
	if m.pendingToolCall != nil {
		help = helpStyle.Render("↑/↓: Navigate • Ctrl+D/ENTER: Select • Ctrl+C: Quit")
		inputArea = m.renderToolPermissionOptions()
		status = "👮 Tool Permission Required"

		// Reduce viewport height to make room for the tool permission list
		// We need extra space for the list (about 5 lines)
		tempViewport := viewport.New(m.viewport.Width, m.viewport.Height-5)
		tempViewport.SetContent(m.renderMessages())
		tempViewport.GotoBottom()
		viewportContent = tempViewport.View()
	} else {
		help = helpStyle.Render("Ctrl+D: Send • Ctrl+L: Clear • Ctrl+C: Quit • ESC: Stop AI")
		inputArea = m.textarea.View()
		viewportContent = m.viewport.View()
	}

	return fmt.Sprintf(
		"%s\n\n%s\n\n%s\n%s",
		titleStyle.Render("AI Code Assistant"),
		viewportContent,
		inputArea,
		lipgloss.JoinHorizontal(lipgloss.Left, status, "  ", help),
	)
}

func (m ChatModel) renderMessages() string {
	if len(m.messages) == 0 {
		return welcomeMessage()
	}

	var b strings.Builder
	for _, msg := range m.messages {
		switch msg.Role {
		case "user":
			b.WriteString(userStyle.Render("You: "))
			b.WriteString(msg.Content)
			b.WriteString("\n\n")
		case "assistant", "assistant-streaming":
			b.WriteString(assistantStyle.Render("Assistant: "))
			b.WriteString(msg.Content)
			if msg.Role == "assistant-streaming" {
				b.WriteString(cursorStyle.Render("▋"))
			}
			b.WriteString("\n\n")
		case "system":
			b.WriteString(systemStyle.Render("System: "))
			b.WriteString(msg.Content)
			b.WriteString("\n\n")
		case "tool":
			b.WriteString(toolStyle.Render(msg.Content))
			b.WriteString("\n\n")
		case "slashcmd":
			b.WriteString(msg.Content)
			b.WriteString("\n\n")
		case "thinking":
			b.WriteString(thinkingStyle.Render(msg.Content))
			b.WriteString("\n\n")
		}
	}

	return wordwrap.String(b.String(), min(m.width, maxLineLength))
}

func welcomeMessage() string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("34")).
		Render(`
		

		░██████████████████████████████████████████████████████████
		░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░ 
		
                █████████  █████         █████████   ███████ 
               ███░░░░░███░░███         ███░░░░░███ ░░░███   
              ███     ░░░  ░███        ░███    ░███   ░███   
             ░███          ░███        ░███████████   ░███   
             ░███          ░███        ░███░░░░░███   ░███   
             ░░███     ███ ░███      █ ░███    ░███   ░███   
              ░░█████████  ███████████ █████   █████ ███████ 
               ░░░░░░░░░  ░░░░░░░░░░░ ░░░░░   ░░░░░ ░░░░░░░  

		░██████████████████████████████████████████████████████████
		░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
`)

}

// Styles
var (
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).MarginBottom(1)
	userStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Bold(true)
	assistantStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	systemStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("21")).Bold(true)
	cursorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	helpStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	thinkingStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	toolStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("227"))
	errorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

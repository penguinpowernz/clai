package ui

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/penguinpowernz/clai/internal/ai"
)

const (
	optAllowToolThisTime    = "Allow to run this time only"
	optAllowToolThisSession = "Allow, and don't ask again this session"
	optDisallowTool         = "Don't allow to run the tool, give the prompt back"
)

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

type UIObserver interface {
	Observe(chan any)
}

// ChatModel is the bubbletea model for the REPL
type ChatModel struct {
	ctx           context.Context
	viewport      viewport.Model
	textarea      textarea.Model
	spinner       spinner.Model
	messages      []chatMessage
	waiting       bool
	thinking      bool
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

type chatMessage struct {
	role    string
	content string
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

func NewChatModel(ctx context.Context) *ChatModel {
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
		textarea:              ta,
		viewport:              vp,
		spinner:               sp,
		messages:              make([]chatMessage, 0),
		currentStream:         &strings.Builder{},
		in:                    make(chan any),
		out:                   make(chan any),
		toolPermissionList:    createToolPermissionList(),
		toolPermissionOptions: []string{optAllowToolThisTime, optAllowToolThisSession, optDisallowTool},
		selectedOption:        0,
	}

	return &model
}

func (m *ChatModel) AddObserver(observer UIObserver) {
	observer.Observe(m.out)
}

func (m *ChatModel) Observe(events chan any) {
	m.in = events
}

func (m *ChatModel) onSystemMessage(msg string) {
	// Add system message to chat messages
	m.messages = append(m.messages, chatMessage{
		role:    "system",
		content: msg,
	})

	// Update viewport
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
}

func (m *ChatModel) OnToolCallReceived(toolCall EventToolCall) {
	// Log tool call
	log.Println("Tool call received in UI:", toolCall.Name)

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
	m.messages = append(m.messages, chatMessage{
		role:    "system",
		content: fmt.Sprintf("LLM wants to use tool: %s%s\n\nUse arrow keys and Ctrl+D to select:", toolCall.Name, argsStr),
	})

	// Update viewport
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
}

func (m *ChatModel) onStreamStarted() {
	log.Println("STREAM STARTED")
	m.waiting = true
	m.currentStream.Reset()

	m.thinking = true
	m.messages = append(m.messages, chatMessage{
		role:    "thinking",
		content: m.currentStream.String(),
	})

	// Update viewport
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
}

func (m *ChatModel) onStreamThink(chunk string) {
	m.currentStream.WriteString(chunk)

	// Update the last streaming message
	if len(m.messages) > 0 && m.messages[len(m.messages)-1].role == "thinking" {
		m.messages[len(m.messages)-1].content = m.currentStream.String()
	}

	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
}

func (m *ChatModel) onStreamChunk(chunk string) {
	if m.thinking {
		m.currentStream.Reset()
		// Add a streaming assistant message
		m.messages = append(m.messages, chatMessage{
			role:    "assistant-streaming",
			content: "",
		})
		m.thinking = false
	}

	m.currentStream.WriteString(chunk)

	// Update the last streaming message
	if len(m.messages) > 0 && m.messages[len(m.messages)-1].role == "assistant-streaming" {
		m.messages[len(m.messages)-1].content = m.currentStream.String()
	}

	// Update viewport
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	log.Printf("UPDATED STREAM CHUNK, CURRENT: %s", m.currentStream.String())
}

func (m *ChatModel) onStreamEnded(finalContent string) {
	m.waiting = false

	// Finalize the streaming message
	if len(m.messages) > 0 && m.messages[len(m.messages)-1].role == "assistant-streaming" {
		m.messages[len(m.messages)-1].role = "assistant"
		m.messages[len(m.messages)-1].content = finalContent
	}

	// Reset current stream
	m.currentStream.Reset()

	// Update viewport
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	log.Println("we ended! final was ", finalContent)
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
		log.Println("allowing tool use for this time")
		m.out <- EventPermitToolUse(*m.pendingToolCall)
		// TODO: Execute the tool with the provided arguments
		// The tool name is: m.pendingToolCall.Name
		// The tool args are: m.pendingToolCall.Args

	case optAllowToolThisSession:
		log.Println("allowing tool use for this session")
		m.out <- EventPermitToolUseThisSession(*m.pendingToolCall)
		// TODO: Add this tool to permanently allowed tools list
		// TODO: Execute the tool with the provided arguments
		// The tool name is: m.pendingToolCall.Name
		// The tool args are: m.pendingToolCall.Args

	case optDisallowTool:
		log.Println("cancelling tool use")
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

		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit

		case tea.KeyEnter:
			if m.pendingToolCall != nil {
				return m.handleToolCallResponse()
			}

		case tea.KeyCtrlD:
			// If in tool permission mode, handle list selection ONLY
			if m.pendingToolCall != nil {
				return m.handleToolCallResponse()
			}

			// Regular message sending (only when NOT in tool permission mode)
			if m.waiting {
				return m, nil
			}

			userMsg := strings.TrimSpace(m.textarea.Value())
			if userMsg == "" {
				return m, nil
			}

			// Add user message
			m.messages = append(m.messages, chatMessage{
				role:    "user",
				content: userMsg,
			})

			// Clear textarea
			m.textarea.Reset()
			m.waiting = true
			m.currentStream.Reset()

			// Update viewport
			m.viewport.SetContent(m.renderMessages())
			m.viewport.GotoBottom()

			return m, tea.Batch(
				m.spinner.Tick,
				func() tea.Msg { m.out <- EventUserPrompt(userMsg); return nil },
				listen(m),
			)

		case tea.KeyCtrlL:
			// Clear screen (only when NOT in tool permission mode)
			if m.pendingToolCall == nil {
				m.messages = make([]chatMessage, 0)
				m.viewport.SetContent(welcomeMessage())
			}
		}

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

	case EventStreamStarted:
		m.onStreamStarted()
		return m, listen(m)

	case EventToolCall:
		m.OnToolCallReceived(msg)
		return m, listen(m)

	}

	return m, tea.Batch(taCmd, vpCmd, spCmd, listCmd)
}

func (m ChatModel) View() string {
	if m.err != nil {
		return errorStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}

	var status string
	if m.waiting {
		status = fmt.Sprintf("%s Thinking...", m.spinner.View())
	} else {
		status = "Ready"
	}

	var help string
	var inputArea string
	var viewportContent string

	// If we have a pending tool call, show the permission list instead of textarea
	if m.pendingToolCall != nil {
		help = helpStyle.Render("↑/↓: Navigate • Ctrl+D/ENTER: Select • Ctrl+C: Quit")
		inputArea = m.renderToolPermissionOptions()
		status = "Tool Permission Required"

		// Reduce viewport height to make room for the tool permission list
		// We need extra space for the list (about 5 lines)
		tempViewport := viewport.New(m.viewport.Width, m.viewport.Height-5)
		tempViewport.SetContent(m.renderMessages())
		tempViewport.GotoBottom()
		viewportContent = tempViewport.View()
	} else {
		help = helpStyle.Render("Ctrl+D: Send • Ctrl+L: Clear • Ctrl+C: Quit")
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
		switch msg.role {
		case "user":
			b.WriteString(userStyle.Render("You: "))
			b.WriteString(msg.content)
			b.WriteString("\n\n")
		case "assistant", "assistant-streaming":
			b.WriteString(assistantStyle.Render("Assistant: "))
			b.WriteString(msg.content)
			if msg.role == "assistant-streaming" {
				b.WriteString(cursorStyle.Render("▋"))
			}
			b.WriteString("\n\n")
		case "system":
			b.WriteString(systemStyle.Render("System: "))
			b.WriteString(msg.content)
			b.WriteString("\n\n")
		case "thinking":
			b.WriteString(thinkingStyle.Render(msg.content))
			b.WriteString("\n\n")
		}
	}

	return b.String()
}

func welcomeMessage() string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("34")).
		Render(`
		
                      ███      █████████  █████         █████████   ███████   ███                     
                     ░███     ███░░░░░███░░███         ███░░░░░███ ░░░███    ░███                     
            █████████░███    ███     ░░░  ░███        ░███    ░███   ░███    ░███ █████████           
 ██████████░░░░░░░░░ ░███   ░███          ░███        ░███████████   ░███    ░███░░░░░░░░░  ██████████
░░░░░░░░░░  █████████░███   ░███          ░███        ░███░░░░░███   ░███    ░███ █████████░░░░░░░░░░ 
           ░░░░░░░░░ ░███   ░░███     ███ ░███      █ ░███    ░███   ░███    ░███░░░░░░░░░            
                     ░███    ░░█████████  ███████████ █████   █████ ███████  ░███                     
                     ░░░      ░░░░░░░░░  ░░░░░░░░░░░ ░░░░░   ░░░░░ ░░░░░░░   ░░░                      
                                                                                   
`)

}

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			MarginBottom(1)

	userStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true)

	assistantStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212")).
			Bold(true)

	systemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("21")).
			Bold(true)

	cursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	thinkingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)
)

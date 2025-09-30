package ui

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/penguinpowernz/clai/internal/ai"
)

type EventStreamStarted string
type EventStreamEnded string
type EventStreamChunk string
type EventToolCall ai.ToolCall
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
	err           error
	width         int
	height        int
	currentStream *strings.Builder
	in, out       chan any
}

type chatMessage struct {
	role    string
	content string
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

	vp := viewport.New(80, 20)
	vp.SetContent(welcomeMessage())

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	model := ChatModel{
		ctx:           ctx,
		textarea:      ta,
		viewport:      vp,
		spinner:       sp,
		messages:      make([]chatMessage, 0),
		currentStream: &strings.Builder{},
		in:            make(chan any),
		out:           make(chan any),
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

func (m *ChatModel) OnToolCallReceived(toolCall *ai.ToolCall) {
	// Log tool call
	log.Println("Tool call received in UI:", toolCall.Name)

	// Add a system message about the tool call
	m.messages = append(m.messages, chatMessage{
		role:    "system",
		content: fmt.Sprintf("LLM attempted to use tool: %s", toolCall.Name),
	})

	// Update viewport
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
}

func (m *ChatModel) onStreamStarted(firstChunk string) {
	m.waiting = true
	m.currentStream.Reset()
	m.currentStream.WriteString(firstChunk)

	// Add a streaming assistant message
	m.messages = append(m.messages, chatMessage{
		role:    "assistant-streaming",
		content: m.currentStream.String(),
	})

	// Update viewport
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	log.Println("STREAM STARTED")
}

func (m *ChatModel) onStreamChunk(chunk string) {
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

func (m ChatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		taCmd tea.Cmd
		vpCmd tea.Cmd
		spCmd tea.Cmd
	)

	m.textarea, taCmd = m.textarea.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)
	m.spinner, spCmd = m.spinner.Update(msg)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Resize components
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 6 // Leave room for textarea and borders
		m.textarea.SetWidth(msg.Width - 4)

		// Re-render messages with new width
		m.viewport.SetContent(m.renderMessages())

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit

		case tea.KeyCtrlD:
			// Send message
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
			// Clear screen
			m.messages = make([]chatMessage, 0)
			m.viewport.SetContent(welcomeMessage())
		}

	case EventStreamChunk:
		m.onStreamChunk(string(msg))
		return m, listen(m)

	case EventSystemMsg:
		m.onSystemMessage(string(msg))

	// case newToolCall:
	// 	m.OnToolCallReceived(&msg.toolCall)

	// case newErrorMessage:
	// 	m.OnStreamError(errors.New(msg.err))

	case EventStreamEnded:
		m.onStreamEnded(string(msg))
		return m, listen(m)

	case EventStreamStarted:
		m.onStreamStarted(string(msg))
		return m, listen(m)

	}

	return m, tea.Batch(taCmd, vpCmd, spCmd)
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

	help := helpStyle.Render("Ctrl+D: Send • Ctrl+L: Clear • Ctrl+C: Quit")

	return fmt.Sprintf(
		"%s\n\n%s\n\n%s\n%s",
		titleStyle.Render("AI Code Assistant"),
		m.viewport.View(),
		m.textarea.View(),
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
		}
	}
	return b.String()
}

func welcomeMessage() string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render("Welcome! Type your message and press Ctrl+D to send.")
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

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)
)

package chat

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/penguinpowernz/aichat/config"
	"github.com/penguinpowernz/aichat/internal/ai"
	"github.com/penguinpowernz/aichat/internal/files"
	"github.com/penguinpowernz/aichat/internal/tools"
)

// Session manages the conversation state
type Session struct {
	config     *config.Config
	client     ai.AIProvider
	messages   []ai.Message
	files      *files.Context
	workingDir string
	tools      []tools.Tool
}

func NewSession(cfg *config.Config, client ai.AIProvider) *Session {
	wd, _ := os.Getwd()

	return &Session{
		config:     cfg,
		client:     client,
		messages:   make([]ai.Message, 0),
		files:      files.NewContext(cfg),
		workingDir: wd,
		tools:      tools.GetAvailableTools(),
	}
}

// ProcessToolUses handles tool calls from the AI
func (s *Session) ProcessToolUses(toolUses []tools.ToolUse) []tools.ToolResult {
	results := make([]tools.ToolResult, len(toolUses))
	for i, toolUse := range toolUses {
		results[i] = tools.ExecuteTool(s.config, toolUse, s.workingDir)
	}
	return results
}

// InteractiveMode starts the bubbletea REPL
func (s *Session) InteractiveMode(ctx context.Context, terminal interface{}) error {
	model := newChatModel(s, ctx)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running interactive mode: %w", err)
	}

	return nil
}

// SendMessage sends a single message (non-interactive)
func (s *Session) SendMessage(ctx context.Context, terminal interface{}, message string) error {
	s.messages = append(s.messages, ai.Message{
		Role:    "user",
		Content: message,
	})

	// Stream the response
	stream, err := s.client.StreamMessage(ctx, s.messages)
	if err != nil {
		return err
	}

	var response strings.Builder
	for chunk := range stream {
		fmt.Print(chunk)
		response.WriteString(chunk)
	}
	fmt.Println()

	s.messages = append(s.messages, ai.Message{
		Role:    "assistant",
		Content: response.String(),
	})

	return nil
}

// chatModel is the bubbletea model for the REPL
type chatModel struct {
	session       *Session
	ctx           context.Context
	viewport      viewport.Model
	textarea      textarea.Model
	spinner       spinner.Model
	messages      []chatMessage
	waiting       bool
	err           error
	width         int
	height        int
	currentStream strings.Builder
	streamChan    <-chan string // Keep reference to the stream channel
}

type chatMessage struct {
	role    string
	content string
}

// Messages for bubbletea updates
type streamChunkMsg string
type streamDoneMsg struct{} // Remove content field
type streamErrMsg struct{ err error }

func newChatModel(session *Session, ctx context.Context) chatModel {
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

	return chatModel{
		session:  session,
		ctx:      ctx,
		textarea: ta,
		viewport: vp,
		spinner:  sp,
		messages: make([]chatMessage, 0),
	}
}

func (m chatModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			m.session.messages = append(m.session.messages, ai.Message{
				Role:    "user",
				Content: userMsg,
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
				m.startStreaming(), // Start streaming - creates the channel
			)

		case tea.KeyCtrlL:
			// Clear screen
			m.messages = make([]chatMessage, 0)
			m.session.messages = make([]ai.Message, 0)
			m.viewport.SetContent(welcomeMessage())
		}

	case streamStartMsg:
		// Store the stream channel and process first chunk
		m.streamChan = msg.stream
		m.currentStream.WriteString(msg.firstChunk)

		m.messages = append(m.messages, chatMessage{
			role:    "assistant-streaming",
			content: m.currentStream.String(),
		})

		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

		// Start reading next chunks
		return m, m.readNextChunk()

	case streamChunkMsg:
		// Append chunk to current stream
		m.currentStream.WriteString(string(msg))

		// Update the last message (or create new one)
		if len(m.messages) > 0 && m.messages[len(m.messages)-1].role == "assistant-streaming" {
			m.messages[len(m.messages)-1].content = m.currentStream.String()
		} else {
			m.messages = append(m.messages, chatMessage{
				role:    "assistant-streaming",
				content: m.currentStream.String(),
			})
		}

		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

		// Continue reading from the SAME stream
		return m, m.readNextChunk()

	case streamDoneMsg:
		// Finalize the assistant message
		if len(m.messages) > 0 && m.messages[len(m.messages)-1].role == "assistant-streaming" {
			m.messages[len(m.messages)-1].role = "assistant"
		}

		m.session.messages = append(m.session.messages, ai.Message{
			Role:    "assistant",
			Content: m.currentStream.String(), // Use m.currentStream instead of msg.content
		})

		m.waiting = false
		m.currentStream.Reset() // Reset for next message
		m.streamChan = nil      // Clear the channel reference
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

	case streamErrMsg:
		m.err = msg.err
		m.waiting = false
		m.viewport.SetContent(m.renderMessages())
	}

	return m, tea.Batch(taCmd, vpCmd, spCmd)
}

func (m chatModel) View() string {
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

// startStreaming initiates a new stream
func (m chatModel) startStreaming() tea.Cmd {
	return func() tea.Msg {
		stream, err := m.session.client.StreamMessage(m.ctx, m.session.messages)
		if err != nil {
			return streamErrMsg{err: err}
		}

		// Store the stream channel in the model
		// This happens in a goroutine, but we'll handle it via messages

		// Read first chunk
		chunk, ok := <-stream
		if !ok {
			return streamDoneMsg{} // Stream ended immediately
		}

		// We need to pass the stream to subsequent reads
		// Store it via a message that includes the channel
		return streamStartMsg{stream: stream, firstChunk: string(chunk)}
	}
}

// streamStartMsg carries the stream channel and first chunk
type streamStartMsg struct {
	stream     <-chan string
	firstChunk string
}

// readNextChunk reads the next chunk from the stored stream
func (m chatModel) readNextChunk() tea.Cmd {
	stream := m.streamChan // Capture the stream
	return func() tea.Msg {
		chunk, ok := <-stream
		if !ok {
			return streamDoneMsg{} // Stream ended
		}
		return streamChunkMsg(chunk)
	}
}

func (m chatModel) renderMessages() string {
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

	cursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)
)

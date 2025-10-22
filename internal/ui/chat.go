package ui

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"github.com/penguinpowernz/clai/config"
	"github.com/penguinpowernz/clai/internal/ai"
	"github.com/penguinpowernz/clai/internal/history"
)

const (
	optAllowToolThisTime    = "Allow to run this time only"
	optAllowToolThisSession = "Allow, and don't ask again this session"
	optDisallowTool         = "Don't allow to run the tool, give the prompt back"

	maxLineLength = 120
)

type UIObserver interface {
	Observe(chan any)
}

// ChatModel is the bubbletea model for the REPL
type ChatModel struct {
	ctx           context.Context
	cfg           *config.Config
	prompt        textinput.Model
	viewport      viewport.Model
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

func NewChatModel(ctx context.Context, cfg *config.Config) *ChatModel {
	ti := textinput.New()
	// ti.Placeholder = "Type your message..."
	ti.Focus()
	ti.Prompt = ""
	ti.Placeholder = "Type your message..."
	ti.CharLimit = 0
	ti.Width = 80
	ti.PromptStyle.Background(lipgloss.Color("235"))

	// ti.PromptStyle
	// ti.FocusedStyle.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("34"))
	// ti.BlurredStyle.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("34"))

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	vp := viewport.New(80, 20)
	vp.MouseWheelEnabled = true

	model := ChatModel{
		height:                20,
		width:                 80,
		ctx:                   ctx,
		cfg:                   cfg,
		prompt:                ti,
		spinner:               sp,
		viewport:              vp,
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

	if m.cfg.SaveHistory {
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

func (m ChatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		taCmd   tea.Cmd
		spCmd   tea.Cmd
		listCmd tea.Cmd
	)

	// Only update textarea if we're not in tool permission mode
	if m.pendingToolCall == nil {
		m.prompt, taCmd = m.prompt.Update(msg)
	}
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
		m.viewport.Height = msg.Height - 1 // Leave room for textarea and borders
		m.prompt.Width = msg.Width - 4

		// Re-render messages with new width
		if m.pendingToolCall != nil {
			m.toolPermissionList.SetWidth(msg.Width - 4)
			m.toolPermissionList.SetHeight(5)
		}
		// m.viewport.SetContent(m.renderMessages() + "\n" + m.toolPermissionList.View())

	case tea.KeyMsg:
		return m.handleKeyPress(msg)

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
		return m, textinput.Blink

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

	// log.Printf("[ui] Unhandled message: %T", msg)
	return m, tea.Batch(taCmd, spCmd, listCmd)
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
	var viewportContent = m.renderMessages()

	// If we have a pending tool call, show the permission list instead of textarea
	if m.pendingToolCall != nil {
		help = helpStyle.Render("↑/↓: Navigate • ENTER: Select • Ctrl+C: Quit")
		inputArea = m.renderToolPermissionOptions()
		status = "👮 Tool Permission Required"

		// Reduce viewport height to make room for the tool permission list
		// We need extra space for the list (about 5 lines)
		// tempViewport := viewport.New(m.viewport.Width, m.viewport.Height-5)
		// tempViewport.SetContent(m.renderMessages())
		// tempViewport.GotoBottom()
		// viewportContent = tempViewport.View()
	} else {
		help = helpStyle.Render("ENTER: Send • Ctrl+L: Clear • Ctrl+C: Quit • ESC: Stop AI")
		inputArea = m.prompt.View()
		// viewportContent = m.viewport.View()
	}

	m.viewport.SetContent(fmt.Sprintf(
		"%s\n%s\n\n%s %s\n\n%s",
		welcomeMessage(),
		viewportContent,
		userStyle.Render("\u2588"),
		inputArea,
		lipgloss.JoinHorizontal(lipgloss.Left, status, "  ", help),
	))

	m.viewport.GotoBottom()
	return m.viewport.View()
}

func (m ChatModel) renderMessages() string {
	if len(m.messages) == 0 {
		return ""
	}

	var b strings.Builder
	for _, msg := range m.messages {
		switch msg.Role {
		case "user":
			b.WriteString(userStyle.Render("\u2588 "))
			b.WriteString(msg.Content)
			b.WriteString("\n\n")
		case "assistant", "assistant-streaming":
			b.WriteString(msg.Content)
			if msg.Role == "assistant-streaming" {
				b.WriteString(cursorStyle.Render("▋"))
			}
			b.WriteString("\n")
		case "system":
			b.WriteString(systemStyle.Render(msg.Content))
			b.WriteString("\n\n")
		case "tool":
			b.WriteString(toolStyle.Render(msg.Content))
			b.WriteString("\n\n")
		case "slashcmd":
			b.WriteString(systemStyle.Render(msg.Content))
			b.WriteString("\n\n")
		case "thinking":
			if m.cfg.ShowThinking {
				b.WriteString(thinkingStyle.Render(msg.Content))
				b.WriteString("\n\n")
			}
		}
	}

	return wordwrap.String(b.String(), min(m.width, maxLineLength))
}

func welcomeMessage() string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("34")).
		Render(`
		
        █████████  █████         █████████   ███████ 
       ███░░░░░███░░███         ███░░░░░███ ░░░███   
      ███     ░░░  ░███        ░███    ░███   ░███   
     ░███          ░███        ░███████████   ░███   
     ░███          ░███        ░███░░░░░███   ░███   
     ░░███     ███ ░███      █ ░███    ░███   ░███   
      ░░█████████  ███████████ █████   █████ ███████ 
       ░░░░░░░░░  ░░░░░░░░░░░ ░░░░░   ░░░░░ ░░░░░░░  
			 
`)

}

// Styles
var (
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

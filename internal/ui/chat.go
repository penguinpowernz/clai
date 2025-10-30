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
	maxLineLength = 120

	titleSelectModel = "Select the model to use"
)

type UIObserver interface {
	Observe(chan any)
}

// ChatModel is the bubbletea model for the REPL
type ChatModel struct {
	ctx           context.Context
	cfg           *config.Config
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
	prompt Prompt
	userIsScrolling bool

	// Tool permission selection
	pendingToolCall       *ai.ToolCall
	toolPermissionList    list.Model
	toolPermissionOptions []string
	selectedOption        int
}

func NewChatModel(ctx context.Context, cfg *config.Config) *ChatModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	vp := viewport.New(80, 20)
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = 3

	ti := NewPrompt()

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
		prompt:             ti,
	}

	return &model
}

func (m *ChatModel) addMessage(role, msg string) {
	m.messages = append(m.messages, ai.Message{
		Role:    role,
		Content: msg,
	})

	m.viewport.SetContent(m.renderMessages())

	if !m.userIsScrolling {
		m.viewport.GotoBottom()
	}

	if m.cfg.SaveHistory {
		if err := history.SaveHistory("ui", m.messages); err != nil {
			log.Println("[ui] Error saving history:", err)
		}
	}
}

func (m *ChatModel) updateMessage(role, chunk string) {
	m.currentStream.WriteString(chunk)

	// Update the last streaming message
	if len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == role {
		m.messages[len(m.messages)-1].Content = m.currentStream.String()
	}
	m.viewport.SetContent(m.renderMessages())
	if !m.userIsScrolling {
		m.viewport.GotoBottom()
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
		cmds                              []tea.Cmd
		cmd, taCmd, spCmd, listCmd, vpCmd tea.Cmd
	)

	if m.currList != nil {
		m.currList, cmd = m.currList.Update(msg)
		if cmd != nil {
			log.Println("[ui.update] setting lcmd")
			cmds = append(cmds, cmd)
			//			return nil, cmd
		}
	}

	// Only update textarea if we're not in tool permission mode
	if m.pendingToolCall == nil {
		m.prompt, taCmd = m.prompt.Update(msg)
		cmds = append(cmds, taCmd)
	}
	m.spinner, spCmd = m.spinner.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, spCmd, vpCmd)

	// Don't update the old list component when in tool permission mode
	if m.pendingToolCall == nil {
		m.toolPermissionList, listCmd = m.toolPermissionList.Update(msg)
		cmds = append(cmds, listCmd)
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
		m.prompt.SetWidth(msg.Width - 4)

		// Re-render messages with new width
		if m.pendingToolCall != nil {
			m.toolPermissionList.SetWidth(msg.Width - 4)
			m.toolPermissionList.SetHeight(5)
		}
		// m.viewport.SetContent(m.renderMessages() + "\n" + m.toolPermissionList.View())
		m.viewport.SetContent(m.renderMessages())

	case tea.MouseMsg:
		if msg.Button == tea.MouseButtonWheelUp {
			m.userIsScrolling = true
		}

		if msg.Button == tea.MouseButtonWheelDown && m.viewport.AtBottom() {
			m.userIsScrolling = false
		}

	case tea.KeyMsg:
		if m.currList == nil {
			return m.handleKeyPress(msg)
		}

	case EventModelSelection:
		l := NewSimpleList(titleSelectModel, msg...)
		m.currList = l

	case EventListDone:
		log.Printf("[ui.event] list done %+v", msg)
		switch msg.title {
		case titleSelectModel:
			cmds = append(cmds, func() tea.Msg { m.out <- EventModelSelected(msg.option); return nil })
		}
		m.currList = nil
		m.prompt.Reset()

	case EventSlashCommand:
		return m.handleSlashCommand(msg)

	case EventExit:
		return m, tea.Quit

	case EventStreamChunk:
		m.onStreamChunk(string(msg))
		cmds = append(cmds, listen(m))

	case EventSystemMsg:
		m.onSystemMessage(string(msg))

	case EventStreamEnded:
		m.onStreamEnded(string(msg))
		return m, textinput.Blink

	case EventStreamThink:
		m.onStreamThink(string(msg))
		cmds = append(cmds, listen(m))

	case EventStreamCancelled:
		m.onStreamCancelled()
		return m, listen(m)

	case EventStreamStarted:
		m.onStreamStarted()
		cmds = append(cmds, listen(m))

	case EventToolCall:
		m.OnToolCallReceived(msg)
		return m, listen(m)

	case EventAssistantMessage:
		m.onAssistantMessage(string(msg))
		return m, listen(m)

	case EventClear:
		m.onClear()
		cmds = append(cmds, listen(m))

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
	return m, tea.Batch(cmds...)
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
	var viewportContent = m.viewport.View()

	switch {
	// If we have a pending tool call, show the permission list instead of textarea
	case m.pendingToolCall != nil:
		help = helpStyle.Render("↑/↓: Navigate • ENTER: Select • Ctrl+C: Quit")
		inputArea = m.renderToolPermissionOptions()
		status = "👮 Tool Permission Required"

		// Reduce viewport height to make room for the tool permission list
		// We need extra space for the list (about 5 lines)
		// tempViewport := viewport.New(m.viewport.Width, m.viewport.Height-5)
		// tempViewport.SetContent(m.renderMessages())
		// tempViewport.GotoBottom()
		// viewportContent = tempViewport.View()
	case m.currList != nil:

		help = helpStyle.Render("↑/↓: Navigate • ENTER: Select • Ctrl+C: Quit")
		inputArea = m.currList.View()
		status = "Selection Required"
	default:
		help = helpStyle.Render("ENTER: Send • Ctrl+C: Quit • ESC: Stop AI")
		inputArea = m.prompt.View()
	}

	var x []string
	var nlcount int
	lines := strings.Split(viewportContent, "\n")
	if len(lines) <= m.viewport.Height {

		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				nlcount++
				continue
			}

			if nlcount > 0 {
				x = append(x, strings.Repeat("\n", nlcount))
				nlcount = 0
			}

			x = append(x, line)
		}

		diff := len(lines) - len(x)
		viewportContent = strings.Repeat("\n", diff) + strings.Join(x, "\n")
	}

	return fmt.Sprintf(
		"%s\n\n%s\n\n%s",
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
	b.WriteString(welcomeMessage())

	for _, msg := range m.messages {
		switch msg.Role {
		case "user":
			b.WriteString("\n\n")
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

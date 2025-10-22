package ui

import (
	"log"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/penguinpowernz/clai/internal/ai"
	"github.com/penguinpowernz/clai/internal/commands"
)

func (m *ChatModel) onSystemMessage(msg string) {
	// Add system message to chat messages
	m.addMessage("system", msg)

}

func (m *ChatModel) onStreamStarted() {
	log.Println("[ui] STREAM STARTED")
	m.typing = false
	m.currentStream.Reset()

	m.thinking = true
	m.addMessage("thinking", m.currentStream.String())

}

func (m *ChatModel) onStreamThink(chunk string) {
	if chunk == "</think>" || chunk == "<think>" {
		return
	}
	m.currentStream.WriteString(chunk)

	// Update the last streaming message
	if len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == "thinking" {
		m.messages[len(m.messages)-1].Content = m.currentStream.String()
	}

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

	log.Println("[ui] we ended! final was ", finalContent)
}

func (m *ChatModel) onAssistantMessage(msg string) {
	// Add assistant message to chat messages
	m.addMessage("assistant", msg)

}

func (m ChatModel) Init() tea.Cmd {
	// No need to manually set system message handler anymore
	return textinput.Blink
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
	m.prompt.Focus()

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

	userMsg := strings.TrimSpace(m.prompt.Value())
	if userMsg == "" {
		return m, nil
	}

	// Add user message
	m.addMessage("user", userMsg)

	// Clear textarea
	m.prompt.Reset()

	if userMsg[0] != '/' {
		m.thinking = true
	}

	m.currentStream.Reset()

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

	return m, listen(m)
}

func (m *ChatModel) onStreamCancelled() {
	log.Println("[ui] STREAM CANCELLED")
	m.typing = false
	m.thinking = false
	m.inThinkBlock = false
}

func (m *ChatModel) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

		return m.handleSubmit()

	case tea.KeyCtrlD:
		return m.handleSubmit()

	case tea.KeyCtrlL:
		// Clear screen (only when NOT in tool permission mode)
		if m.pendingToolCall == nil {
			m.messages = make([]ai.Message, 0)
			welcomeMessage()
		}
	}
	return m, nil
}

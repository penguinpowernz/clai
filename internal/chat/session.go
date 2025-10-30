package chat

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/penguinpowernz/clai/config"
	"github.com/penguinpowernz/clai/internal/ai"
	"github.com/penguinpowernz/clai/internal/commands"
	"github.com/penguinpowernz/clai/internal/files"
	"github.com/penguinpowernz/clai/internal/history"
	"github.com/penguinpowernz/clai/internal/tools"
	"github.com/penguinpowernz/clai/internal/ui"
)

// UIObserver defines the interface for UI elements to receive updates from the session
type UIObserver interface {
	Observe(chan any)
}

// Session manages the conversation state
type Session struct {
	id         string
	config     *config.Config
	client     ai.Provider
	messages   []ai.Message
	files      *files.Context
	workingDir string
	tools      []tools.Tool
	mu         sync.Mutex
	currStrm   *Stream

	permitToolCall chan bool
	permittedTools map[string]bool
	toolCalls      chan *ai.ToolCall

	events   chan any // events going out to the UI
	uievents chan any // events coming in from the UI
}

// AddObserver registers a new UI observer
func (s *Session) AddObserver(observer UIObserver) {
	observer.Observe(s.events)
}

func (s *Session) Export() []ai.Message {
	return s.messages
}

func (s *Session) ClearMessages() {
	s.messages = make([]ai.Message, 0)
	s.uievents <- ui.EventClear{}
}

func (s *Session) GetClient() ai.Provider {
	return s.client
}

func NewSession(cfg *config.Config, client ai.Provider, id string) *Session {
	wd, _ := os.Getwd()
	tt := tools.GetAvailableTools()
	client.SetTools(tt)

	pt := make(map[string]bool)
	for _, t := range cfg.PermittedTools {
		pt[t] = true
	}

	return &Session{
		id:             id,
		config:         cfg,
		client:         client,
		messages:       make([]ai.Message, 0),
		files:          files.NewContext(cfg),
		workingDir:     wd,
		tools:          tt,
		events:         make(chan any, 2),
		uievents:       make(chan any, 2),
		mu:             sync.Mutex{},
		permittedTools: pt,
		permitToolCall: make(chan bool, 2),
		toolCalls:      make(chan *ai.ToolCall, 2),
	}
}

func (s *Session) AddMessage(message ai.Message) {
	s.messages = append(s.messages, message)
	if s.config.SaveHistory {
		if err := history.SaveHistory("context", s.messages); err != nil {
			log.Println("[session] failed to save history:", err)
		}
	}
}

// InteractiveMode starts the bubbletea REPL
func (s *Session) InteractiveMode(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil

		case tc := <-s.toolCalls:
			go s.handleToolCall(ctx, tc)

		case ev := <-s.uievents:
			log.Println("[session] got UI event")
			s.handleUIEvent(ctx, ev)

		}
	}
}

func (s *Session) handleToolCall(ctx context.Context, tc *ai.ToolCall) {
	log.Print("[session] handling tool call for tool: ", tc.Name)

	if !tools.IsValid(s.tools, tc.Name) {
		log.Println("[session] Tool not found:", tc.Name)
		s.AddMessage(ai.Message{
			Role:       "user",
			Content:    "Tool not found: `" + tc.Name + "`, available tools are: " + strings.Join(tools.GetNames(s.tools), ", "),
			ToolCallID: tc.ID,
		})
		if err := s.sendFullContext(ctx); err != nil {
			log.Println("[session] failed to send full context:", err)
		}
		return
	}

	// Check if the tool is permitted, otherwise request permission from UI
	if _, permitted := s.permittedTools[tc.Name]; !permitted {
		log.Println("[session] Requesting permission for tool:", tc.Name)
		s.events <- ui.EventToolCall(*tc)
		log.Println("[session] Waiting for tool call permission...")
		if ok := <-s.permitToolCall; !ok {
			log.Println("[session] Permission denied by UI to call tool:", tc.Name)
			return
		}
	}

	s.events <- ui.EventRunningTool(*tc)
	log.Println("[session] Permission granted to call tool:", tc.Name)
	output := s.executeTool(tc)
	s.events <- ui.EventRunningToolDone("")
	s.events <- ui.EventToolOutput(output)
	s.respondWithToolOutput(ctx, tc.ID, output)
}

func (s *Session) handleCommand(ctx context.Context, cmd string) {
	log.Println("[session] handling command:", cmd)

	// if we get the models command, prepare a list of models to send to the user for selection
	if strings.HasPrefix(cmd, "/models") {
		models := s.client.ListModels()
		for i, name := range models {
			name = strings.Split(name, " ")[0]
			models[i] = name

			if name == s.config.Model {
				models[i] = "*" + name
			}
		}
		s.events <- ui.EventModelSelection(models)
		return
	}

	res, err := commands.DefaultRegistry.Execute(ctx, cmd, &commands.Environment{
		Session:    s,
		Files:      s.files,
		Config:     s.config,
		WorkingDir: s.workingDir,
	})

	if err != nil {
		log.Println("[session] failed to execute command:", err)
		return
	}

	s.events <- ui.EventSlashCommand(*res)
}

func (s Session) Context() (system any, input []any, output []any) {
	system = map[string]any{
		"role":    "system",
		"content": s.config.SystemPrompt,
	}

	for _, msg := range s.messages {
		switch msg.Role {
		case "assistant":
			output = append(output, msg)
		default:
			input = append(input, msg)
		}
	}

	return
}

func (s *Session) handleUIEvent(ctx context.Context, ev any) {
	switch msg := ev.(type) {
	case ui.EventUserPrompt:
		if string(msg)[0] == '/' {
			s.handleCommand(ctx, string(msg))
		} else {
			s.SendMessage(ctx, string(msg))
		}

	case ui.EventCancelStream:
		log.Println("[session] Canceling stream...")
		s.currStrm.Close()
		s.currStrm.Wait()
		log.Println("[session] stream cancelled")
		s.events <- ui.EventStreamCancelled{}

	case ui.EventPermitToolUse:
		log.Printf("[session] Tool permission granted for: %s", msg.Name)
		s.permitToolCall <- true // tell the stream loop to continue
		log.Printf("[session] told stream loop to continue")

	case ui.EventPermitToolUseThisSession:
		log.Printf("[session] Tool permission granted for this session: %s\n", msg.Name)
		s.permittedTools[msg.Name] = true
		s.permitToolCall <- true // tell the stream loop to continue
		log.Printf("[session] told stream loop to continue")

	case ui.EventCancelToolUse:
		log.Printf("[session] Tool use cancelled for: %s\n", msg.Name)
		s.permitToolCall <- false // tell the stream loop to continue
		log.Printf("[session] told stream loop to continue")

	case ui.EventModelSelected:
		model := string(msg)
		if !strings.Contains(model, "*") {
			s.config.Model = model
			s.events <- ui.EventSystemMsg("Model changed to " + model)
		}

	default:
		log.Printf("[session] Unknown UI event: %T %+v", ev, ev)
	}
}

func (s *Session) executeTool(tool *ai.ToolCall) string {
	result := tools.ExecuteTool(s.config, tools.ToolUse(*tool), s.workingDir)
	return result.Content
}

func (s *Session) respondWithToolOutput(ctx context.Context, toolUseID string, output string) {
	log.Printf("[session] responding to tool call: %s with output %s", toolUseID, output)

	s.AddMessage(ai.Message{
		Role:       "tool",
		Content:    output,
		ToolCallID: toolUseID,
	})

	s.sendFullContext(ctx)
}

func (s *Session) Observe(events chan any) {
	s.uievents = events
}

// SendMessage add a new user message to the conversation and then sends the
// fulll context to the LLM
func (s *Session) SendMessage(ctx context.Context, message string) error {
	message = enhanceMessage(s.config, message)

	// Add user message to conversation
	s.AddMessage(ai.Message{
		Role:    "user",
		Content: message,
	})

	return s.sendFullContext(ctx)
}

func (s *Session) handleStreamChunk(chunk ai.MessageChunk) {
	switch chunk.Type() {
	case ai.ChunkMessage:
		s.events <- ui.EventStreamChunk(chunk.String())
	case ai.ChunkThink:
		s.events <- ui.EventStreamThink(chunk.String())
	}
}

// sendFullContext sends a full conversation context to the LLM, using streaming
func (s *Session) sendFullContext(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	strm := NewStream(s.client)
	s.currStrm = strm
	strm.OnChunk(s.handleStreamChunk)

	strm.OnStart(func() {
		log.Println("[session] stream started")
		s.events <- ui.EventStreamStarted("")
	})

	strm.OnEnd(func(msg string) {
		log.Println("[session] stream ended")
		s.events <- ui.EventStreamEnded(msg)
	})

	log.Println("[session] starting stream")
	strm.Start(ctx, s.messages)

	strm.Wait()
	log.Println("[session] stream is done")

	if strm.Content() != "" {
		log.Println("[session] stream ended with content, updating conversation")

		// Add assistant message
		s.AddMessage(ai.Message{
			Role:    "assistant",
			Content: strm.Content(),
		})
	}

	if tc := strm.ToolCall(); tc != nil {
		s.AddMessage(ai.Message{
			Role:       "assistant",
			Content:    "Request to use tool: `" + tc.Name + "` with args: `" + string(tc.Input) + "`",
			ToolCallID: tc.ID,
		})

		log.Println("[session] stream ended with tool call, passing it off")
		s.toolCalls <- tc
	}

	return nil
}

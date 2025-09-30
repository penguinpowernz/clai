package chat

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/penguinpowernz/clai/config"
	"github.com/penguinpowernz/clai/internal/ai"
	"github.com/penguinpowernz/clai/internal/files"
	"github.com/penguinpowernz/clai/internal/tools"
	"github.com/penguinpowernz/clai/internal/ui"
)

// UIObserver defines the interface for UI elements to receive updates from the session
type UIObserver interface {
	Observe(chan any)
}

// Session manages the conversation state
type Session struct {
	config     *config.Config
	client     ai.Provider
	messages   []ai.Message
	files      *files.Context
	workingDir string
	tools      []tools.Tool
	mu         sync.Mutex

	permitToolCall chan bool
	permittedTools map[string]bool

	// Streaming-related fields
	currentStream    <-chan ai.MessageChunk
	streamContext    context.Context
	streamFirstChunk ai.MessageChunk
	streamStarted    bool

	events   chan any // events going out to the UI
	uievents chan any // events coming in from the UI
}

// AddObserver registers a new UI observer
func (s *Session) AddObserver(observer UIObserver) {
	observer.Observe(s.events)
}

func (s *Session) ClearMessages() {
	s.messages = make([]ai.Message, 0)
}

func (s *Session) GetClient() ai.Provider {
	return s.client
}

func NewSession(cfg *config.Config, client ai.Provider) *Session {
	wd, _ := os.Getwd()
	tt := tools.GetAvailableTools()
	client.SetTools(tt)
	return &Session{
		config:         cfg,
		client:         client,
		messages:       make([]ai.Message, 0),
		files:          files.NewContext(cfg),
		workingDir:     wd,
		tools:          tt,
		events:         make(chan any),
		uievents:       make(chan any),
		mu:             sync.Mutex{},
		permittedTools: map[string]bool{},
		permitToolCall: make(chan bool),
	}
}

// InteractiveMode starts the bubbletea REPL
func (s *Session) InteractiveMode(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil

		case ev := <-s.uievents:
			switch msg := ev.(type) {
			case ui.EventUserPrompt:
				s.messages = append(s.messages, ai.Message{
					Role:    "user",
					Content: string(msg),
				})

				go s.SendMessage(ctx, string(msg))

			case ui.EventPermitToolUse:
				log.Printf("Tool permission granted for: %s", msg.Name)
				s.permitToolCall <- true // tell the stream loop to continue
				log.Printf("told stream loop to continue")

			case ui.EventPermitToolUseThisSession:
				log.Printf("Tool permission granted for this session: %s\n", msg.Name)
				s.permittedTools[msg.Name] = true
				s.permitToolCall <- true // tell the stream loop to continue
				log.Printf("told stream loop to continue")

			case ui.EventCancelToolUse:
				log.Printf("Tool use cancelled for: %s\n", msg.Name)
				s.permitToolCall <- false // tell the stream loop to continue
				log.Printf("told stream loop to continue")

			default:
				log.Printf("Unknown UI event: %T %+v", ev, ev)
			}
		}
	}
}

func (s *Session) executeTool(tool *ai.ToolCall) string {
	result := tools.ExecuteTool(s.config, tools.ToolUse(*tool), s.workingDir)
	return result.Content
}

func (s *Session) respondWithToolOutput(ctx context.Context, toolUseID string, output string) {
	log.Printf("responding to tool call: %s with output %s", toolUseID, output)

	s.messages = append(s.messages, ai.Message{
		Role:       "user",
		Content:    output,
		ToolCallID: toolUseID,
	})

	s.sendFullContext(ctx)
}

// StartStream begins a new message stream
func (s *Session) StartStream(ctx context.Context) (string, error) {
	// Reset streaming state
	s.streamStarted = false
	s.currentStream = nil
	s.streamFirstChunk = ai.MessageChunk{}
	s.streamContext = ctx

	// Start streaming
	stream, err := s.client.StreamMessage(ctx, s.messages)
	if err != nil {
		return "", err
	}

	s.events <- ui.EventStreamStarted("")

	var response strings.Builder
loop:
	for chunk := range stream {
		switch chunk.Type() {
		case ai.ChunkToolCall:

			// Check if the tool is permitted, otherwise request permission from UI
			if _, permitted := s.permittedTools[chunk.ToolCall.Name]; !permitted {
				log.Println("Requesting permission for tool:", chunk.ToolCall.Name)
				s.events <- ui.EventToolCall(*chunk.ToolCall)
				log.Println("Waiting for tool call permission...")
				if ok := <-s.permitToolCall; !ok {
					continue
				}
			}

			log.Println("Permission granted to call tool:", chunk.ToolCall.Name)
			output := s.executeTool(chunk.ToolCall)
			s.respondWithToolOutput(ctx, chunk.ToolCall.ID, output)
			break loop

		case ai.ChunkMessage:
			s.events <- ui.EventStreamChunk(chunk.String())
			response.WriteString(chunk.String())

		case ai.ChunkThink:
			s.events <- ui.EventStreamThink(chunk.String())
		default:
			log.Println("Unknown chunk type:", chunk.Type())
		}
	}

	log.Println("stream loop ended")

	// Store stream for subsequent processing
	s.currentStream = stream
	s.streamStarted = true

	return response.String(), nil
}

func (s *Session) Observe(events chan any) {
	s.uievents = events
}

// EndStream finalizes the current stream
func (s *Session) EndStream(finalContent string) {
	if !s.streamStarted {
		return
	}

	s.events <- ui.EventStreamEnded(finalContent)

	// Reset streaming state
	s.streamStarted = false
	s.currentStream = nil
	s.streamFirstChunk = ai.MessageChunk{}
	log.Println("STREAM ENDED")
}

// SendMessage starts the process of waiting for streaming chunks from the LLM
// and feeding them to the UI
func (s *Session) SendMessage(ctx context.Context, message string) error {
	// Add user message to conversation
	s.messages = append(s.messages, ai.Message{
		Role:    "user",
		Content: message,
	})

	return s.sendFullContext(ctx)
}

// sendFullContext sends a full conversation context to the LLM, using streaming
func (s *Session) sendFullContext(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Start streaming
	res, err := s.StartStream(ctx)
	if err != nil {
		s.events <- ui.EventStreamErr(err)
		s.EndStream("")
		return err
	}

	s.EndStream(res)

	// Add assistant message
	s.messages = append(s.messages, ai.Message{
		Role:    "assistant",
		Content: res,
	})

	return nil
}

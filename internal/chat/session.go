package chat

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

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
	client     ai.AIProvider
	messages   []ai.Message
	files      *files.Context
	workingDir string
	tools      []tools.Tool

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

func NewSession(cfg *config.Config, client ai.AIProvider) *Session {
	wd, _ := os.Getwd()
	tt := tools.GetAvailableTools()
	client.SetTools(tt)
	return &Session{
		config:     cfg,
		client:     client,
		messages:   make([]ai.Message, 0),
		files:      files.NewContext(cfg),
		workingDir: wd,
		tools:      tt,
		events:     make(chan any),
		uievents:   make(chan any),
	}
}

// ProcessToolUses handles tool calls from the AI
func (s *Session) ProcessToolUses(toolCall *ai.ToolCall) []tools.ToolResult {
	if !tools.IsValid(s.tools, toolCall.Name) {
		log.Println("Invalid tool use:", toolCall.Name)

		s.events <- ui.EventSystemMsg(fmt.Sprintf("The LLM tried to use an invalid tool: %s", toolCall.Name))

		s.messages = append(s.messages, ai.Message{
			Role:       "user",
			ToolCallID: toolCall.ID,
			Content:    "There is no tool named " + toolCall.Name + ", valid tools are: " + strings.Join(tools.GetNames(s.tools), ", "),
		})

		res, err := s.client.SendMessage(context.Background(), s.messages)
		if err != nil {
			log.Println("Error sending message about invalid tool use:", err)
			return nil
		}

		if res.Content == "" {
			log.Println("Empty response to message about invalid tool use")
			return nil
		}

		s.messages = append(s.messages, ai.Message{
			Role:    "assistant",
			Content: res.Content,
		})

		return nil
	}

	// results := make([]tools.ToolResult, len(toolUses))
	// for i, toolUse := range toolUses {
	// 	results[i] = tools.ExecuteTool(s.config, toolUse, s.workingDir)
	// }

	// s.events <- newToolCallReceived(toolCall)
	return nil
}

// InteractiveMode starts the bubbletea REPL
func (s *Session) InteractiveMode(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil

		case ev := <-s.uievents:
			msg, ok := ev.(ui.EventUserPrompt)
			if !ok {
				log.Printf("Unknown UI event: %T %+v", ev)
				continue
			}

			s.messages = append(s.messages, ai.Message{
				Role:    "user",
				Content: string(msg),
			})

			go s.SendMessage(ctx, string(msg))
		}
	}
}

// StartStream begins a new message stream
func (s *Session) StartStream(ctx context.Context, message string) (string, error) {
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
	for chunk := range stream {
		switch chunk.Type() {
		case ai.ChunkToolCall:
			s.ProcessToolUses(chunk.ToolCall)
			return response.String(), nil

		case ai.ChunkMessage:
			s.events <- ui.EventStreamChunk(chunk.String())
			response.WriteString(chunk.String())

		case ai.ChunkThink:
			// we don't show these yet
		}
	}

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

	// Start streaming
	res, err := s.StartStream(ctx, message)
	if err != nil {
		s.events <- ui.EventStreamErr(err)
		s.EndStream("")
		return err
	}

	s.EndStream(res)

	// Add assistant message
	assistantMsg := ai.Message{
		Role:    "assistant",
		Content: res,
	}
	s.messages = append(s.messages, assistantMsg)

	return nil
}

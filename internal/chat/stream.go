package chat

import (
	"context"
	"log"
	"strings"

	"github.com/penguinpowernz/clai/internal/ai"
)

type Stream struct {
	client ai.Provider
	stream <-chan ai.MessageChunk
	waiter context.Context
	cancel func()

	// callbacks
	onChunk func(ai.MessageChunk)
	onStart func()
	onEnd   func(string)
	onErr   func(error)

	// artifacts
	content   strings.Builder
	reasoning strings.Builder
	toolCall  *ai.ToolCall
}

func NewStream(client ai.Provider) *Stream {
	return &Stream{
		client:    client,
		onChunk:   func(ai.MessageChunk) {},
		onStart:   func() {},
		onEnd:     func(string) {},
		onErr:     func(error) {},
		content:   strings.Builder{},
		reasoning: strings.Builder{},
	}
}

func (s *Stream) OnStart(f func()) {
	s.onStart = f
}

func (s *Stream) OnError(f func(error)) {
	s.onErr = f
}

func (s *Stream) OnEnd(f func(msg string)) {
	s.onEnd = f
}

func (s *Stream) OnChunk(f func(ai.MessageChunk)) {
	s.onChunk = f
}

func (s *Stream) Start(ctx context.Context, cctx []ai.Message) (err error) {
	s.stream, err = s.client.StreamMessage(ctx, cctx)
	if err != nil {
		return err
	}

	// this ctx will lets us know the whole stream lifecycle is done with Wait()
	var done func()
	s.waiter, done = context.WithCancel(context.Background())
	defer done()

	// this ctx will let us cancel, or be cancelled
	ctx, s.cancel = context.WithCancel(ctx)
	defer s.cancel()

	s.onStart()
	log.Println("[stream] starting loop")
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			log.Println("[stream] context done")
			break // this will break out of the loop thanks to the ctx.Err() check
		case chunk := <-s.stream:
			switch chunk.Type() {
			case ai.ChunkToolCall:
				s.toolCall = chunk.ToolCall
				log.Println("[stream] tool called, closing")
				s.Close() // we bail after a single tool call
			case ai.ChunkMessage:
				// log.Println("[stream] message received")
				s.content.WriteString(chunk.Content)
			case ai.ChunkThink:
				// log.Println("[stream] thinking")
				s.reasoning.WriteString(chunk.Content)
			}

			s.onChunk(chunk)
		}
	}
	log.Println("[stream] loop is done")

	s.onEnd(s.content.String())

	done()
	log.Println("[stream] finished")
	return nil
}

func (s *Stream) Close() {
	s.cancel()
}

func (s *Stream) Wait() {
	<-s.waiter.Done()
}

func (s *Stream) Reasoning() string {
	return s.reasoning.String()
}

// ToolCall returns the tool call that happened in this stream, it may be nil
func (s *Stream) ToolCall() *ai.ToolCall {
	return s.toolCall
}

func (s *Stream) Content() string {
	return s.content.String()
}

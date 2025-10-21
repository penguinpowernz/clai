package ui

import (
	"github.com/penguinpowernz/clai/internal/ai"
	"github.com/penguinpowernz/clai/internal/commands"
)

type EventSlashCommand commands.Result
type EventExit struct{}
type EventCancelStream struct{}
type EventStreamCancelled struct{}
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
type EventAssistantMessage string
type EventRunningTool ai.ToolCall
type EventRunningToolDone string
type EventToolOutput string

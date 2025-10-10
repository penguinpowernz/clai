package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripThinkBlock(t *testing.T) {
	assert.Equal(t, "Normal", stripThinkBlock("<think>Hello</think>Normal"))

	block := `<think>
Okay, the user said "hello". I need to respond appropriately. Since there are no specific commands or tools needed here,                                                                                                                                     
I should just greet them back and offer assistance. Let me make sure to keep it friendly and open-ended. Maybe something                                                                                                                                     
like, "Hello! How can I assist you today?" That should cover it without any further action required.    
</think>

Hello! How can I assist you today?`

	assert.Equal(t, "Hello! How can I assist you today?", stripThinkBlock(block))
}

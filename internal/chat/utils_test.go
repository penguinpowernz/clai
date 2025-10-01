package chat

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnhanceMessage(t *testing.T) {
	fn := ""
	fileReader = func(filename string) ([]byte, error) {
		fn = filename
		return []byte("TEST DATA"), nil
	}

	message := enhanceMessage(nil, "I want you to read @test.txt and look for mentions of horse")
	assert.Contains(t, message, "You can see the content of test.txt here:\n```\nTEST DATA\n```\n")
	assert.Equal(t, "test.txt", fn)

	message = enhanceMessage(nil, "I want you to read @cmd/test/main.go and look for mentions of cat")
	assert.Contains(t, message, "You can see the content of cmd/test/main.go here:\n```\nTEST DATA\n```\n")
	assert.Equal(t, "cmd/test/main.go", fn)
}

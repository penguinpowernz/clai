package chat

import (
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/penguinpowernz/clai/config"
)

var reTaggedFilename = regexp.MustCompile(`(@[./a-zA-Z0-9_-]+)`)

var fileReader = os.ReadFile

func enhanceMessage(config *config.Config, message string) string {
	if strings.Contains(message, "@") {
		matches := reTaggedFilename.FindStringSubmatch(message)
		if len(matches) > 0 {
			for _, fn := range matches {
				_fn := fn
				fn := strings.TrimPrefix(fn, "@")
				data, err := fileReader(fn)
				if err != nil {
					log.Println("[session.enhance] failed to read file:", fn, err)
					continue
				}

				message = strings.ReplaceAll(message, _fn, fn)
				message += "\n\nYou can see the content of " + _fn + " here:\n```\n" + string(data) + "\n```\n"
			}
		}
	}

	return message
}

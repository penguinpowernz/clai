package history

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ghodss/yaml"
	"github.com/penguinpowernz/clai/config"
	"github.com/penguinpowernz/clai/internal/ai"
)

var (
	cfg config.Config
	id  string
	mu  sync.Mutex
)

func SetConfig(c config.Config) { cfg = c }
func SetSessionID(s string)     { id = s }

type History struct {
	Context []ai.Message `yaml:"context"`
	UI      []ai.Message `yaml:"ui"`
}

func SaveHistory(what string, messages []ai.Message) error {
	mu.Lock()
	defer mu.Unlock()

	history, err := LoadHistory()
	if err != nil {
		return err
	}

	outfile := filepath.Join(cfg.SessionDir, fmt.Sprintf("%s.yml", id))

	switch what {
	case "context":
		history.Context = messages
	case "ui":
		history.UI = messages
	}

	data, err := yaml.Marshal(history)
	if err != nil {
		return err
	}

	if err := os.WriteFile(outfile, data, 0644); err != nil {
		return err
	}

	return nil
}

func LoadHistory() (History, error) {
	fn := filepath.Join(cfg.SessionDir, fmt.Sprintf("%s.yml", id))
	if _, err := os.Stat(fn); os.IsNotExist(err) {
		return History{}, nil
	}

	data, err := os.ReadFile(fn)
	if err != nil {
		return History{}, err
	}

	var history History
	if err := yaml.Unmarshal(data, &history); err != nil {
		return History{}, err
	}

	return history, nil
}

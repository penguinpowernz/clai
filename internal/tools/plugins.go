package tools

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/penguinpowernz/clai/config"
)

func PluginTools(cfg config.Config) []Tool {
	dir := strings.ReplaceAll(cfg.PluginDir, "~", os.Getenv("HOME"))
	files, _ := os.ReadDir(dir)
	out := []Tool{}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		// TODO: check executable permissions

		fn := filepath.Join(dir, file.Name())
		def, err := loadToolDefinition(fn)
		if err != nil {
			log.Printf("[tools] failed to load tool definition for %s: %s", fn, err)
			continue
		}

		out = append(out, def)
	}

	return out
}

func loadToolDefinition(fn string) (Tool, error) {
	cmd := exec.Command(fn, "--openai")
	data, err := cmd.Output()
	if err != nil {
		return Tool{}, err
	}
	var tool Tool
	err = json.Unmarshal(data, &tool)
	if err != nil {
		return Tool{}, err
	}

	tool.exec = pluginExecutor(fn)
	return tool, nil
}

func pluginExecutor(fn string) toolExecutor {
	return toolExecutor(func(cfg config.Config, input json.RawMessage, workingDir string) (string, error) {
		cmd := exec.Command(fn)

		buf := bytes.NewBuffer(nil)

		// generate the payload
		json.NewEncoder(buf).Encode(map[string]any{
			"input":  string(input),
			"cwd":    workingDir,
			"config": cfg,
		})

		out := bytes.NewBuffer(nil)

		cmd.Stdin = buf
		cmd.Stdout = out
		cmd.Stderr = out
		err := cmd.Run()

		return out.String(), err
	})
}

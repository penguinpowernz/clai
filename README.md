# CLAI

A **work-in-progress**chat CLI written in golang that can talk to Ollama (and others).

**WARNING: AI SLOP** much of this code was written by Claude Sonnet

# Usage

Edit your config in `~/.clai.yml`

```bash
go run ./cmd/clai
```

## TODO

- [x] terminal UI using bubbletea
- [x] scrollable history
- [x] specify system prompt in the config file
- [x] specify settings including model and URL in config file
- [x] working chat with ollama models
- [ ] add `read_file` and `write_file` tools
- [ ] add `search_file` and `list_files` tools
- [ ] send files in prompt when `@filename` is in the prompt
- [ ] add list `/models` command
- [ ] add `/model <modelname>` command
- [ ] add `/clear` command to reset the prompt
- [ ] add `/quit` command to exit
- [ ] save chat history to file
- [ ] working chat with anthropic models
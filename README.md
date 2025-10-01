# CLAI

A **work-in-progress** chat CLI written in golang that can talk to Ollama (and others).

The current status is that it is working with self hosted Ollama models.  Tool use is working but has not been extensively tested. It is quite simple for now, and things are subject to change.

<img width="862" height="259" alt="image" src="https://github.com/user-attachments/assets/bc5d3972-c985-46d4-8fff-eb8e9af64873" />

# Usage

Edit your config in `~/.clai.yml`:

```yml
# AI Code Assistant Configuration

system_prompt: |
  You are a helpful coding assistant.  The user is chatting with you via a CLI agent.  This agent will make various tools available to you to help the user.

# AI Provider (anthropic, openai, ollama, or custom)
provider: ollama
model: gpt-oss:latest
base_url: http://192.168.1.118:11434/v1

# Base URL for API endpoint (optional - defaults set per provider)
# For Ollama: http://localhost:11434/v1
# For custom OpenAI-compatible APIs
# base_url: http://localhost:11434/v1

# API Key (or use environment variable)
# Not required for Ollama or local models
# api_key: your-api-key-here

# Behavior
auto_apply: false      # Automatically apply code changes
context_files: 5       # Max files to include in context
max_tokens: 4096       # Max tokens per request
temperature: 0.7       # Model temperature (0.0 - 1.0)

# UI
verbose: false         # Verbose logging
editor: vim            # Preferred editor

# File handling
exclude_patterns:
  - node_modules/
  - .git/
  - "*.log"
  - "*.tmp"
  - vendor/
  - dist/
  - build/

include_hidden: false  # Include hidden files
max_file_size: 50000   # Max file size in bytes (50KB)

# Session
session_dir: .aicode   # Where to store session data
save_history: true     # Save conversation history
max_history_size: 100  # Max messages to keep in history
```

To run it:

```bash
go run ./cmd/clai
```

Send a prompt using CTRL+D, quit with CTRL+C or ESC...

## TODO

In order of priority.

- [x] terminal UI using bubbletea
- [x] scrollable history
- [x] specify system prompt in the config file
- [x] specify settings including model and URL in config file
- [x] working chat with ollama models
- [x] add reasoning output
- [ ] get errors and system messages showing in the UI
- [ ] cancel running inference with CTRL+C
- [ ] send files in prompt when `@filename` is in the prompt
- [x] add `search_file` and `list_files` tools
- [x] ask for permission for the AI to use tools
- [x] add `read_file` and `write_file` tools
- [ ] add list `/models` command
- [ ] add `/model <modelname>` command
- [ ] add `/clear` command to reset the prompt
- [ ] add `/quit` command to exit
- [ ] turn thinking output on and off with `/thinking` and config item
- [x] save chat history to file
- [ ] load chat history from file
- [ ] add `run_command` tool
- [ ] support for git tool usage
- [ ] automatic git commit for every change
- [ ] ~~working chat with anthropic models~~

# Code of conduct

Our community is built on the principles of **meritocracy**, where contributions are valued based on their quality, relevance, and impact. We strive to create an inclusive environment where individuals can collaborate, learn, and grow **without the influence of personal biases, affiliations, or identities**.

[Non-Toxic CoC](https://github.com/penguinpowernz/clai/blob/main/CODE_OF_CONDUCT.md)
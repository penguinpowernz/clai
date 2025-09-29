# CLAI

A **work-in-progress**chat CLI written in golang that can talk to Ollama (and others).

**WARNING: AI SLOP** much of this code was written by Claude Sonnet.  The current status is that it is working
with self hosted Ollama models.

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
auto_commit: false     # Automatically commit changes
stream: true           # Stream responses
context_files: 5       # Max files to include in context
max_tokens: 4096       # Max tokens per request
temperature: 0.7       # Model temperature (0.0 - 1.0)

# Git
use_git: true          # Enable git integration
git_auto_stage: false  # Auto-stage changes

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

```bash
go run ./cmd/clai
```

## TODO

In order of priority.

- [x] terminal UI using bubbletea
- [x] scrollable history
- [x] specify system prompt in the config file
- [x] specify settings including model and URL in config file
- [x] working chat with ollama models
- [ ] send files in prompt when `@filename` is in the prompt
- [ ] add `search_file` and `list_files` tools
- [ ] ask for permission for the AI to use tools
- [ ] add `read_file` and `write_file` tools
- [ ] add list `/models` command
- [ ] add `/model <modelname>` command
- [ ] add `/clear` command to reset the prompt
- [ ] add `/quit` command to exit
- [ ] save chat history to file
- [ ] working chat with anthropic models
- [ ] support for git tool usage
- [ ] automatic git commit for every change
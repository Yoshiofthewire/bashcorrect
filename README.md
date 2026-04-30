# BashCorrect

AI-powered shell autocorrect and assistant. Hooks into your shell to fix failed commands and answer questions directly from the terminal.

## Features

- **Autocorrect** — when a command fails, BashCorrect asks your AI provider for the fix, shows a colored diff, and prompts `[y/N/edit]`
- **Direct queries** — prefix any natural-language request with `?` to get an AI answer in your terminal
- **Inline buffer replacement** — press **Alt+Enter** on any command to replace it with an AI suggestion before running
- **Multi-provider** — switch between OpenAI, Anthropic, Gemini, and GitHub Copilot in one flag
- **Multi-shell** — bash, zsh, fish, and PowerShell 7+ (Linux, macOS, Windows)
- **Single binary** — no runtime dependencies; shell integration scripts are embedded and written on `init`

## Installation

**Requirements:** Go 1.22+

```sh
go install github.com/Yoshiofthewire/bashcorrect@latest
```

Or build from source:

```sh
git clone https://github.com/Yoshiofthewire/bashcorrect
cd bashcorrect
make install
```

Cross-platform binaries (Linux, macOS, Windows — amd64 + arm64):

```sh
make dist   # outputs to dist/
```

## Quick Start

### 1. Set up shell integration

```sh
bashcorrect init              # auto-detects your shell
# or specify explicitly:
bashcorrect init --shell bash
bashcorrect init --shell zsh
bashcorrect init --shell fish
bashcorrect init --shell powershell
```

This writes the integration script to `~/.config/bashcorrect/shell/` and appends a sourcing line to your rc file. Restart your shell or `source` the rc file.

### 2. Configure a provider

Edit `~/.config/bashcorrect/config.toml` (created automatically on first run):

```toml
active_provider = "openai"

[providers.openai]
api_key = "sk-..."
model   = "gpt-4o"

[providers.anthropic]
api_key = "sk-ant-..."

[providers.gemini]
api_key = "AIza..."

[providers.copilot]
# No key needed — auto-resolved from `gh auth login` or $GITHUB_TOKEN
```

### 3. Use it

```sh
# Autocorrect — just run a broken command; BashCorrect triggers on non-zero exit
gti status
# → shows diff: - gti status / + git status
# → Run corrected command? [y/N/e(dit)]

# Direct query
? how do I recursively find files modified in the last 24 hours

# Alt+Enter — with cursor on any command in the buffer, press Alt+Enter
# to replace it with an AI suggestion before running
```

## Commands

### `bashcorrect correct`

```
bashcorrect correct --cmd <failed-command> --exit-code <N> [--stderr <output>]
```

Called automatically by the shell hook after a non-zero exit. Sends the failed command and exit code to the active provider, displays a colored diff, and prompts the user to accept, skip, or edit.

| Flag | Description |
|---|---|
| `--cmd` | The failed command (required) |
| `--exit-code` | Exit code of the failed command (default 1) |
| `--stderr` | Captured stderr output to include as context |

### `bashcorrect query`

```
bashcorrect query <prompt> [flags]
```

Send a natural-language prompt to the active AI provider.

| Flag | Description |
|---|---|
| `--inline` | Output only the extracted command (used by keybindings) |
| `--cwd` | Include current working directory as context |
| `--history-lines N` | Include last N lines of shell history as context |

### `bashcorrect init`

```
bashcorrect init [--shell bash|zsh|fish|powershell]
```

Installs shell integration. Auto-detects the shell from `$SHELL` if `--shell` is omitted.

### Global flags

| Flag | Description |
|---|---|
| `--provider` | Override active provider for this invocation |
| `--model` | Override model for this invocation |
| `--config` | Path to config file (default: `$XDG_CONFIG_HOME/bashcorrect/config.toml`) |

## Providers

| Provider | Default model | How to authenticate |
|---|---|---|
| `openai` | `gpt-4o` | Set `providers.openai.api_key` in config |
| `anthropic` | `claude-3-5-sonnet-20241022` | Set `providers.anthropic.api_key` in config |
| `gemini` | `gemini-1.5-pro` | Set `providers.gemini.api_key` in config |
| `copilot` | `gpt-4o` | Auto-resolved from `gh auth login` → `~/.config/github-copilot/hosts.json`, or `$GITHUB_TOKEN` |

Switch provider per-command:

```sh
bashcorrect query "explain this error" --provider anthropic
```

## Shell Integration Details

### bash

- Autocorrect hook via `PROMPT_COMMAND`
- `?` shell function alias: `? what does chmod 755 mean`
- Alt+Enter via `bind -x '"\e\n":...'`

### zsh

- Autocorrect hook via `add-zsh-hook precmd`
- `alias '?'='bashcorrect query'`
- Alt+Enter via `zle` widget + `bindkey`

### fish

- Autocorrect hook via `fish_postexec` event function
- `abbr --add '?' 'bashcorrect query'`
- Alt+Enter via `bind \e\n`

### PowerShell (Linux / macOS / Windows)

Requires **PowerShell 7+** (`pwsh`) and **PSReadLine** (included by default).

- Autocorrect hook wraps the existing `prompt` function, preserving any customisations
- `bc?` alias for queries (avoids conflict with built-in `?` / `Where-Object`); override with `$env:BASHCORRECT_ALIAS`
- Alt+Enter via `Set-PSReadLineKeyHandler -Chord 'Alt+Enter'`
- Profile path: `~/.config/powershell/Microsoft.PowerShell_profile.ps1` (Linux/macOS) or `Documents\PowerShell\Microsoft.PowerShell_profile.ps1` (Windows); override with `$env:BASHCORRECT_PS_PROFILE`

## Configuration Reference

Default config (`~/.config/bashcorrect/config.toml`):

```toml
active_provider = "openai"
trigger_prefix  = "?"

[autocorrect]
auto_run  = false   # if true, run the fix without prompting
show_diff = true    # show before/after diff

[providers.openai]
api_key = ""
model   = "gpt-4o"

[providers.anthropic]
api_key = ""
model   = "claude-3-5-sonnet-20241022"

[providers.gemini]
api_key = ""
model   = "gemini-1.5-pro"

[providers.copilot]
api_key = ""        # leave blank to auto-resolve from gh CLI
model   = "gpt-4o"
```

## Building

```sh
make build    # build ./bashcorrect
make install  # go install
make test     # go test ./...
make lint     # go vet + staticcheck
make dist     # cross-compile for all platforms → dist/
```

## License

MIT

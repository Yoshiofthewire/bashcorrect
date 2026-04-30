# BashCorrect — AI-Powered Shell Autocorrect & Assistant

BashCorrect is a Go CLI tool that integrates into your shell to:
1. **Autocorrect failed commands** — when a command exits non-zero, it asks an AI for the corrected version, shows a before/after diff, and prompts you to run, skip, or edit the fix.
2. **Answer direct AI queries** — type `? how do I list files modified in the last 24 hours` or press **Alt+Enter** on any command to get an AI suggestion in-place.

## Inspiration

- **Warp Terminal** (https://github.com/warpdotdev/warp) — agentic development environment born out of the terminal; inspiration for AI-native shell UX.
- **OpenClaw** (https://github.com/openclaw/openclaw) — personal AI assistant architecture; inspiration for multi-provider routing and local-first design. OpenClaw is **not** a runtime dependency.

## Supported Shells

| Shell | Autocorrect hook | Query alias | Alt+Enter keybinding |
|---|---|---|---|
| bash | `PROMPT_COMMAND` | `?` function | `bind -x` |
| zsh | `add-zsh-hook precmd` | `alias ?` | `zle` widget |
| fish | `fish_postexec` event | `abbr ?` | `bind \e\n` |
| PowerShell 7+ | `prompt` wrapper | `bc?` alias | `Set-PSReadLineKeyHandler` |

PowerShell integration works on **Linux, macOS, and Windows** (PowerShell 7+ / `pwsh`).

## Supported AI Providers

| Provider | Default model | Auth |
|---|---|---|
| OpenAI | `gpt-4o` | `providers.openai.api_key` in config |
| Anthropic | `claude-3-5-sonnet-20241022` | `providers.anthropic.api_key` |
| Google Gemini | `gemini-1.5-pro` | `providers.gemini.api_key` |
| GitHub Copilot | `gpt-4o` | auto-resolved from `gh` CLI or `$GITHUB_TOKEN` |

Providers are interchangeable — switch at any time with `--provider` or by editing config.

## Architecture

```
bashcorrect/
├── main.go
├── go.mod
├── cmd/
│   ├── root.go        # Cobra root, global flags (--provider, --model, --config)
│   ├── correct.go     # `bashcorrect correct` — autocorrect a failed command
│   ├── query.go       # `bashcorrect query`   — direct AI query
│   └── init.go        # `bashcorrect init`    — shell integration installer
├── providers/
│   ├── provider.go    # Provider interface + factory
│   ├── openai.go      # OpenAI
│   ├── anthropic.go   # Anthropic
│   ├── gemini.go      # Google Gemini
│   └── copilot.go     # GitHub Copilot
├── config/
│   └── config.go      # TOML config at $XDG_CONFIG_HOME/bashcorrect/config.toml
├── shelldata/
│   ├── embed.go       # //go:embed bundles scripts into the binary
│   ├── bash.sh
│   ├── zsh.sh
│   ├── fish.fish
│   └── powershell.ps1
└── Makefile
```

## Implementation Notes

- Single static binary — no runtime dependencies, no Node/Python required.
- Config location uses `os.UserConfigDir()`: `~/.config/bashcorrect/` on Linux/macOS, `%APPDATA%\bashcorrect\` on Windows.
- Shell integration scripts are embedded in the binary via `//go:embed` and written to disk by `bashcorrect init`.
- All provider calls use stdlib `net/http` — no vendor SDKs, minimal attack surface.
- PowerShell's built-in `?` alias (for `Where-Object`) is preserved; BashCorrect uses `bc?` by default (overridable via `$env:BASHCORRECT_ALIAS`).
- `bashcorrect correct` prints the accepted command to stdout; the shell hook `eval`s it, so the corrected command appears in shell history naturally. 
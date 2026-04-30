#!/usr/bin/env bash
# BashCorrect — bash integration
# Source this file from your ~/.bashrc via:
#   eval "$(bashcorrect init --shell bash)"
# or add:
#   source <path/to/bash.sh>

# ---------------------------------------------------------------------------
# Autocorrect hook: runs after every command with a non-zero exit code
# ---------------------------------------------------------------------------
_bashcorrect_precmd() {
    local _bc_exit=$?
    # Ignore exit code 0 and interactive-signal exits (130 = Ctrl-C)
    if [[ $_bc_exit -eq 0 || $_bc_exit -eq 130 ]]; then
        return
    fi
    local _bc_last_cmd
    _bc_last_cmd=$(HISTTIMEFORMAT='' history 1 | sed 's/^[[:space:]]*[0-9]*[[:space:]]*//')
    if [[ -z "$_bc_last_cmd" ]]; then
        return
    fi
    # Avoid recursive correction
    if [[ "$_bc_last_cmd" == bashcorrect* ]]; then
        return
    fi
    local _bc_suggestion
    _bc_suggestion=$(bashcorrect correct --cmd "$_bc_last_cmd" --exit-code "$_bc_exit" 2>/dev/tty)
    if [[ -n "$_bc_suggestion" ]]; then
        eval "$_bc_suggestion"
    fi
}

# Prepend to PROMPT_COMMAND, preserving any existing value
if [[ -z "$PROMPT_COMMAND" ]]; then
    PROMPT_COMMAND="_bashcorrect_precmd"
else
    PROMPT_COMMAND="_bashcorrect_precmd;${PROMPT_COMMAND}"
fi

# ---------------------------------------------------------------------------
# Direct query alias: `? how do I list files recursively`
# ---------------------------------------------------------------------------
'?'() {
    bashcorrect query "$*"
}

# ---------------------------------------------------------------------------
# Alt+Enter keybinding: replace current readline buffer with AI suggestion
# ---------------------------------------------------------------------------
_bashcorrect_keybind() {
    local _bc_result
    _bc_result=$(bashcorrect query "$READLINE_LINE" --inline 2>/dev/null)
    if [[ -n "$_bc_result" ]]; then
        READLINE_LINE="$_bc_result"
        READLINE_POINT=${#READLINE_LINE}
    fi
}
# Bind Alt+Enter (\e\n) — works in readline-enabled terminals
bind -x '"\e\n":_bashcorrect_keybind' 2>/dev/null || true

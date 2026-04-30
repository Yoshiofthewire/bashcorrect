#!/usr/bin/env zsh
# BashCorrect — zsh integration
# Source this file from your ~/.zshrc via:
#   source <path/to/zsh.sh>
# or let `bashcorrect init --shell zsh` append the sourcing line automatically.

autoload -Uz add-zsh-hook

# ---------------------------------------------------------------------------
# Autocorrect hook: runs after every command with a non-zero exit code
# ---------------------------------------------------------------------------
_bashcorrect_precmd() {
    local _bc_exit=$?
    if [[ $_bc_exit -eq 0 || $_bc_exit -eq 130 ]]; then
        return
    fi
    local _bc_last_cmd="${history[$HISTCMD]}"
    if [[ -z "$_bc_last_cmd" ]]; then
        return
    fi
    if [[ "$_bc_last_cmd" == bashcorrect* ]]; then
        return
    fi
    local _bc_suggestion
    _bc_suggestion=$(bashcorrect correct --cmd "$_bc_last_cmd" --exit-code "$_bc_exit" 2>/dev/tty)
    if [[ -n "$_bc_suggestion" ]]; then
        eval "$_bc_suggestion"
    fi
}

add-zsh-hook precmd _bashcorrect_precmd

# ---------------------------------------------------------------------------
# Direct query alias
# ---------------------------------------------------------------------------
alias '?'='bashcorrect query'

# ---------------------------------------------------------------------------
# Alt+Enter zle widget: replace the current buffer with an AI suggestion
# ---------------------------------------------------------------------------
_bashcorrect_widget() {
    local _bc_result
    _bc_result=$(bashcorrect query "$BUFFER" --inline 2>/dev/null)
    if [[ -n "$_bc_result" ]]; then
        BUFFER="$_bc_result"
        CURSOR=${#BUFFER}
    fi
    zle redisplay
}
zle -N _bashcorrect_widget

# Alt+Enter: \e then Enter (\n)
bindkey "^[\n" _bashcorrect_widget
# Also bind the common escape sequence for terminals that send \e^M for Alt+Enter
bindkey "^[^M" _bashcorrect_widget

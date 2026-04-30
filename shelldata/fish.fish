# BashCorrect — fish integration
# Add to your fish config via:
#   bashcorrect init --shell fish
# or manually append to ~/.config/fish/config.fish:
#   source /path/to/fish.fish

# ---------------------------------------------------------------------------
# Autocorrect hook: fish_postexec runs after every command
# ---------------------------------------------------------------------------
function _bashcorrect_postexec --on-event fish_postexec
    set -l last_exit $status
    set -l last_cmd $argv[1]

    if test $last_exit -eq 0; or test $last_exit -eq 130
        return
    end
    if test -z "$last_cmd"
        return
    end
    # Avoid recursive correction
    if string match -q 'bashcorrect*' -- $last_cmd
        return
    end

    set -l suggestion (bashcorrect correct --cmd $last_cmd --exit-code $last_exit 2>/dev/tty)
    if test -n "$suggestion"
        eval $suggestion
    end
end

# ---------------------------------------------------------------------------
# Direct query abbreviation: `? how do I list files`
# ---------------------------------------------------------------------------
abbr --add '?' 'bashcorrect query'

# ---------------------------------------------------------------------------
# Alt+Enter keybinding: replace commandline buffer with AI suggestion
# ---------------------------------------------------------------------------
function _bashcorrect_keybind
    set -l current_cmd (commandline)
    if test -z "$current_cmd"
        return
    end
    set -l suggestion (bashcorrect query $current_cmd --inline 2>/dev/null)
    if test -n "$suggestion"
        commandline --replace -- $suggestion
    end
end

# Bind Alt+Enter (\e\n)
bind \e\n _bashcorrect_keybind

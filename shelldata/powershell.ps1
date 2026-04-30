# BashCorrect — PowerShell integration (Linux, macOS, Windows)
# Requires: PowerShell 7+ (pwsh) and PSReadLine (included by default)
#
# Add to your $PROFILE via:
#   bashcorrect init --shell powershell
# or manually append:
#   . /path/to/powershell.ps1

# ---------------------------------------------------------------------------
# Autocorrect hook: wrap the prompt function to check $LASTEXITCODE
# ---------------------------------------------------------------------------
$__BashCorrectOriginalPrompt = $null

function global:Invoke-BashCorrectHook {
    $exitCode = $LASTEXITCODE
    $lastCmd  = (Get-History -Count 1).CommandLine

    if ($exitCode -ne 0 -and $exitCode -ne $null -and $lastCmd -and
        -not $lastCmd.StartsWith('bashcorrect')) {

        $suggestion = & bashcorrect correct --cmd $lastCmd --exit-code $exitCode 2>$null
        if ($suggestion) {
            Invoke-Expression $suggestion
        }
    }
}

# Wrap the existing prompt to inject our hook without replacing user customisations
if (-not $__BashCorrectOriginalPrompt) {
    $__BashCorrectOriginalPrompt = (Get-Item function:prompt).ScriptBlock
    function global:prompt {
        Invoke-BashCorrectHook
        & $__BashCorrectOriginalPrompt
    }
}

# ---------------------------------------------------------------------------
# Direct query function and alias
# NOTE: '?' is a built-in alias for Where-Object; we use 'bc?' instead.
#       Users can override by setting $env:BASHCORRECT_ALIAS before sourcing.
# ---------------------------------------------------------------------------
function global:Invoke-BashCorrectQuery {
    param([Parameter(ValueFromRemainingArguments)][string[]]$Prompt)
    & bashcorrect query @Prompt
}

$__BcAlias = if ($env:BASHCORRECT_ALIAS) { $env:BASHCORRECT_ALIAS } else { 'bc?' }
Set-Alias -Name $__BcAlias -Value Invoke-BashCorrectQuery -Scope Global -Force

# ---------------------------------------------------------------------------
# Alt+Enter keybinding via PSReadLine: replace current buffer with suggestion
# ---------------------------------------------------------------------------
if (Get-Module -ListAvailable PSReadLine -ErrorAction SilentlyContinue) {
    Set-PSReadLineKeyHandler -Chord 'Alt+Enter' -ScriptBlock {
        $line   = $null
        $cursor = $null
        [Microsoft.PowerShell.PSConsoleReadLine]::GetBufferState([ref]$line, [ref]$cursor)

        if ($line) {
            $suggestion = & bashcorrect query $line --inline 2>$null
            if ($suggestion) {
                [Microsoft.PowerShell.PSConsoleReadLine]::RevertLine()
                [Microsoft.PowerShell.PSConsoleReadLine]::Insert($suggestion)
            }
        }
    }
}

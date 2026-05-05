# BashCorrect — PowerShell integration (Linux, macOS, Windows)
# Requires: PowerShell 7+ (pwsh) and PSReadLine (included by default)
#
# Add to your $PROFILE via:
#   bashcorrect init --shell powershell
# or manually append:
#   . /path/to/powershell.ps1

$__BashCorrectBin = '__BASHCORRECT_BIN__'
if (-not (Test-Path $__BashCorrectBin)) {
    $cmd = Get-Command bashcorrect -ErrorAction SilentlyContinue
    if ($cmd) {
        $__BashCorrectBin = $cmd.Source
    } else {
        return
    }
}

# ---------------------------------------------------------------------------
# Autocorrect hook: wrap the prompt function to check $LASTEXITCODE
# ---------------------------------------------------------------------------
$__BashCorrectOriginalPrompt = $null

function global:Invoke-BashCorrectHook {
    $exitCode = $LASTEXITCODE
    $lastCmd  = (Get-History -Count 1).CommandLine

    if ($exitCode -ne 0 -and $exitCode -ne $null -and $lastCmd -and
        -not $lastCmd.Contains('bashcorrect')) {

        $suggestion = & $__BashCorrectBin correct --cmd $lastCmd --exit-code $exitCode --shell powershell 2>$null
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
# By default we define a global '?' function so usage matches bash/zsh/fish.
# This is more reliable than alias replacement of the built-in Where-Object.
# Users can override via $env:BASHCORRECT_ALIAS before sourcing.
# ---------------------------------------------------------------------------
function global:Invoke-BashCorrectQuery {
    param([Parameter(ValueFromRemainingArguments)][string[]]$Prompt)
    & $__BashCorrectBin query @Prompt
}

if ($env:BASHCORRECT_ALIAS) {
    Set-Alias -Name $env:BASHCORRECT_ALIAS -Value Invoke-BashCorrectQuery -Scope Global -Force
} else {
    Remove-Item alias:? -Force -ErrorAction SilentlyContinue
    function global:? {
        param([Parameter(ValueFromRemainingArguments)][string[]]$Prompt)
        Invoke-BashCorrectQuery @Prompt
    }
}

# Always provide bc? as a stable fallback alias.
Set-Alias -Name 'bc?' -Value Invoke-BashCorrectQuery -Scope Global -Force

# ---------------------------------------------------------------------------
# Alt+Enter keybinding via PSReadLine: replace current buffer with suggestion
# ---------------------------------------------------------------------------
if (Get-Module -ListAvailable PSReadLine -ErrorAction SilentlyContinue) {
    Set-PSReadLineKeyHandler -Chord 'Alt+Enter' -ScriptBlock {
        $line   = $null
        $cursor = $null
        [Microsoft.PowerShell.PSConsoleReadLine]::GetBufferState([ref]$line, [ref]$cursor)

        if ($line) {
            $suggestion = & $__BashCorrectBin query $line --inline 2>$null
            if ($suggestion) {
                [Microsoft.PowerShell.PSConsoleReadLine]::RevertLine()
                [Microsoft.PowerShell.PSConsoleReadLine]::Insert($suggestion)
            }
        }
    }
}

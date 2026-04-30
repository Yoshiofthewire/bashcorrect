// Package shelldata embeds the BashCorrect shell integration scripts so they
// can be bundled into the single binary and written out by `bashcorrect init`.
package shelldata

import "embed"

// FS holds all shell integration scripts.
//
//go:embed bash.sh zsh.sh fish.fish powershell.ps1
var FS embed.FS

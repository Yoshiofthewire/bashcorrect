package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Yoshiofthewire/bashcorrect/shelldata"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Install BashCorrect shell integration",
	Long: `Appends the BashCorrect sourcing line to your shell's rc file and writes
the integration script to ~/.config/bashcorrect/shell/.

Supported shells: bash, zsh, fish, powershell`,
	RunE: runInit,
}

var initShellFlag string

func init() {
	initCmd.Flags().StringVar(&initShellFlag, "shell", "", "shell to configure: bash, zsh, fish, powershell (auto-detected if omitted)")
	rootCmd.AddCommand(initCmd)
}

type shellDef struct {
	scriptSrc string // path inside embed.FS
	rcFile    func() string
	sourceLine func(scriptPath string) string
}

var shells = map[string]shellDef{
	"bash": {
		scriptSrc: "bash.sh",
		rcFile:    rcFileBash,
		sourceLine: func(p string) string {
			return fmt.Sprintf("\n# BashCorrect\nsource %q\n", p)
		},
	},
	"zsh": {
		scriptSrc: "zsh.sh",
		rcFile:    rcFileZsh,
		sourceLine: func(p string) string {
			return fmt.Sprintf("\n# BashCorrect\nsource %q\n", p)
		},
	},
	"fish": {
		scriptSrc: "fish.fish",
		rcFile:    rcFileFish,
		sourceLine: func(p string) string {
			return fmt.Sprintf("\n# BashCorrect\nsource %q\n", p)
		},
	},
	"powershell": {
		scriptSrc: "powershell.ps1",
		rcFile:    rcFilePowerShell,
		sourceLine: func(p string) string {
			return fmt.Sprintf("\n# BashCorrect\n. '%s'\n", p)
		},
	},
}

func runInit(_ *cobra.Command, _ []string) error {
	shell := initShellFlag
	if shell == "" {
		shell = detectShell()
	}
	if shell == "" {
		return fmt.Errorf("could not detect shell — pass --shell bash|zsh|fish|powershell")
	}

	def, ok := shells[shell]
	if !ok {
		return fmt.Errorf("unsupported shell %q — valid choices: bash, zsh, fish, powershell", shell)
	}

	// Write integration script to config dir
	cfgDir, err := config_dir()
	if err != nil {
		return err
	}
	shellDir := filepath.Join(cfgDir, "shell")
	if err := os.MkdirAll(shellDir, 0o700); err != nil {
		return fmt.Errorf("creating shell dir: %w", err)
	}

	scriptData, err := shelldata.FS.ReadFile(def.scriptSrc)
	if err != nil {
		return fmt.Errorf("reading embedded script: %w", err)
	}

	scriptDest := filepath.Join(shellDir, filepath.Base(def.scriptSrc))
	if err := os.WriteFile(scriptDest, scriptData, 0o644); err != nil {
		return fmt.Errorf("writing script: %w", err)
	}

	// Append sourcing line to rc file
	rcPath := def.rcFile()
	if rcPath == "" {
		return fmt.Errorf("could not determine rc file path for %s", shell)
	}

	sourceLine := def.sourceLine(scriptDest)

	// Check if already installed
	existing, _ := os.ReadFile(rcPath)
	if strings.Contains(string(existing), scriptDest) {
		fmt.Printf("BashCorrect is already configured in %s\n", rcPath)
		return nil
	}

	// Create rc file if it doesn't exist (common for PowerShell profiles)
	if err := os.MkdirAll(filepath.Dir(rcPath), 0o700); err != nil {
		return fmt.Errorf("creating rc file directory: %w", err)
	}

	f, err := os.OpenFile(rcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening rc file %s: %w", rcPath, err)
	}
	defer f.Close()

	if _, err := f.WriteString(sourceLine); err != nil {
		return fmt.Errorf("writing to rc file: %w", err)
	}

	fmt.Printf("BashCorrect installed for %s\n", shell)
	fmt.Printf("  Script: %s\n", scriptDest)
	fmt.Printf("  RC file: %s\n", rcPath)
	fmt.Printf("\nRestart your shell or run:\n")
	switch shell {
	case "bash", "zsh":
		fmt.Printf("  source %s\n", rcPath)
	case "fish":
		fmt.Printf("  source %s\n", rcPath)
	case "powershell":
		fmt.Printf("  . '%s'\n", rcPath)
	}
	return nil
}

func detectShell() string {
	shellEnv := os.Getenv("SHELL")
	switch {
	case strings.HasSuffix(shellEnv, "bash"):
		return "bash"
	case strings.HasSuffix(shellEnv, "zsh"):
		return "zsh"
	case strings.HasSuffix(shellEnv, "fish"):
		return "fish"
	}
	// Check if running under pwsh on Windows
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	return ""
}

func rcFileBash() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".bashrc")
}

func rcFileZsh() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	// Respect ZDOTDIR if set
	if zdot := os.Getenv("ZDOTDIR"); zdot != "" {
		return filepath.Join(zdot, ".zshrc")
	}
	return filepath.Join(home, ".zshrc")
}

func rcFileFish() string {
	// Respect XDG_CONFIG_HOME
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "fish", "config.fish")
}

func rcFilePowerShell() string {
	// $PROFILE resolves differently per OS; check env first
	if p := os.Getenv("BASHCORRECT_PS_PROFILE"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	switch runtime.GOOS {
	case "windows":
		docs := os.Getenv("USERPROFILE")
		if docs == "" {
			docs = home
		}
		return filepath.Join(docs, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	default:
		// Linux and macOS
		return filepath.Join(home, ".config", "powershell", "Microsoft.PowerShell_profile.ps1")
	}
}

func config_dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("could not determine config directory: %w", err)
	}
	return filepath.Join(base, "bashcorrect"), nil
}

package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/Yoshiofthewire/bashcorrect/providers"
	"github.com/spf13/cobra"
)

var correctCmd = &cobra.Command{
	Use:   "correct",
	Short: "Suggest a correction for a failed shell command",
	Long: `Sends the failed command to the active AI provider and displays the
suggested correction with a before/after diff. You are prompted to run,
skip, or edit the suggestion.`,
	RunE: runCorrect,
}

var (
	correctCmdFlag      string
	correctExitCodeFlag int
	correctStderrFlag   string
	correctShellFlag    string
	stdinReader         = bufio.NewReader(os.Stdin)
)

func init() {
	correctCmd.Flags().StringVar(&correctCmdFlag, "cmd", "", "the failed command (required)")
	correctCmd.Flags().IntVar(&correctExitCodeFlag, "exit-code", 1, "exit code of the failed command")
	correctCmd.Flags().StringVar(&correctStderrFlag, "stderr", "", "stderr output of the failed command")
	correctCmd.Flags().StringVar(&correctShellFlag, "shell", "", "shell name (optional override, e.g. bash/zsh/fish/powershell)")
	_ = correctCmd.MarkFlagRequired("cmd")
	rootCmd.AddCommand(correctCmd)
}

const correctSystemPrompt = `You are an expert shell assistant. The user ran a command that failed.
Output ONLY the corrected command — no explanation, no markdown, no code fences, just the raw command.
If the command cannot be meaningfully corrected, output the original command unchanged.

When the prompt includes "Command-not-found mode: true":
- Prefer a safe command that helps the user resolve a missing executable.
- First consider local file checks or running a local script/binary when appropriate.
- Otherwise provide a distro-appropriate package search command from the prompt context.`

func runCorrect(cmd *cobra.Command, _ []string) error {
	p, err := providers.NewActive(cfg)
	if err != nil {
		return err
	}

	if isCommandNotFound(correctExitCodeFlag, correctStderrFlag) {
		if fallback := buildCommandNotFoundFallback(correctCmdFlag); fallback != "" {
			printDiff(correctCmdFlag, fallback)
			if cfg.Autocorrect.AutoRun {
				fmt.Println(fallback)
				return nil
			}

			answer := promptUser("Run corrected command? [y/N/e(dit)] ")
			switch strings.ToLower(strings.TrimSpace(answer)) {
			case "y", "yes":
				fmt.Println(fallback)
			case "e", "edit":
				edited, err := openInEditor(fallback)
				if err != nil {
					return err
				}
				fmt.Println(strings.TrimSpace(edited))
			}
			return nil
		}
	}

	userPrompt := buildCorrectPrompt(correctCmdFlag, correctExitCodeFlag, correctStderrFlag, correctShellFlag)

	fmt.Fprintf(os.Stderr, "\033[2m[bashcorrect] asking %s...\033[0m\n", p.Name())
	suggestion, err := p.Query(context.Background(), correctSystemPrompt, userPrompt)
	if err != nil {
		return fmt.Errorf("provider error: %w", err)
	}
	suggestion = strings.TrimSpace(suggestion)

	// Show diff
	printDiff(correctCmdFlag, suggestion)

	if cfg.Autocorrect.AutoRun {
		fmt.Println(suggestion)
		return nil
	}

	answer := promptUser("Run corrected command? [y/N/e(dit)] ")
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		fmt.Println(suggestion)
	case "e", "edit":
		edited, err := openInEditor(suggestion)
		if err != nil {
			return err
		}
		fmt.Println(strings.TrimSpace(edited))
	default:
		// user declined — print nothing, shell hook will not eval
	}
	return nil
}

func buildCorrectPrompt(cmd string, exitCode int, stderr, shellOverride string) string {
	var sb strings.Builder
	shellName := detectShellName(shellOverride)
	sb.WriteString(fmt.Sprintf("OS: %s", runtime.GOOS))
	sb.WriteString(fmt.Sprintf("\nShell: %s", shellName))
	if osRelease := buildOSReleasePromptContext(); osRelease != "" {
		sb.WriteString("\n")
		sb.WriteString(osRelease)
	}
	sb.WriteString(fmt.Sprintf("\nFailed command (exit code %d): %s", exitCode, cmd))
	if stderr != "" {
		sb.WriteString(fmt.Sprintf("\nError output:\n%s", stderr))
	}
	if isCommandNotFound(exitCode, stderr) {
		failed := extractFailedBinary(cmd)
		sb.WriteString("\nCommand-not-found mode: true")
		if failed != "" {
			sb.WriteString(fmt.Sprintf("\nMissing executable: %s", failed))
			sb.WriteString(fmt.Sprintf("\nLocal file check command: find . -maxdepth 3 -type f \\( -name %q -o -name %q \\)", failed, failed+".*"))
			if pkgCmd := detectPackageSearchCommand(failed); pkgCmd != "" {
				sb.WriteString(fmt.Sprintf("\nPackage search command: %s", pkgCmd))
			}
		}
	}
	return sb.String()
}

func isCommandNotFound(exitCode int, stderr string) bool {
	if exitCode == 127 {
		return true
	}
	lower := strings.ToLower(stderr)
	if strings.Contains(lower, "command not found") {
		return true
	}
	if strings.Contains(lower, "is not recognized as") {
		return true
	}
	if strings.Contains(lower, "not found") {
		return true
	}
	return false
}

func extractFailedBinary(cmd string) string {
	parts := strings.Fields(strings.TrimSpace(cmd))
	if len(parts) == 0 {
		return ""
	}
	for _, part := range parts {
		if part == "sudo" || part == "env" || part == "command" || part == "time" || part == "nohup" || part == "exec" {
			continue
		}
		if strings.Contains(part, "=") && !strings.HasPrefix(part, "./") && !strings.HasPrefix(part, "/") {
			continue
		}
		base := filepath.Base(part)
		if base == "" || base == "." {
			continue
		}
		return base
	}
	return ""
}

func detectPackageSearchCommand(binary string) string {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		return ""
	}
	return detectPackageSearchCommandFor(binary, runtime.GOOS, exec.LookPath)
}

func detectPackageSearchCommandFor(binary, goos string, lookPath func(string) (string, error)) string {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		return ""
	}
	if goos == "windows" {
		if _, err := lookPath("winget"); err == nil {
			return fmt.Sprintf("winget search %q", binary)
		}
		return ""
	}
	if _, err := lookPath("apt-cache"); err == nil {
		return fmt.Sprintf("apt-cache search %q", binary)
	}
	if _, err := lookPath("apt"); err == nil {
		return fmt.Sprintf("apt search %q", binary)
	}
	if _, err := lookPath("dnf"); err == nil {
		return fmt.Sprintf("dnf search %q", binary)
	}
	if _, err := lookPath("pacman"); err == nil {
		return fmt.Sprintf("pacman -sS %q", binary)
	}
	return ""
}

func buildCommandNotFoundFallback(failedCmd string) string {
	binary := extractFailedBinary(failedCmd)
	if binary == "" {
		return ""
	}

	searchCmd := detectPackageSearchCommand(binary)
	quoted := strconv.Quote(binary)
	if runtime.GOOS == "windows" {
		if searchCmd != "" {
			return searchCmd
		}
		return fmt.Sprintf("echo Missing command %s", quoted)
	}

	findCmd := fmt.Sprintf("find . -maxdepth 3 -type f \\( -name %s -o -name %s \\) | head -n 20", quoted, strconv.Quote(binary+".*"))
	if searchCmd == "" {
		return findCmd
	}
	return fmt.Sprintf("%s; echo; echo \"If not found locally, searching packages:\"; %s", findCmd, searchCmd)
}

func detectShellName(shellOverride string) string {
	if s := strings.TrimSpace(shellOverride); s != "" {
		return strings.ToLower(s)
	}
	if s := strings.TrimSpace(os.Getenv("BASHCORRECT_SHELL")); s != "" {
		return strings.ToLower(s)
	}
	if s := strings.TrimSpace(os.Getenv("SHELL")); s != "" {
		base := strings.ToLower(filepath.Base(s))
		if base != "" {
			return base
		}
	}
	if runtime.GOOS == "windows" {
		if strings.TrimSpace(os.Getenv("PSModulePath")) != "" {
			return "powershell"
		}
		if s := strings.TrimSpace(os.Getenv("ComSpec")); s != "" {
			base := strings.ToLower(filepath.Base(s))
			if base != "" {
				return base
			}
		}
	}
	return "unknown"
}

func printDiff(original, corrected string) {
	if original == corrected {
		fmt.Fprintf(os.Stderr, "\033[33mNo change suggested.\033[0m\n")
		return
	}
	fmt.Fprintf(os.Stderr, "\033[31m- %s\033[0m\n", original)
	fmt.Fprintf(os.Stderr, "\033[32m+ %s\033[0m\n", corrected)
}

func promptUser(prompt string) string {
	fmt.Fprint(os.Stderr, prompt)
	line, err := stdinReader.ReadString('\n')
	if err == nil {
		return strings.TrimRight(line, "\r\n")
	}
	if line != "" {
		return strings.TrimRight(line, "\r\n")
	}
	return ""
}

func openInEditor(content string) (string, error) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	tmp, err := os.CreateTemp("", "bashcorrect-*.sh")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.WriteString(content); err != nil {
		return "", err
	}
	tmp.Close()

	c := exec.Command(editor, tmp.Name())
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return "", fmt.Errorf("editor exited with error: %w", err)
	}

	data, err := os.ReadFile(tmp.Name())
	if err != nil {
		return "", err
	}
	return string(data), nil
}

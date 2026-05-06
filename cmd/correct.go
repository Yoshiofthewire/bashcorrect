package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

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

const commandNotFoundDecisionSystemPrompt = `You classify shell command-not-found failures.
Output ONLY compact JSON with keys:
- decision: one of "typo", "install", "unknown"
- corrected_command: string (required only when decision is "typo")
- package: string (required only when decision is "install")

Rules:
- Use decision="typo" only when the command is likely misspelled and you can provide a safer corrected command.
- Use decision="install" when the command appears valid but missing from the system.
- Use decision="unknown" when confidence is low.
- No markdown, no prose, JSON only.`

type commandNotFoundDecision struct {
	Decision         string `json:"decision"`
	CorrectedCommand string `json:"corrected_command"`
	Package          string `json:"package"`
}

func runCorrect(cmd *cobra.Command, _ []string) error {
	p, err := providers.NewActive(cfg)
	if err != nil {
		return err
	}

	if isCommandNotFound(correctExitCodeFlag, correctStderrFlag) {
		suggestion, handled, err := buildCommandNotFoundSuggestion(context.Background(), p, correctCmdFlag, correctExitCodeFlag, correctStderrFlag, correctShellFlag)
		if err != nil {
			return err
		}
		if handled {
			return applySuggestion(correctCmdFlag, suggestion)
		}
	}

	userPrompt := buildCorrectPrompt(correctCmdFlag, correctExitCodeFlag, correctStderrFlag, correctShellFlag)

	stop := startThinkingThrobber("Thinking")
	suggestion, err := p.Query(context.Background(), correctSystemPrompt, userPrompt)
	stop()
	if err != nil {
		return fmt.Errorf("provider error: %w", err)
	}
	suggestion = strings.TrimSpace(suggestion)

	return applySuggestion(correctCmdFlag, suggestion)
}

func applySuggestion(original, suggestion string) error {
	printDiff(original, suggestion)

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

func buildCommandNotFoundSuggestion(ctx context.Context, p providers.Provider, failedCmd string, exitCode int, stderr, shellOverride string) (string, bool, error) {
	decision, err := askAICommandNotFoundDecision(ctx, p, failedCmd, exitCode, stderr, shellOverride)
	if err != nil {
		return "", false, nil
	}

	switch decision.Decision {
	case "typo":
		corrected := strings.TrimSpace(decision.CorrectedCommand)
		if corrected != "" && corrected != strings.TrimSpace(failedCmd) {
			return corrected, true, nil
		}
		return "", false, nil
	case "install":
		pkg := strings.TrimSpace(decision.Package)
		if pkg == "" {
			pkg = extractFailedBinary(failedCmd)
		}
		if pkg != "" {
			if installCmd := detectPackageInstallCommand(pkg); installCmd != "" {
				return fmt.Sprintf("%s && %s", installCmd, failedCmd), true, nil
			}
		}
		if fallback := buildCommandNotFoundFallback(failedCmd); fallback != "" {
			return fallback, true, nil
		}
		return "", false, nil
	case "unknown":
		return "", false, nil
	}

	return "", false, nil
}

func askAICommandNotFoundDecision(ctx context.Context, p providers.Provider, failedCmd string, exitCode int, stderr, shellOverride string) (commandNotFoundDecision, error) {
	userPrompt := buildCommandNotFoundDecisionPrompt(failedCmd, exitCode, stderr, shellOverride)
	stop := startThinkingThrobber("Thinking")
	defer stop()
	resp, err := p.Query(ctx, commandNotFoundDecisionSystemPrompt, userPrompt)
	if err != nil {
		return commandNotFoundDecision{}, err
	}
	return parseCommandNotFoundDecision(resp)
}

func startThinkingThrobber(label string) func() {
	frames := []string{"|", "/", "-", "\\"}
	ticker := time.NewTicker(120 * time.Millisecond)
	done := make(chan struct{})
	stopped := make(chan struct{})

	go func() {
		defer close(stopped)
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-done:
				fmt.Fprint(os.Stderr, "\r\033[2K")
				return
			case <-ticker.C:
				fmt.Fprintf(os.Stderr, "\r\033[2m[%s %s]\033[0m", label, frames[i%len(frames)])
				i++
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			<-stopped
		})
	}
}

func parseCommandNotFoundDecision(raw string) (commandNotFoundDecision, error) {
	var out commandNotFoundDecision
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &out); err != nil {
		return commandNotFoundDecision{}, fmt.Errorf("invalid command-not-found decision response: %w", err)
	}
	out.Decision = strings.ToLower(strings.TrimSpace(out.Decision))
	out.CorrectedCommand = strings.TrimSpace(out.CorrectedCommand)
	out.Package = strings.TrimSpace(out.Package)
	if out.Decision != "typo" && out.Decision != "install" && out.Decision != "unknown" {
		return commandNotFoundDecision{}, fmt.Errorf("invalid command-not-found decision value %q", out.Decision)
	}
	return out, nil
}

func buildCommandNotFoundDecisionPrompt(cmd string, exitCode int, stderr, shellOverride string) string {
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
	if failed := extractFailedBinary(cmd); failed != "" {
		sb.WriteString(fmt.Sprintf("\nMissing executable guess: %s", failed))
	}
	if installer := detectPackageInstallCommand("<package>"); installer != "" {
		sb.WriteString(fmt.Sprintf("\nInstaller template: %s", installer))
	}
	return sb.String()
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

func detectPackageInstallCommand(pkg string) string {
	pkg = strings.TrimSpace(pkg)
	if pkg == "" {
		return ""
	}
	return detectPackageInstallCommandFor(pkg, runtime.GOOS, exec.LookPath)
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

func detectPackageInstallCommandFor(pkg, goos string, lookPath func(string) (string, error)) string {
	pkg = strings.TrimSpace(pkg)
	if pkg == "" {
		return ""
	}
	quoted := strconv.Quote(pkg)
	if goos == "windows" {
		if _, err := lookPath("winget"); err == nil {
			return fmt.Sprintf("winget install %s", quoted)
		}
		return ""
	}
	if goos == "darwin" {
		if _, err := lookPath("brew"); err == nil {
			return fmt.Sprintf("brew install %s", quoted)
		}
	}
	if _, err := lookPath("apt-get"); err == nil {
		return fmt.Sprintf("sudo apt-get install -y %s", quoted)
	}
	if _, err := lookPath("apt"); err == nil {
		return fmt.Sprintf("sudo apt install -y %s", quoted)
	}
	if _, err := lookPath("dnf"); err == nil {
		return fmt.Sprintf("sudo dnf install -y %s", quoted)
	}
	if _, err := lookPath("pacman"); err == nil {
		return fmt.Sprintf("sudo pacman -S --needed %s", quoted)
	}
	if _, err := lookPath("brew"); err == nil {
		return fmt.Sprintf("brew install %s", quoted)
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

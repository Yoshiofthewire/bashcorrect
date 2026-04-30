package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/bashcorrect/bashcorrect/providers"
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
)

func init() {
	correctCmd.Flags().StringVar(&correctCmdFlag, "cmd", "", "the failed command (required)")
	correctCmd.Flags().IntVar(&correctExitCodeFlag, "exit-code", 1, "exit code of the failed command")
	correctCmd.Flags().StringVar(&correctStderrFlag, "stderr", "", "stderr output of the failed command")
	_ = correctCmd.MarkFlagRequired("cmd")
	rootCmd.AddCommand(correctCmd)
}

const correctSystemPrompt = `You are an expert shell assistant. The user ran a command that failed.
Output ONLY the corrected command — no explanation, no markdown, no code fences, just the raw command.
If the command cannot be meaningfully corrected, output the original command unchanged.`

func runCorrect(cmd *cobra.Command, _ []string) error {
	p, err := providers.NewActive(cfg)
	if err != nil {
		return err
	}

	userPrompt := buildCorrectPrompt(correctCmdFlag, correctExitCodeFlag, correctStderrFlag)

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

func buildCorrectPrompt(cmd string, exitCode int, stderr string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Failed command (exit code %d): %s", exitCode, cmd))
	if stderr != "" {
		sb.WriteString(fmt.Sprintf("\nError output:\n%s", stderr))
	}
	return sb.String()
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
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return scanner.Text()
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

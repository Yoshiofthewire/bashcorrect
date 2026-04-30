package cmd

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/bashcorrect/bashcorrect/providers"
	"github.com/spf13/cobra"
)

var queryCmd = &cobra.Command{
	Use:   "query <prompt>",
	Short: "Ask the AI assistant a question or request a shell command",
	Long: `Sends a natural-language prompt to the active AI provider and prints the
response. When --inline is set, only the extracted command is printed (for use
by shell keybindings that replace the current input buffer).`,
	Args: cobra.MinimumNArgs(1),
	RunE: runQuery,
}

var (
	queryInline       bool
	queryCWD          bool
	queryHistoryLines int
)

func init() {
	queryCmd.Flags().BoolVar(&queryInline, "inline", false, "output only the extracted command (for keybinding buffer replacement)")
	queryCmd.Flags().BoolVar(&queryCWD, "cwd", false, "include current working directory in context")
	queryCmd.Flags().IntVar(&queryHistoryLines, "history-lines", 0, "include last N lines of shell history as context")
	rootCmd.AddCommand(queryCmd)
}

const querySystemPrompt = `You are an expert terminal assistant. Answer shell questions concisely.
If your answer includes a command, wrap it in a fenced code block using triple backticks.
Prefer POSIX-compatible commands unless the user specifies a shell.`

const queryInlineSystemPrompt = `You are an expert terminal assistant. The user wants a single shell command.
Output ONLY the raw command — no explanation, no markdown, no code fences.`

func runQuery(_ *cobra.Command, args []string) error {
	prompt := strings.Join(args, " ")

	p, err := providers.NewActive(cfg)
	if err != nil {
		return err
	}

	userPrompt := buildQueryPrompt(prompt, queryCWD, queryHistoryLines)

	sysPrompt := querySystemPrompt
	if queryInline {
		sysPrompt = queryInlineSystemPrompt
	}

	if !queryInline {
		fmt.Fprintf(os.Stderr, "\033[2m[bashcorrect] asking %s...\033[0m\n", p.Name())
	}

	response, err := p.Query(context.Background(), sysPrompt, userPrompt)
	if err != nil {
		return fmt.Errorf("provider error: %w", err)
	}
	response = strings.TrimSpace(response)

	if queryInline {
		fmt.Println(extractCommand(response))
		return nil
	}

	fmt.Println(response)

	// If the response contains a fenced code block, offer to run it
	if cmd := extractCommand(response); cmd != "" && cmd != response {
		answer := promptUser("\nRun this command? [y/N] ")
		if strings.ToLower(strings.TrimSpace(answer)) == "y" {
			fmt.Println(cmd)
		}
	}
	return nil
}

func buildQueryPrompt(prompt string, includeCWD bool, historyLines int) string {
	var sb strings.Builder

	if includeCWD {
		cwd, err := os.Getwd()
		if err == nil {
			sb.WriteString(fmt.Sprintf("Current directory: %s\n", cwd))
		}
	}

	if historyLines > 0 {
		history := readShellHistory(historyLines)
		if history != "" {
			sb.WriteString(fmt.Sprintf("Recent shell history:\n%s\n", history))
		}
	}

	sb.WriteString(prompt)
	return sb.String()
}

// extractCommand pulls the first fenced code block out of a markdown response.
// If none is found, returns the whole response (for inline mode).
var fencedCodeRe = regexp.MustCompile("(?s)```(?:bash|sh|shell|zsh|fish|powershell|ps1|pwsh)?\n?(.*?)```")

func extractCommand(response string) string {
	matches := fencedCodeRe.FindStringSubmatch(response)
	if len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	return strings.TrimSpace(response)
}

func readShellHistory(n int) string {
	histFile := os.Getenv("HISTFILE")
	if histFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		// Try common locations
		for _, candidate := range []string{".zsh_history", ".bash_history", ".history"} {
			path := home + "/" + candidate
			if _, err := os.Stat(path); err == nil {
				histFile = path
				break
			}
		}
	}
	if histFile == "" {
		return ""
	}

	data, err := os.ReadFile(histFile)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

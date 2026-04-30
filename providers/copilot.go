package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Yoshiofthewire/bashcorrect/config"
)

const copilotEndpoint = "https://api.githubcopilot.com/chat/completions"

type copilotProvider struct {
	token string
	model string
}

func newCopilot(pc config.ProviderConfig) Provider {
	model := strings.TrimSpace(pc.Model)
	token := pc.APIKey
	if token == "" {
		token = resolveCopilotToken()
	}
	return &copilotProvider{token: token, model: model}
}

func (p *copilotProvider) Name() string { return "copilot" }

func (p *copilotProvider) Query(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if p.token == "" {
		if res, err := queryViaGhCopilotCLI(ctx, systemPrompt, userPrompt); err == nil {
			return res, nil
		}
		return "", fmt.Errorf("copilot: no token found — set providers.copilot.api_key in config, authenticate via `gh auth login`, or install GitHub Copilot CLI via `gh copilot`")
	}
	body := map[string]any{
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
	}
	if p.model != "" {
		body["model"] = p.model
	}

	extraHeaders := map[string]string{
		"Accept":                  "application/json",
		"Copilot-Integration-Id":  "vscode-chat",
		"Editor-Version":          "vscode/1.0.0",
		"Editor-Plugin-Version":   "bashcorrect/0.1.2",
		"User-Agent":              "bashcorrect/0.1.2",
		"X-Request-Source":        "bashcorrect",
	}

	res, err := doOpenAICompatRequestWithHeaders(ctx, copilotEndpoint, "Bearer "+p.token, body, extraHeaders, "copilot")
	if err != nil {
		msg := err.Error()
		if p.model != "" && strings.Contains(msg, "model_not_supported") {
			delete(body, "model")
			res, retryErr := doOpenAICompatRequestWithHeaders(ctx, copilotEndpoint, "Bearer "+p.token, body, extraHeaders, "copilot")
			if retryErr == nil {
				return res, nil
			}
			return "", fmt.Errorf("copilot rejected model %q (model_not_supported). Leave providers.copilot.model empty for auto-selection or set a supported model for your account: %w", p.model, retryErr)
		}
		if strings.Contains(msg, fmt.Sprintf("API error %d", http.StatusForbidden)) {
			if cliRes, cliErr := queryViaGhCopilotCLI(ctx, systemPrompt, userPrompt); cliErr == nil {
				return cliRes, nil
			}
			return "", fmt.Errorf("copilot endpoint forbidden (403): your token is valid but does not have Copilot chat endpoint access. Ensure this account has an active GitHub Copilot seat and try `gh auth refresh -h github.com -s read:org -s gist`; alternatively install/use `gh copilot` locally or set providers.copilot.api_key to a Copilot-compatible token")
		}
		return "", err
	}
	return res, nil
}

func queryViaGhCopilotCLI(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	prompt := buildGhCopilotPrompt(systemPrompt, userPrompt)
	cmd := exec.CommandContext(ctx, "gh", "copilot", "-p", prompt)
	cmd.Env = append(os.Environ(),
		"GH_PROMPT_DISABLED=1",
		"NO_COLOR=1",
		"CLICOLOR=0",
		"CLICOLOR_FORCE=0",
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("gh copilot CLI failed: %s", msg)
	}

	response := cleanGhCopilotOutput(stdout.String())
	if response == "" {
		response = cleanGhCopilotOutput(stderr.String())
	}
	if response == "" {
		return "", fmt.Errorf("gh copilot CLI returned no output")
	}
	return response, nil
}

func buildGhCopilotPrompt(systemPrompt, userPrompt string) string {
	if systemPrompt == "" {
		return userPrompt
	}
	return strings.TrimSpace(systemPrompt) + "\n\nUser request:\n" + strings.TrimSpace(userPrompt)
}

var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func cleanGhCopilotOutput(s string) string {
	s = ansiEscapeRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r", "")
	return strings.TrimSpace(s)
}

// resolveCopilotToken attempts to find a GitHub Copilot token from the CLI auth
// store (~/.config/github-copilot/hosts.json) or the GITHUB_TOKEN env var.
func resolveCopilotToken() string {
	for _, envName := range []string{"GITHUB_TOKEN", "GH_TOKEN", "COPILOT_TOKEN"} {
		if tok := strings.TrimSpace(os.Getenv(envName)); tok != "" {
			return tok
		}
	}

	for _, hostsFile := range copilotHostsFiles() {
		if tok := readTokenFromCopilotHostsJSON(hostsFile); tok != "" {
			return tok
		}
	}

	// Fallback to GitHub CLI auth if available and logged in.
	if tok := readTokenFromGhCLI(); tok != "" {
		return tok
	}

	return ""
}


func readTokenFromCopilotHostsJSON(hostsFile string) string {
	data, err := os.ReadFile(hostsFile)
	if err != nil {
		return ""
	}

	// hosts.json structure: { "github.com": { "oauth_token": "..." } }
	var hosts map[string]struct {
		OAuthToken string `json:"oauth_token"`
	}
	if err := json.Unmarshal(data, &hosts); err != nil {
		return ""
	}
	if h, ok := hosts["github.com"]; ok {
		return strings.TrimSpace(h.OAuthToken)
	}
	return ""
}

func readTokenFromGhCLI() string {
	cmd := exec.Command("gh", "auth", "token")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func copilotHostsFiles() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, ".config", "github-copilot", "hosts.json"),
		filepath.Join(home, ".config", "GitHub Copilot", "hosts.json"),
	}
}

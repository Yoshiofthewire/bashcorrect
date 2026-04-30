package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
		return "", fmt.Errorf("copilot: no token found — set providers.copilot.api_key in config or authenticate via `gh auth login`")
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
			return "", fmt.Errorf("copilot endpoint forbidden (403): your token is valid but does not have Copilot chat endpoint access. Ensure this account has an active GitHub Copilot seat and try `gh auth refresh -h github.com -s read:org -s gist`; alternatively set providers.copilot.api_key to a Copilot-compatible token")
		}
		return "", err
	}
	return res, nil
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

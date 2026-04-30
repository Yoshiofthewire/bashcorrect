package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Yoshiofthewire/bashcorrect/config"
)

const copilotEndpoint = "https://api.githubcopilot.com/chat/completions"
const copilotDefaultModel = "gpt-4o"

type copilotProvider struct {
	token string
	model string
}

func newCopilot(pc config.ProviderConfig) Provider {
	model := pc.Model
	if model == "" {
		model = copilotDefaultModel
	}
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
		"model": p.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
	}
	return doOpenAICompatRequest(ctx, copilotEndpoint, "Bearer "+p.token, body)
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

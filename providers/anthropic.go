package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/bashcorrect/bashcorrect/config"
)

const anthropicEndpoint = "https://api.anthropic.com/v1/messages"
const anthropicVersion = "2023-06-01"
const anthropicDefaultModel = "claude-3-5-sonnet-20241022"
const anthropicMaxTokens = 4096

type anthropicProvider struct {
	apiKey string
	model  string
}

func newAnthropic(pc config.ProviderConfig) Provider {
	model := pc.Model
	if model == "" {
		model = anthropicDefaultModel
	}
	return &anthropicProvider{apiKey: pc.APIKey, model: model}
}

func (p *anthropicProvider) Name() string { return "anthropic" }

func (p *anthropicProvider) Query(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if p.apiKey == "" {
		return "", fmt.Errorf("anthropic: api_key not set — run `bashcorrect config set providers.anthropic.api_key <key>`")
	}

	body := map[string]any{
		"model":      p.model,
		"max_tokens": anthropicMaxTokens,
		"system":     systemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": userPrompt},
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicEndpoint, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anthropic API error %d: %s", resp.StatusCode, string(raw))
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", err
	}
	if result.Error != nil {
		return "", fmt.Errorf("anthropic: %s", result.Error.Message)
	}
	for _, c := range result.Content {
		if c.Type == "text" {
			return c.Text, nil
		}
	}
	return "", fmt.Errorf("anthropic: no text content in response")
}

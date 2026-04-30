package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/Yoshiofthewire/bashcorrect/config"
)

const openAIEndpoint = "https://api.openai.com/v1/chat/completions"
const openAIDefaultModel = "gpt-4o"

type openAIProvider struct {
	apiKey string
	model  string
}

func newOpenAI(pc config.ProviderConfig) Provider {
	model := pc.Model
	if model == "" {
		model = openAIDefaultModel
	}
	return &openAIProvider{apiKey: pc.APIKey, model: model}
}

func (p *openAIProvider) Name() string { return "openai" }

func (p *openAIProvider) Query(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if p.apiKey == "" {
		return "", fmt.Errorf("openai: api_key not set — run `bashcorrect config set providers.openai.api_key <key>`")
	}

	body := map[string]any{
		"model": p.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
	}
	return doOpenAICompatRequest(ctx, openAIEndpoint, "Bearer "+p.apiKey, body)
}

// openAICompatRequest is shared by openai and copilot providers.
func doOpenAICompatRequest(ctx context.Context, endpoint, authHeader string, body map[string]any) (string, error) {
	return doOpenAICompatRequestWithHeaders(ctx, endpoint, authHeader, body, nil, "openai")
}

func doOpenAICompatRequestWithHeaders(ctx context.Context, endpoint, authHeader string, body map[string]any, extraHeaders map[string]string, providerName string) (string, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s request: %w", providerName, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s API error %d: %s", providerName, resp.StatusCode, string(raw))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", err
	}
	if result.Error != nil {
		return "", fmt.Errorf("%s: %s", providerName, result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("%s: empty response", providerName)
	}
	return result.Choices[0].Message.Content, nil
}

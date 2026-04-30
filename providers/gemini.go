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

const geminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/models"
const geminiDefaultModel = "gemini-1.5-pro"

type geminiProvider struct {
	apiKey string
	model  string
}

func newGemini(pc config.ProviderConfig) Provider {
	model := pc.Model
	if model == "" {
		model = geminiDefaultModel
	}
	return &geminiProvider{apiKey: pc.APIKey, model: model}
}

func (p *geminiProvider) Name() string { return "gemini" }

func (p *geminiProvider) Query(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if p.apiKey == "" {
		return "", fmt.Errorf("gemini: api_key not set — run `bashcorrect config set providers.gemini.api_key <key>`")
	}

	url := fmt.Sprintf("%s/%s:generateContent?key=%s", geminiBaseURL, p.model, p.apiKey)

	body := map[string]any{
		"system_instruction": map[string]any{
			"parts": []map[string]string{
				{"text": systemPrompt},
			},
		},
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]string{
					{"text": userPrompt},
				},
			},
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini API error %d: %s", resp.StatusCode, string(raw))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", err
	}
	if result.Error != nil {
		return "", fmt.Errorf("gemini: %s", result.Error.Message)
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini: empty response")
	}
	return result.Candidates[0].Content.Parts[0].Text, nil
}

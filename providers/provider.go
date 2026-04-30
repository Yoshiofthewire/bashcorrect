package providers

import (
	"context"
	"fmt"

	"github.com/bashcorrect/bashcorrect/config"
)

// Provider is the common interface for all AI backends.
type Provider interface {
	// Name returns the canonical lowercase name of this provider (e.g. "openai").
	Name() string
	// Query sends a prompt to the AI and returns the text response.
	Query(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// New instantiates the named provider using the given config.
func New(name string, cfg config.Config) (Provider, error) {
	pc := cfg.ProviderCfg(name)
	switch name {
	case "openai":
		return newOpenAI(pc), nil
	case "anthropic":
		return newAnthropic(pc), nil
	case "gemini":
		return newGemini(pc), nil
	case "copilot":
		return newCopilot(pc), nil
	default:
		return nil, fmt.Errorf("unknown provider %q — valid choices: openai, anthropic, gemini, copilot", name)
	}
}

// NewActive instantiates the provider set as active in cfg.
func NewActive(cfg config.Config) (Provider, error) {
	return New(cfg.ActiveProvider, cfg)
}

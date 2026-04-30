package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const defaultConfigFile = "config.toml"
const appName = "bashcorrect"

// ProviderConfig holds per-provider credentials and model selection.
type ProviderConfig struct {
	APIKey string `toml:"api_key"`
	Model  string `toml:"model"`
}

// AutocorrectConfig controls autocorrect behaviour.
type AutocorrectConfig struct {
	AutoRun  bool `toml:"auto_run"`
	ShowDiff bool `toml:"show_diff"`
}

// Config is the top-level config struct.
type Config struct {
	ActiveProvider string                    `toml:"active_provider"`
	TriggerPrefix  string                    `toml:"trigger_prefix"`
	Providers      map[string]ProviderConfig `toml:"providers"`
	Autocorrect    AutocorrectConfig         `toml:"autocorrect"`

	// Path is the file this config was loaded from (not persisted).
	Path string `toml:"-"`
}

// DefaultConfig returns a Config pre-populated with sensible defaults.
func DefaultConfig() Config {
	return Config{
		ActiveProvider: "openai",
		TriggerPrefix:  "?",
		Providers: map[string]ProviderConfig{
			"openai": {Model: "gpt-4o"},
			"anthropic": {Model: "claude-3-5-sonnet-20241022"},
			"gemini": {Model: "gemini-1.5-pro"},
			"copilot": {Model: "gpt-4o"},
		},
		Autocorrect: AutocorrectConfig{
			AutoRun:  false,
			ShowDiff: true,
		},
	}
}

// Dir returns the platform-appropriate config directory for bashcorrect.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("could not determine config directory: %w", err)
	}
	return filepath.Join(base, appName), nil
}

// FilePath returns the full path to the config file, using override if non-empty.
func FilePath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, defaultConfigFile), nil
}

// Load reads the config file at the given path (or the default location).
// If the file does not exist, it returns the default config.
func Load(override string) (Config, error) {
	path, err := FilePath(override)
	if err != nil {
		return Config{}, err
	}

	cfg := DefaultConfig()
	cfg.Path = path

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if err := Save(cfg); err != nil {
				return Config{}, fmt.Errorf("creating default config %s: %w", path, err)
			}
			return cfg, nil
		}
		return Config{}, fmt.Errorf("reading config %s: %w", path, err)
	}

	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config %s: %w", path, err)
	}
	cfg.Path = path
	return cfg, nil
}

// Save writes cfg to cfg.Path, creating parent directories as needed.
func Save(cfg Config) error {
	if cfg.Path == "" {
		path, err := FilePath("")
		if err != nil {
			return err
		}
		cfg.Path = path
	}

	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	f, err := os.OpenFile(cfg.Path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	defer f.Close()

	return toml.NewEncoder(f).Encode(cfg)
}

// ProviderCfg returns the ProviderConfig for the named provider, or an empty one if not set.
func (c Config) ProviderCfg(name string) ProviderConfig {
	if c.Providers == nil {
		return ProviderConfig{}
	}
	return c.Providers[name]
}

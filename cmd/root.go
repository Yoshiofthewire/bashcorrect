package cmd

import (
	"fmt"
	"os"

	"github.com/Yoshiofthewire/bashcorrect/config"
	"github.com/spf13/cobra"
)

var (
	cfgFile      string
	providerFlag string
	modelFlag    string
	cfg          config.Config
	version      = "0.1.2"
)

var rootCmd = &cobra.Command{
	Use:   "bashcorrect",
	Short: "AI-powered shell autocorrect and assistant",
	Long: `BashCorrect hooks into your shell to automatically suggest fixes for failed
commands and lets you ask your AI assistant anything directly from the terminal.

Supported providers: openai, anthropic, gemini, copilot
Supported shells:    bash, zsh, fish, powershell`,
	Version: version,
}

// Execute is the entry point called from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: $XDG_CONFIG_HOME/bashcorrect/config.toml)")
	rootCmd.PersistentFlags().StringVar(&providerFlag, "provider", "", "AI provider to use for this invocation (overrides config)")
	rootCmd.PersistentFlags().StringVar(&modelFlag, "model", "", "model to use for this invocation (overrides config)")
}

func initConfig() {
	var err error
	cfg, err = config.Load(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not load config: %v\n", err)
		cfg = config.DefaultConfig()
	}

	if providerFlag != "" {
		cfg.ActiveProvider = providerFlag
	}
	if modelFlag != "" {
		pc := cfg.ProviderCfg(cfg.ActiveProvider)
		pc.Model = modelFlag
		if cfg.Providers == nil {
			cfg.Providers = make(map[string]config.ProviderConfig)
		}
		cfg.Providers[cfg.ActiveProvider] = pc
	}
}

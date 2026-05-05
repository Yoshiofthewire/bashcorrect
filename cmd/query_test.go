package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestBuildVaultQueryContextIncludesVaultMemorySoulToolsAndRelevantPages(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	vaultPath, err := resolveVaultPath("")
	if err != nil {
		t.Fatalf("resolveVaultPath: %v", err)
	}

	if err := initVaultAtPath(vaultPath, false, false); err != nil {
		t.Fatalf("initVaultAtPath: %v", err)
	}

	profile := vaultBootstrapProfile{
		BotName:            "Copilot",
		BotNature:          "AI pair programmer",
		BotVibe:            "direct and pragmatic",
		BotEmoji:           ":robot:",
		UserName:           "Yoshi",
		AddressAs:          "yoshi",
		Timezone:           "Asia/Tokyo",
		UserNotes:          "prefers concise answers",
		SoulPriorities:     "engineering quality and speed",
		SoulBoundaries:     "no fluff or cheerleading",
		SoulPreferences:    "be direct and specific",
		CompletedAtRFC3339: "2026-05-05T00:00:00Z",
	}
	if err := runVaultBootstrapWithProfile(vaultPath, profile); err != nil {
		t.Fatalf("runVaultBootstrapWithProfile: %v", err)
	}

	relevantPage := filepath.Join(vaultPath, "concepts", "docker-memory.md")
	if err := os.WriteFile(relevantPage, []byte("# Docker Memory\n\nStore docker troubleshooting notes and memory references here.\n"), 0o600); err != nil {
		t.Fatalf("write relevant page: %v", err)
	}

	systemPrompt, userPrompt := buildVaultQueryContext("how should I use docker memory", vaultPath)

	for _, want := range []string{"local BashCorrect memory vault", "AI pair programmer", "engineering quality and speed", "verify-memory-files"} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("system prompt missing %q\n%s", want, systemPrompt)
		}
	}

	for _, want := range []string{"Local vault path:", "Durable memory:", "Relevant vault pages:", "concepts/docker-memory.md", "prefers concise answers"} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("user prompt missing %q\n%s", want, userPrompt)
		}
	}
}

func TestBuildVaultQueryContextMatchesNormalizedTermsAcrossVaultPages(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	vaultPath, err := resolveVaultPath("")
	if err != nil {
		t.Fatalf("resolveVaultPath: %v", err)
	}

	if err := initVaultAtPath(vaultPath, false, false); err != nil {
		t.Fatalf("initVaultAtPath: %v", err)
	}

	pagePath := filepath.Join(vaultPath, "concepts", "release-playbook.md")
	content := "# Deploying Workloads\n\nUse this rollout checklist before shipping services.\n"
	if err := os.WriteFile(pagePath, []byte(content), 0o600); err != nil {
		t.Fatalf("write release playbook: %v", err)
	}

	_, userPrompt := buildVaultQueryContext("deployment rollouts", vaultPath)
	for _, want := range []string{"concepts/release-playbook.md", "Deploying Workloads", "rollout checklist"} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("normalized retrieval missing %q\n%s", want, userPrompt)
		}
	}
}

func TestBuildVaultQueryContextAnchorsWeatherToExplicitLocation(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	vaultPath, err := resolveVaultPath("")
	if err != nil {
		t.Fatalf("resolveVaultPath: %v", err)
	}

	if err := initVaultAtPath(vaultPath, false, false); err != nil {
		t.Fatalf("initVaultAtPath: %v", err)
	}

	userPath := filepath.Join(vaultPath, "USER.md")
	userData := `# USER

- name: Yoshi
- address_as: yoshi
- timezone: Asia/Tokyo
- location: Austin, TX
- notes:
  - prefers concise answers
`
	if err := os.WriteFile(userPath, []byte(userData), 0o600); err != nil {
		t.Fatalf("write USER.md: %v", err)
	}

	systemPrompt, userPrompt := buildVaultQueryContext("what's the weather today", vaultPath)
	if !strings.Contains(systemPrompt, "exact preferred location") || !strings.Contains(systemPrompt, "Austin, TX") {
		t.Fatalf("weather directive missing explicit location\n%s", systemPrompt)
	}
	if !strings.Contains(userPrompt, "Weather location anchor: Austin, TX") {
		t.Fatalf("weather anchor missing explicit location\n%s", userPrompt)
	}
}

func TestBuildVaultQueryContextAnchorsWeatherToTimezoneFallback(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	vaultPath, err := resolveVaultPath("")
	if err != nil {
		t.Fatalf("resolveVaultPath: %v", err)
	}

	if err := initVaultAtPath(vaultPath, false, false); err != nil {
		t.Fatalf("initVaultAtPath: %v", err)
	}

	userPath := filepath.Join(vaultPath, "USER.md")
	userData := `# USER

- name: Yoshi
- address_as: yoshi
- timezone: Asia/Tokyo
- notes:
  - prefers concise answers
`
	if err := os.WriteFile(userPath, []byte(userData), 0o600); err != nil {
		t.Fatalf("write USER.md: %v", err)
	}

	systemPrompt, userPrompt := buildVaultQueryContext("weather tomorrow", vaultPath)
	if !strings.Contains(systemPrompt, "Tokyo") || !strings.Contains(systemPrompt, "USER.md timezone") {
		t.Fatalf("weather directive missing timezone fallback\n%s", systemPrompt)
	}
	if !strings.Contains(userPrompt, "Weather location anchor: Tokyo") {
		t.Fatalf("weather anchor missing timezone fallback\n%s", userPrompt)
	}
}

func TestBuildQueryPromptIncludesFileContext(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "sample.txt")
	content := "alpha\nbeta\ngamma\n"
	if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
		t.Fatalf("write sample file: %v", err)
	}

	prompt := buildQueryPrompt("Summarize it", false, 0, []string{filePath})
	if !strings.Contains(prompt, "Files provided by user:") {
		t.Fatalf("missing file context header\n%s", prompt)
	}
	if !strings.Contains(prompt, "alpha") || !strings.Contains(prompt, "gamma") {
		t.Fatalf("missing file content in prompt\n%s", prompt)
	}
	if !strings.Contains(prompt, "Summarize it") {
		t.Fatalf("missing user prompt text\n%s", prompt)
	}
}

func TestValidateQueryArgsAllowsSummarizeWithoutPositionalPrompt(t *testing.T) {
	oldFiles := queryFiles
	oldSummarize := querySummarize
	defer func() {
		queryFiles = oldFiles
		querySummarize = oldSummarize
	}()

	queryFiles = []string{"README.md"}
	querySummarize = true
	if err := validateQueryArgs(&cobra.Command{}, nil); err != nil {
		t.Fatalf("expected summarize mode validation to pass: %v", err)
	}

	queryFiles = nil
	querySummarize = false
	if err := validateQueryArgs(&cobra.Command{}, nil); err == nil {
		t.Fatalf("expected validation to fail without prompt or summarize flags")
	}
}

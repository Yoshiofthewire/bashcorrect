package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitVaultAndBootstrapGenerateMemoryFilesAndValidationTools(t *testing.T) {
	vaultPath := filepath.Join(t.TempDir(), "vault")
	if err := initVaultAtPath(vaultPath, false, false); err != nil {
		t.Fatalf("initVaultAtPath: %v", err)
	}

	toolsPath := filepath.Join(vaultPath, "TOOLS.md")
	toolsData, err := os.ReadFile(toolsPath)
	if err != nil {
		t.Fatalf("read TOOLS.md: %v", err)
	}
	toolsContent := string(toolsData)
	for _, want := range []string{"verify-memory-files", "verify-vault-layout"} {
		if !strings.Contains(toolsContent, want) {
			t.Fatalf("TOOLS.md missing %q\n%s", want, toolsContent)
		}
	}

	profile := vaultBootstrapProfile{
		BotName:            "Copilot",
		BotNature:          "AI pair programmer",
		BotVibe:            "direct and pragmatic",
		BotEmoji:           ":robot:",
		UserName:           "Yoshi",
		AddressAs:          "yoshi",
		Timezone:           "Asia/Tokyo",
		WeatherLocation:    "Austin, TX",
		UserNotes:          "prefers concise answers",
		SoulPriorities:     "engineering quality and speed",
		SoulBoundaries:     "no fluff or cheerleading",
		SoulPreferences:    "be direct and specific",
		CompletedAtRFC3339: "2026-05-05T00:00:00Z",
	}
	if err := runVaultBootstrapWithProfile(vaultPath, profile); err != nil {
		t.Fatalf("runVaultBootstrapWithProfile: %v", err)
	}

	for _, rel := range []string{"IDENTITY.md", "USER.md", "SOUL.md", "MEMORY.md"} {
		path := filepath.Join(vaultPath, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if strings.TrimSpace(string(data)) == "" {
			t.Fatalf("%s is empty", rel)
		}
	}

	userData, err := os.ReadFile(filepath.Join(vaultPath, "USER.md"))
	if err != nil {
		t.Fatalf("read USER.md: %v", err)
	}
	if !strings.Contains(string(userData), "weather_location: Austin, TX") {
		t.Fatalf("USER.md missing weather location\n%s", string(userData))
	}

	memoryData, err := os.ReadFile(filepath.Join(vaultPath, "MEMORY.md"))
	if err != nil {
		t.Fatalf("read MEMORY.md: %v", err)
	}
	memoryContent := string(memoryData)
	for _, want := range []string{"## Bootstrap Seed", "## Bootstrap Profile", "assistant_name: Copilot", "user_name: Yoshi", "weather_location: Austin, TX"} {
		if !strings.Contains(memoryContent, want) {
			t.Fatalf("MEMORY.md missing %q\n%s", want, memoryContent)
		}
	}

	largeMemory, err := readLargeMemoryContext(vaultPath, 20, 10)
	if err != nil {
		t.Fatalf("readLargeMemoryContext: %v", err)
	}
	if !strings.Contains(largeMemory, "weather_location: Austin, TX") {
		t.Fatalf("SQLite memory missing weather location\n%s", largeMemory)
	}

	if _, err := os.Stat(filepath.Join(vaultPath, "BOOTSTRAP.md")); !os.IsNotExist(err) {
		t.Fatalf("BOOTSTRAP.md should be removed after bootstrap, got err=%v", err)
	}
}

func TestUpdateVaultWeatherLocationUpdatesUserAndMemory(t *testing.T) {
	vaultPath := filepath.Join(t.TempDir(), "vault")
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
		WeatherLocation:    "Austin, TX",
		UserNotes:          "prefers concise answers",
		SoulPriorities:     "engineering quality and speed",
		SoulBoundaries:     "no fluff or cheerleading",
		SoulPreferences:    "be direct and specific",
		CompletedAtRFC3339: "2026-05-05T00:00:00Z",
	}
	if err := runVaultBootstrapWithProfile(vaultPath, profile); err != nil {
		t.Fatalf("runVaultBootstrapWithProfile: %v", err)
	}

	if err := updateVaultWeatherLocation(vaultPath, "Berlin, DE"); err != nil {
		t.Fatalf("updateVaultWeatherLocation: %v", err)
	}

	userData, err := os.ReadFile(filepath.Join(vaultPath, "USER.md"))
	if err != nil {
		t.Fatalf("read USER.md: %v", err)
	}
	userContent := string(userData)
	if !strings.Contains(userContent, "weather_location: Berlin, DE") {
		t.Fatalf("USER.md did not update weather location\n%s", userContent)
	}
	if strings.Contains(userContent, "weather_location: Austin, TX") {
		t.Fatalf("USER.md still contains old weather location\n%s", userContent)
	}

	memoryData, err := os.ReadFile(filepath.Join(vaultPath, "MEMORY.md"))
	if err != nil {
		t.Fatalf("read MEMORY.md: %v", err)
	}
	memoryContent := string(memoryData)
	if !strings.Contains(memoryContent, "weather_location: Berlin, DE") {
		t.Fatalf("MEMORY.md did not update weather location\n%s", memoryContent)
	}
	if strings.Contains(memoryContent, "weather_location: Austin, TX") {
		t.Fatalf("MEMORY.md still contains old weather location\n%s", memoryContent)
	}

	largeMemory, err := readLargeMemoryContext(vaultPath, 20, 10)
	if err != nil {
		t.Fatalf("readLargeMemoryContext: %v", err)
	}
	if !strings.Contains(largeMemory, "weather_location: Berlin, DE") {
		t.Fatalf("SQLite memory did not update weather location\n%s", largeMemory)
	}
	if strings.Contains(largeMemory, "weather_location: Austin, TX") {
		t.Fatalf("SQLite memory still contains old weather location\n%s", largeMemory)
	}
}

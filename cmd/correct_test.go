package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBuildCorrectPromptIncludesOSAndShell(t *testing.T) {
	prompt := buildCorrectPrompt("ls /nope", 2, "no such file", "zsh")

	if !strings.Contains(prompt, "OS: "+runtime.GOOS) {
		t.Fatalf("expected prompt to include OS %q; got: %s", runtime.GOOS, prompt)
	}
	if !strings.Contains(prompt, "Shell: zsh") {
		t.Fatalf("expected prompt to include shell; got: %s", prompt)
	}
	if !strings.Contains(prompt, "Failed command (exit code 2): ls /nope") {
		t.Fatalf("expected failed command details; got: %s", prompt)
	}
	if !strings.Contains(prompt, "Error output:\nno such file") {
		t.Fatalf("expected stderr details; got: %s", prompt)
	}
}

func TestBuildCorrectPromptWithoutStderr(t *testing.T) {
	prompt := buildCorrectPrompt("pwd", 1, "", "bash")

	if strings.Contains(prompt, "Error output:") {
		t.Fatalf("did not expect stderr section; got: %s", prompt)
	}
}

func TestDetectShellNameUsesOverride(t *testing.T) {
	got := detectShellName("fish")
	if got != "fish" {
		t.Fatalf("expected fish, got %q", got)
	}
}

func TestIsCommandNotFoundByExitCode(t *testing.T) {
	if !isCommandNotFound(127, "") {
		t.Fatal("expected exit code 127 to be treated as command-not-found")
	}
}

func TestBuildCorrectPromptCommandNotFoundIncludesFileHint(t *testing.T) {
	prompt := buildCorrectPrompt("notrealcmd --help", 127, "", "bash")

	if !strings.Contains(prompt, "Command-not-found mode: true") {
		t.Fatalf("expected command-not-found mode in prompt; got: %s", prompt)
	}
	if !strings.Contains(prompt, "Local file check command:") {
		t.Fatalf("expected local file check guidance in prompt; got: %s", prompt)
	}
}

func TestExtractFailedBinarySkipsWrappers(t *testing.T) {
	got := extractFailedBinary("sudo FOO=bar mytool --version")
	if got != "mytool" {
		t.Fatalf("expected mytool, got %q", got)
	}
}

func TestDetectPackageSearchCommandForLinuxPrefersAptCache(t *testing.T) {
	look := func(name string) (string, error) {
		if name == "apt-cache" {
			return "/usr/bin/apt-cache", nil
		}
		return "", fmt.Errorf("not found")
	}

	got := detectPackageSearchCommandFor("jq", "linux", look)
	if got != `apt-cache search "jq"` {
		t.Fatalf("expected apt-cache search command, got %q", got)
	}
}

func TestDetectPackageSearchCommandForWindowsUsesWinget(t *testing.T) {
	look := func(name string) (string, error) {
		if name == "winget" {
			return "C:/Windows/System32/winget.exe", nil
		}
		return "", fmt.Errorf("not found")
	}

	got := detectPackageSearchCommandFor("ripgrep", "windows", look)
	if got != `winget search "ripgrep"` {
		t.Fatalf("expected winget search command, got %q", got)
	}
}

func TestBuildCommandNotFoundFallbackIncludesFindAndSearch(t *testing.T) {
	fallback := buildCommandNotFoundFallback("nosuchcmd --help")

	if fallback == "" {
		t.Fatal("expected non-empty fallback command")
	}
	if runtime.GOOS != "windows" && !strings.Contains(fallback, "find . -maxdepth 3") {
		t.Fatalf("expected local file check in fallback, got: %s", fallback)
	}
}

func TestBuildCorrectPromptIncludesOSReleaseContext(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only /etc/os-release context test")
	}

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "os-release")
	content := "NAME=TestOS\nID=testos\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp os-release: %v", err)
	}
	t.Setenv("BASHCORRECT_OS_RELEASE_PATH", path)

	prompt := buildCorrectPrompt("echo hi", 1, "", "bash")
	if !strings.Contains(prompt, "OS release data (cat /etc/os-release):") {
		t.Fatalf("expected os-release header; got: %s", prompt)
	}
	if !strings.Contains(prompt, "NAME=TestOS") {
		t.Fatalf("expected os-release content; got: %s", prompt)
	}
}

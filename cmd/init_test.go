package cmd

import (
	"reflect"
	"testing"
)

func TestInitTargetShellsDetectsCurrentShellByDefault(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/zsh")

	got, err := initTargetShells("")
	if err != nil {
		t.Fatalf("initTargetShells default: %v", err)
	}
	want := []string{"zsh"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected shells: got %v want %v", got, want)
	}
}

func TestInitTargetShellsFallsBackToAllWhenUnknown(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/tcsh")

	got, err := initTargetShells("")
	if err != nil {
		t.Fatalf("initTargetShells default fallback: %v", err)
	}
	want := []string{"bash", "zsh", "fish", "powershell"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected shells: got %v want %v", got, want)
	}
}

func TestInitTargetShellsSingle(t *testing.T) {
	got, err := initTargetShells("zsh")
	if err != nil {
		t.Fatalf("initTargetShells zsh: %v", err)
	}
	want := []string{"zsh"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected shells: got %v want %v", got, want)
	}
}

func TestInitTargetShellsInvalid(t *testing.T) {
	if _, err := initTargetShells("tcsh"); err == nil {
		t.Fatal("expected error for unsupported shell")
	}
}

func TestDetectShellNameForInit(t *testing.T) {
	tests := []struct {
		name string
		env  string
		goos string
		want string
	}{
		{name: "bash", env: "/bin/bash", goos: "linux", want: "bash"},
		{name: "zsh", env: "/usr/bin/zsh", goos: "linux", want: "zsh"},
		{name: "fish", env: "/usr/local/bin/fish", goos: "linux", want: "fish"},
		{name: "windows default", env: "", goos: "windows", want: "powershell"},
		{name: "unknown", env: "/bin/tcsh", goos: "linux", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectShellNameForInit(tt.env, tt.goos); got != tt.want {
				t.Fatalf("detectShellNameForInit(%q, %q) = %q; want %q", tt.env, tt.goos, got, tt.want)
			}
		})
	}
}

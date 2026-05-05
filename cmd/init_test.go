package cmd

import (
	"reflect"
	"testing"
)

func TestInitTargetShellsAllByDefault(t *testing.T) {
	got, err := initTargetShells("")
	if err != nil {
		t.Fatalf("initTargetShells default: %v", err)
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

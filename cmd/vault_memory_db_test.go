package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureMemoryDBAndReadContext(t *testing.T) {
	vaultPath := filepath.Join(t.TempDir(), "vault")
	if err := os.MkdirAll(filepath.Join(vaultPath, ".bashcorrect-vault", "cache"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := ensureMemoryDB(vaultPath); err != nil {
		t.Fatalf("ensureMemoryDB: %v", err)
	}
	if _, err := os.Stat(memoryDBPath(vaultPath)); err != nil {
		t.Fatalf("memory db missing: %v", err)
	}

	if err := upsertMemoryKV(vaultPath, "weather_location", "Austin, TX"); err != nil {
		t.Fatalf("upsertMemoryKV: %v", err)
	}
	if err := appendMemoryNote(vaultPath, "bootstrap", "first note", "USER.md"); err != nil {
		t.Fatalf("appendMemoryNote: %v", err)
	}

	context, err := readLargeMemoryContext(vaultPath, 20, 10)
	if err != nil {
		t.Fatalf("readLargeMemoryContext: %v", err)
	}
	if !strings.Contains(context, "weather_location: Austin, TX") {
		t.Fatalf("context missing kv entry: %s", context)
	}
	if !strings.Contains(context, "first note") {
		t.Fatalf("context missing note: %s", context)
	}
}

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVersionFlag(t *testing.T) {
	if code := runMain([]string{"--version"}); code != 0 {
		t.Fatalf("--version exit = %d, want 0", code)
	}
}

func TestNoArgsBootsGUI(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "boot.png")
	code := runMain([]string{"--dir", dir, "--png", out})
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected a rendered frame: %v", err)
	}
}

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLITTSWritesAClip(t *testing.T) {
	configDir := t.TempDir()
	// The cache path is pinned inside the throwaway config dir so the command
	// writes nothing into the working tree.
	configYAML := "paths:\n  cache: " + filepath.Join(configDir, "cache") +
		"\nmedia:\n  tts:\n    type: builtin\n    default_voice: narrator\n"
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "run", ".", "tts", "hello there")
	cmd.Env = append(os.Environ(), "LOCALRPG_CONFIG_DIR="+configDir)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tts failed: %v: %s", err, out)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	line := strings.TrimSpace(lines[len(lines)-1])
	if _, err := os.Stat(line); err != nil {
		t.Errorf("expected %q to be a written clip path: %v", line, err)
	}
}

func TestCLIImageCommand(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "image", "A tavern in the mist")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %v, output: %s", err, string(out))
	}

	if !strings.Contains(string(out), "Generating image: A tavern in the mist") {
		t.Errorf("unexpected output: %s", string(out))
	}
}

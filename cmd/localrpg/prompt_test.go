package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCLIPromptCommand(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "prompt", "--cmd", "echo", "Hello adventurer")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %v, output: %s", err, string(out))
	}

	if !strings.Contains(string(out), "Hello adventurer") {
		t.Errorf("expected prompt output, got: %s", string(out))
	}
}

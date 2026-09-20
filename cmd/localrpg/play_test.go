package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCLIPlayMissingArg(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "play")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error on missing game-id argument")
	}

	if !strings.Contains(string(out), "Usage: localrpg play <game-id>") {
		t.Errorf("unexpected output: %s", string(out))
	}
}

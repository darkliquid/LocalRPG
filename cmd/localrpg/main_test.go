package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCLIVersionAndHelp(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %v, output: %s", err, string(out))
	}

	if !strings.Contains(string(out), "LocalRPG v0.1.0") {
		t.Errorf("expected version output, got: %s", string(out))
	}
}

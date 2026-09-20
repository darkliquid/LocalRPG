package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCLIRollCommand(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "roll", "3d6+4")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %v, output: %s", err, string(out))
	}

	outputStr := string(out)
	if !strings.Contains(outputStr, "Roll: 3d6+4") || !strings.Contains(outputStr, "Total:") {
		t.Errorf("unexpected output: %s", outputStr)
	}
}

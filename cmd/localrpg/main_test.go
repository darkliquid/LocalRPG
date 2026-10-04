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

	// Compare against the package variable rather than a literal: the release
	// task rewrites the version, and a hardcoded expectation would fail the
	// first build after every release.
	want := "LocalRPG v" + Version
	if !strings.Contains(string(out), want) {
		t.Errorf("expected output to contain %q, got: %s", want, string(out))
	}
}

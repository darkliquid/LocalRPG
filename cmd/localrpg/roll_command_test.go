package main

// The shape this proposal exists for: a command is a function that returns a code
// and writes to the streams it was given, so a test can read what it said without
// starting a process. roll_test.go drives `go run .` instead, which compiles the
// binary to check the same thing; these call it.

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRollCommandReportsToItsWriters(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := runRollCommand([]string{"3d6+4"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("code = %d, want 0 (stderr %q)", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Roll: 3d6+4") || !strings.Contains(got, "Total:") {
		t.Errorf("the result did not reach stdout: %q", got)
	}
	if stderr.Len() != 0 {
		t.Errorf("a clean roll wrote to stderr: %q", stderr.String())
	}
}

func TestRunRollCommandWithoutANotationIsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := runRollCommand(nil, &stdout, &stderr)

	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "Usage: localrpg roll") {
		t.Errorf("the usage did not reach stderr: %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("usage wrote to stdout: %q", stdout.String())
	}
}

func TestRunRollCommandReportsANotationItCannotEvaluate(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := runRollCommand([]string{"not-dice"}, &stdout, &stderr)

	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "Error evaluating roll") {
		t.Errorf("the failure did not reach stderr: %q", stderr.String())
	}
}

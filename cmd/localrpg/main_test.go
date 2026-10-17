package main

import (
	"bytes"
	"io"
	"os"
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

func TestPrintUsage(t *testing.T) {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	os.Stdout = w

	printUsage()

	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()

	output := buf.String()
	commands := []string{"roll", "prompt", "play", "tts", "image", "gui", "export", "content", "publisher", "registry", "debug", "config", "styles", "version"}
	for _, cmd := range commands {
		if !strings.Contains(output, cmd) {
			t.Errorf("printUsage missing command %q", cmd)
		}
	}
}

package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCLITTSCommand(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "tts", "Hello world")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %v, output: %s", err, string(out))
	}

	if !strings.Contains(string(out), "Synthesizing TTS: Hello world") {
		t.Errorf("unexpected output: %s", string(out))
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

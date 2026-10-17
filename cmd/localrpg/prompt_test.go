package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunPromptCommand(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "empty args shows usage",
			args:       nil,
			wantCode:   1,
			wantStderr: "Usage: localrpg prompt",
		},
		{
			name:       "unknown flag fails",
			args:       []string{"--unknown-flag"},
			wantCode:   1,
			wantStderr: "flag provided but not defined",
		},
		{
			name:       "default echo harness",
			args:       []string{"Greetings traveler"},
			wantCode:   0,
			wantStdout: "Greetings traveler",
		},
		{
			name:       "explicit cli harness",
			args:       []string{"--cmd", "echo", "Hello from harness"},
			wantCode:   0,
			wantStdout: "Hello from harness",
		},
		{
			name:       "failing cli harness",
			args:       []string{"--cmd", "nonexistent-cmd-xyz", "Hello"},
			wantCode:   1,
			wantStderr: "Prompt failed:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runPromptCommand(tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Errorf("code = %d, want %d (stdout: %q, stderr: %q)", code, tt.wantCode, stdout.String(), stderr.String())
			}
			if tt.wantStdout != "" && !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want substring %q", stdout.String(), tt.wantStdout)
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want substring %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

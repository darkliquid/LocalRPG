package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunPlayCommand(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStderr string
	}{
		{
			name:       "empty args shows usage",
			args:       nil,
			wantCode:   1,
			wantStderr: "Usage: localrpg play <game-id>",
		},
		{
			name:       "nonexistent game returns error",
			args:       []string{"nonexistent-game-xyz"},
			wantCode:   1,
			wantStderr: "Error loading game manifest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runPlayCommand(tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Errorf("code = %d, want %d", code, tt.wantCode)
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

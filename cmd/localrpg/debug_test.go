package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseDebugFlags(t *testing.T) {
	cmd, args, err := parseDebugArgs([]string{"test-run", "--scenario", "scenarios/smoke.yaml", "--headless=true"})
	if err != nil {
		t.Fatalf("parseDebugArgs failed: %v", err)
	}
	if cmd != "test-run" {
		t.Errorf("Expected command 'test-run', got %s", cmd)
	}
	if args.Scenario != "scenarios/smoke.yaml" || !args.Headless {
		t.Errorf("Unexpected args parsed: %+v", args)
	}
}

func TestRunDebugCommandTable(t *testing.T) {
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
			wantStderr: "Usage: localrpg debug",
		},
		{
			name:       "unknown flag fails",
			args:       []string{"test-run", "--unknown-flag"},
			wantCode:   1,
			wantStderr: "flag provided but not defined",
		},
		{
			name:       "test-run missing scenario fails",
			args:       []string{"test-run", "--scenario", "nonexistent-smoke.yaml"},
			wantCode:   1,
			wantStderr: "Open scenario:",
		},
		{
			name:       "test-system missing system fails",
			args:       []string{"test-system", "--system", "nonexistent-sys-xyz"},
			wantCode:   1,
			wantStderr: "test-system: load system:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runDebugCommand(tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Errorf("code = %d, want %d (stderr: %q)", code, tt.wantCode, stderr.String())
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want substring %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

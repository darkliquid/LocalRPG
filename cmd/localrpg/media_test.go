package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunImageCommand(t *testing.T) {
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
			wantStderr: "Usage: localrpg image <prompt>",
		},
		{
			name:       "valid prompt",
			args:       []string{"A tavern in the mist"},
			wantCode:   0,
			wantStdout: "Generating image: A tavern in the mist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runImageCommand(tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Errorf("code = %d, want %d", code, tt.wantCode)
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

func TestRunTTSCommand(t *testing.T) {
	configDir := t.TempDir()
	configYAML := "paths:\n  cache: " + filepath.Join(configDir, "cache") +
		"\nmedia:\n  tts:\n    type: builtin\n    default_voice: narrator\n"
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALRPG_CONFIG_DIR", configDir)

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStderr string
		checkFile  bool
	}{
		{
			name:       "empty args shows usage",
			args:       nil,
			wantCode:   1,
			wantStderr: "Usage: localrpg tts",
		},
		{
			name:       "unknown flag fails",
			args:       []string{"--unknown-flag"},
			wantCode:   1,
			wantStderr: "flag provided but not defined",
		},
		{
			name:      "synthesizes text",
			args:      []string{"--voice", "narrator", "--pitch", "1.0", "--rate", "1.0", "hello there"},
			wantCode:  0,
			checkFile: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runTTSCommand(tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Errorf("code = %d, want %d (stderr: %q)", code, tt.wantCode, stderr.String())
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want substring %q", stderr.String(), tt.wantStderr)
			}
			if tt.checkFile {
				path := strings.TrimSpace(stdout.String())
				if _, err := os.Stat(path); err != nil {
					t.Errorf("expected synthesized file %q to exist: %v", path, err)
				}
			}
		})
	}
}

func TestRunTTSBatchCommand(t *testing.T) {
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
			wantStderr: "Usage: localrpg tts batch",
		},
		{
			name:       "unknown flag fails",
			args:       []string{"--bad-flag"},
			wantCode:   1,
			wantStderr: "flag provided but not defined",
		},
		{
			name:       "provider without batch API returns error",
			args:       []string{"nonexistent-game"},
			wantCode:   1,
			wantStderr: "has no batch API",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runTTSBatchCommand(tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Errorf("code = %d, want %d", code, tt.wantCode)
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want substring %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

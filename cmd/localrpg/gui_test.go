// cmd/localrpg/gui_test.go
package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestRunGUICommand(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStderr string
	}{
		{
			name:     "help flag returns zero",
			args:     []string{"--help"},
			wantCode: 0,
		},
		{
			name:       "unknown flag fails",
			args:       []string{"--unknown-flag"},
			wantCode:   1,
			wantStderr: "Flag error:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runGUICommand(tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Errorf("code = %d, want %d", code, tt.wantCode)
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want substring %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestParseGUIConfig(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantPort   int
		wantSocket string
		wantWeb    bool
	}{
		{
			name:       "default flags",
			args:       []string{},
			wantPort:   0,
			wantSocket: "",
			wantWeb:    false,
		},
		{
			name:       "explicit socket",
			args:       []string{"--socket", "/tmp/custom.sock"},
			wantPort:   0,
			wantSocket: "/tmp/custom.sock",
			wantWeb:    false,
		},
		{
			name:       "explicit port",
			args:       []string{"--port", "9090"},
			wantPort:   9090,
			wantSocket: "",
			wantWeb:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseGUIConfig(tt.args, io.Discard)
			if err != nil {
				t.Fatalf("parseGUIConfig failed: %v", err)
			}
			if cfg.Port != tt.wantPort {
				t.Errorf("Port = %d, want %d", cfg.Port, tt.wantPort)
			}
			if cfg.SocketPath != tt.wantSocket {
				t.Errorf("SocketPath = %q, want %q", cfg.SocketPath, tt.wantSocket)
			}
			if cfg.WebMode != tt.wantWeb {
				t.Errorf("WebMode = %v, want %v", cfg.WebMode, tt.wantWeb)
			}
		})
	}
}

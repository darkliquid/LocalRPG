// cmd/localrpg/gui_test.go
package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCLIGUICommandHelp(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "gui", "--help")
	out, err := cmd.CombinedOutput()
	output := string(out)
	if !strings.Contains(output, "Launch desktop GUI or browser app") && !strings.Contains(output, "Usage of gui") {
		t.Errorf("unexpected output: %s, err: %v", output, err)
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
			cfg, err := parseGUIConfig(tt.args)
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

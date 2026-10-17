package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunStylesCommandTable(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("LOCALRPG_CONFIG_DIR", cfgDir)

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
			wantStderr: "Usage: localrpg styles",
		},
		{
			name:       "help flag shows usage",
			args:       []string{"--help"},
			wantCode:   0,
			wantStdout: "Usage: localrpg styles",
		},
		{
			name:       "unknown subcommand",
			args:       []string{"unknown"},
			wantCode:   1,
			wantStderr: "Unknown styles command",
		},
		{
			name:       "list with no packs",
			args:       []string{"list"},
			wantCode:   0,
			wantStdout: "(none; the built-in look is in force)",
		},
		{
			name:       "validate missing path",
			args:       []string{"validate"},
			wantCode:   1,
			wantStderr: "Usage: localrpg styles",
		},
		{
			name:       "validate nonexistent file",
			args:       []string{"validate", "nonexistent.yaml"},
			wantCode:   1,
			wantStderr: "is not usable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runStylesCommand(tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Errorf("code = %d, want %d (stderr: %q)", code, tt.wantCode, stderr.String())
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

func TestRunStylesValidateAndListWithPack(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("LOCALRPG_CONFIG_DIR", cfgDir)

	stylesDir := filepath.Join(cfgDir, "styles")
	if err := os.MkdirAll(stylesDir, 0755); err != nil {
		t.Fatal(err)
	}

	validPack := "id: dark-fantasy\nlabel: Dark Fantasy\npalette:\n  background: \"#000000\"\n"
	packPath := filepath.Join(stylesDir, "dark-fantasy.yaml")
	if err := os.WriteFile(packPath, []byte(validPack), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer

	// 1. validate valid pack
	code := runStylesCommand([]string{"validate", packPath}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "is valid") {
		t.Errorf("validate: code = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}

	// 2. list showing the pack
	stdout.Reset()
	stderr.Reset()
	code = runStylesCommand([]string{"list"}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "dark-fantasy") {
		t.Errorf("list: code = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
}

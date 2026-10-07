package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	_ "github.com/darkliquid/localrpg/pkg/provider/all"
)

func TestOfflineConfigCommands(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create a config with a cloud provider
	cfgMgr := config.NewConfigManager()
	cfg := config.DefaultConfig()
	cfg.Agents.Roles["gm"] = config.AgentRoleConfig{Type: "builtin", BuiltinName: "gemini"}
	if err := cfgMgr.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// 1. check-offline should report the issue
	var stdout, stderr bytes.Buffer
	code := runConfigCommand([]string{"check-offline"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit code when offline check fails, got 0. Out: %s, Err: %s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "gemini") {
		t.Fatalf("expected check-offline output to mention gemini, got: %s", stdout.String())
	}

	// 2. offline-preset should apply the preset
	stdout.Reset()
	stderr.Reset()
	code = runConfigCommand([]string{"offline-preset", "--tts", "native-os"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected 0 exit code, got %d. Err: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "narrative-oracle") {
		t.Fatalf("expected output to mention narrative-oracle, got: %s", stdout.String())
	}

	// 3. check-offline should now pass
	stdout.Reset()
	stderr.Reset()
	code = runConfigCommand([]string{"check-offline"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected 0 exit code when config is offline, got %d. Out: %s, Err: %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "All configured providers are offline") {
		t.Fatalf("expected success message, got: %s", stdout.String())
	}
}

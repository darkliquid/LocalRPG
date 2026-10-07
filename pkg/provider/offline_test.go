package provider_test

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/provider"
	_ "github.com/darkliquid/localrpg/pkg/provider/all"
)

func TestVerifyOfflineClean(t *testing.T) {
	cfg := config.DefaultConfig()
	config.ApplyOfflinePreset(cfg, "native-os")
	if r := config.VerifyOffline(cfg); !r.Offline {
		t.Fatalf("report = %+v", r)
	}
}

func TestVerifyOfflineFlagsCloud(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles["gm"] = config.AgentRoleConfig{Type: "builtin", BuiltinName: "gemini"}
	r := config.VerifyOffline(cfg)
	if r.Offline || len(r.Issues) == 0 {
		t.Fatalf("report = %+v", r)
	}
}

func TestVerifyOfflineFlagsLocalServer(t *testing.T) {
	cfg := config.DefaultConfig()
	config.ApplyOfflinePreset(cfg, "native-os")
	cfg.Media.Image = config.ImageConfig{Type: "http", Endpoint: "http://localhost:8188"}
	r := config.VerifyOffline(cfg)
	if r.Offline || len(r.Issues) == 0 {
		t.Fatalf("report = %+v", r)
	}
	found := false
	for _, issue := range r.Issues {
		if issue.Role == "image" && issue.Reason == "local server, not offline" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected issue with 'local server, not offline', got: %+v", r.Issues)
	}
}

func TestInspectOffline(t *testing.T) {
	if _, ok := provider.InspectOffline("gm", provider.KeyLLMNarrativeOracle); !ok {
		t.Fatal("narrative-oracle should be offline")
	}
	if _, ok := provider.InspectOffline("tts", provider.KeyTTSNativeOS); !ok {
		t.Fatal("native-os should be offline")
	}
	if issue, ok := provider.InspectOffline("gm", provider.KeyLLMGemini); ok || issue.Tier != provider.TierCloud {
		t.Fatalf("gemini should not be offline: issue=%+v, ok=%v", issue, ok)
	}
}

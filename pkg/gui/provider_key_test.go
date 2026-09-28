package gui

import (
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func TestLimitKeyMatchesLedgerKeyForLLM(t *testing.T) {
	svc := NewService(t.TempDir())
	svc.configMgr.Get().Agents.Roles["gm"] = config.AgentRoleConfig{
		Type:     "http",
		Endpoint: "http://localhost:11434/v1",
	}

	ledger, ok := harness.KeyFor(harness.ProviderConfig{Type: "http", Endpoint: "http://localhost:11434/v1"})
	if !ok {
		t.Fatal("expected an LLM key")
	}
	if got := svc.providerKeyForRole("gm"); got != string(ledger) {
		t.Errorf("limit key %q != ledger key %q", got, ledger)
	}
}

func TestLimitKeyFollowsAnInheritChain(t *testing.T) {
	svc := NewService(t.TempDir())
	cfg := svc.configMgr.Get()
	cfg.Agents.Roles["gm"] = config.AgentRoleConfig{Type: "http", Endpoint: "http://localhost:11434/v1"}
	cfg.Agents.Roles["extractor"] = config.AgentRoleConfig{Type: "inherit", InheritFrom: "gm"}

	gmKey, ok := harness.KeyFor(harness.ProviderConfig{Type: "http", Endpoint: "http://localhost:11434/v1"})
	if !ok {
		t.Fatal("expected an LLM key")
	}
	if got := svc.providerKeyForRole("extractor"); got != string(gmKey) {
		t.Errorf("extractor limit key %q != gm key %q", got, gmKey)
	}
}

func TestInstanceBlocksDoNotLeak(t *testing.T) {
	svc := NewService(t.TempDir())
	cfg := svc.configMgr.Get()
	cfg.Media.TTS = config.TTSConfig{Type: "http", Endpoint: "http://host-a:8880"}
	keyA := svc.providerKeyForRole("tts")
	cfg.Media.TTS.Endpoint = "http://host-b:8880"
	keyB := svc.providerKeyForRole("tts")

	if keyA == keyB {
		t.Fatalf("both endpoints resolved to %q", keyA)
	}
	if provider.Key(keyA).Parent() != provider.KeyTTSHTTP {
		t.Fatalf("key %q does not parent to the adapter", keyA)
	}

	svc.limits.Block(keyA, "tts", time.Now().Add(time.Minute))
	if _, blocked := svc.limits.Blocked(keyB, "tts"); blocked {
		t.Errorf("block on %q leaked to %q", keyA, keyB)
	}
	if _, blocked := svc.limits.Blocked(keyA, "tts"); !blocked {
		t.Errorf("block on %q did not apply", keyA)
	}

	svc.limits.Block(string(provider.KeyTTSHTTP), "tts", time.Now().Add(time.Minute))
	if _, blocked := svc.limits.Blocked(keyB, "tts"); !blocked {
		t.Errorf("adapter-level block did not stop %q", keyB)
	}
}

func TestMediaRoleKeyFallsBackToTheRoleName(t *testing.T) {
	svc := NewService(t.TempDir())
	svc.configMgr.Get().Media.Image = config.ImageConfig{Type: "disabled"}
	if got := svc.providerKeyForRole("image"); got != "image" {
		t.Errorf("disabled image key = %q, want the role name", got)
	}
}

func TestEmbeddingUsageIsRecorded(t *testing.T) {
	svc := NewService(t.TempDir())
	svc.RecordEmbeddingUsage("embedding:gemini@default", "text-embedding-004", 12, 1)

	ledger, err := svc.usageLedger()
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ledger.UsageByGame(UsageScopeGlobal)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, row := range rows {
		if row.Provider == "embedding:gemini@default" && row.Role == "embedding" {
			found = true
			if row.InputTokens != 12 {
				t.Errorf("input tokens = %d, want 12", row.InputTokens)
			}
		}
	}
	if !found {
		t.Fatalf("no embedding row in %+v", rows)
	}
}

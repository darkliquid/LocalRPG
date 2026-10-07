package gui

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/models"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func TestMediaInspectListsEveryEntry(t *testing.T) {
	_, svc := setupTestGame(t)
	svc.configMgr.Get().Media.TTSProviders = map[string]config.TTSConfig{
		"npc": {Type: "builtin", BuiltinName: "echo"},
	}

	resp, err := svc.InspectMedia(context.Background(), MediaInspectRequestDTO{Family: "tts"})
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, entry := range resp.Entries {
		names[entry.Name] = true
	}
	if !names["default"] || !names["npc"] {
		t.Fatalf("entries = %v, want default and npc", names)
	}
}

func TestMediaInspectReportsTierAndKey(t *testing.T) {
	_, svc := setupTestGame(t)
	svc.configMgr.Get().Media.TTS = config.TTSConfig{Type: "builtin", BuiltinName: "native-os"}
	svc.configMgr.Get().Media.TTSProviders = map[string]config.TTSConfig{
		"npc": {Type: "builtin", BuiltinName: "native-os"},
	}

	resp, err := svc.InspectMedia(context.Background(), MediaInspectRequestDTO{Family: "tts"})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range resp.Entries {
		if entry.ProviderKey == "" {
			t.Errorf("entry %q has no provider key", entry.Name)
		}
		if entry.Tier == "" {
			t.Errorf("entry %q has no tier", entry.Name)
		}
	}
}

func TestMediaInspectUnknownFamilyIsEmpty(t *testing.T) {
	_, svc := setupTestGame(t)
	resp, err := svc.InspectMedia(context.Background(), MediaInspectRequestDTO{Family: "bogus"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Entries) != 0 {
		t.Fatalf("entries = %+v, want none", resp.Entries)
	}
}

func TestMediaInspectListsEmbeddingEntries(t *testing.T) {
	_, svc := setupTestGame(t)
	cfg := svc.configMgr.Get()
	cfg.Embeddings.Enabled = true
	cfg.Embeddings.Provider = "local"
	cfg.Embeddings.Providers = map[string]config.EmbeddingProviderConfig{
		"local": {Type: "onnx"},
		"oa":    {Type: "http", Endpoint: "https://api.openai.com/v1"},
	}

	resp, err := svc.InspectMedia(context.Background(), MediaInspectRequestDTO{Family: "embedding"})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]MediaInspectEntryDTO{}
	for _, entry := range resp.Entries {
		byName[entry.Name] = entry
	}
	if len(resp.Entries) != 2 {
		t.Fatalf("entries = %+v, want 2", resp.Entries)
	}

	local := byName["local"]
	if local.ProviderKey != "embedding:onnx@default" {
		t.Errorf("local provider key = %q, want embedding:onnx@default", local.ProviderKey)
	}
	if local.Tier != string(provider.TierOfflineNeural) {
		t.Errorf("local tier = %q, want %q", local.Tier, provider.TierOfflineNeural)
	}
	if local.ModelID != models.EmbeddingEncoderModelID {
		t.Errorf("local model id = %q, want %q", local.ModelID, models.EmbeddingEncoderModelID)
	}

	oa := byName["oa"]
	if oa.ProviderKey != "embedding:openai@api.openai.com" {
		t.Errorf("oa provider key = %q, want embedding:openai@api.openai.com", oa.ProviderKey)
	}
	if oa.Tier != string(provider.TierCloud) {
		t.Errorf("oa tier = %q, want %q", oa.Tier, provider.TierCloud)
	}
}

package gui

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
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

package gui

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestSearchTTSVoicesRejectsNonGemini(t *testing.T) {
	svc := NewService(t.TempDir())

	res, err := svc.SearchTTSVoices(context.Background(), VoiceSearchRequestDTO{
		Config: config.TTSConfig{Type: "builtin", BuiltinName: "native-os"},
	})
	if err != nil {
		t.Fatalf("SearchTTSVoices: %v", err)
	}
	if res.Error == "" {
		t.Fatal("expected an explanatory error for a non-Gemini provider")
	}
	if len(res.Voices) != 0 {
		t.Fatalf("expected no voices, got %d", len(res.Voices))
	}
}

func TestListModelsWithoutKeyReportsError(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	svc := NewService(t.TempDir())

	res, err := svc.ListModels(context.Background(), ModelCatalogueRequestDTO{})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if res.Error == "" {
		t.Fatal("expected an error when no Gemini key is available")
	}
}

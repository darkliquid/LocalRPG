package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/embeddings"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func newTestExecutor(t *testing.T) *Executor {
	t.Helper()
	store, err := storage.NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	warden := &entity.Entity{
		ID: "warden", Name: "The Warden", Type: "character", Body: "A grim warden of the eastern gate.",
		Location: "eastern-gate", Wikilinks: []string{"eastern-gate"},
	}
	gate := &entity.Entity{ID: "eastern-gate", Name: "Eastern Gate", Type: "location", Body: "Iron-bound and old."}
	for _, ent := range []*entity.Entity{warden, gate} {
		if err := store.SaveEntity(ent); err != nil {
			t.Fatalf("SaveEntity(%s): %v", ent.ID, err)
		}
	}
	return NewExecutor(store, 4000)
}

func call(name, arguments string) harness.ToolCall {
	return harness.ToolCall{ID: "1", Name: name, Arguments: arguments}
}

func TestSearchEntitiesTool(t *testing.T) {
	executor := newTestExecutor(t)

	result, ok := executor.Execute(context.Background(), call("search_entities", `{"query":"warden"}`))
	if !ok {
		t.Fatalf("Execute reported failure: %s", result)
	}
	if !strings.Contains(result, "warden") || !strings.Contains(result, "The Warden") {
		t.Errorf("result = %s", result)
	}

	result, ok = executor.Execute(context.Background(), call("search_entities", `{"query":"warden","type":"location"}`))
	if !ok || !strings.Contains(result, "No entities") {
		t.Errorf("filtered result = %q, ok = %v", result, ok)
	}
}

func TestGetEntityTool(t *testing.T) {
	executor := newTestExecutor(t)

	result, ok := executor.Execute(context.Background(), call("get_entity", `{"id_or_name":"The Warden"}`))
	if !ok || !strings.Contains(result, "A grim warden") {
		t.Errorf("result = %q, ok = %v", result, ok)
	}

	result, ok = executor.Execute(context.Background(), call("get_entity", `{"id_or_name":"Nobody"}`))
	if ok || !strings.Contains(result, "error:") {
		t.Errorf("a missing entity must be a readable error, got %q, ok = %v", result, ok)
	}
}

func TestGraphNeighboursTool(t *testing.T) {
	executor := newTestExecutor(t)

	result, ok := executor.Execute(context.Background(), call("graph_neighbours", `{"id":"warden"}`))
	if !ok || !strings.Contains(result, "eastern-gate") {
		t.Errorf("result = %q, ok = %v", result, ok)
	}
}

func TestSearchTimelineTool(t *testing.T) {
	executor := newTestExecutor(t)
	if err := executor.store.SaveTurn(storage.TurnRecord{Number: 1, Mode: "Do", Input: "I run", Narration: "The guttered lantern flared."}); err != nil {
		t.Fatalf("SaveTurn: %v", err)
	}

	result, ok := executor.Execute(context.Background(), call("search_timeline", `{"query":"guttering"}`))
	if !ok || !strings.Contains(result, "turn 1") {
		t.Errorf("result = %q, ok = %v", result, ok)
	}
}

func TestToolErrorsAreReadableResults(t *testing.T) {
	executor := newTestExecutor(t)

	result, ok := executor.Execute(context.Background(), call("teleport", `{}`))
	if ok || !strings.Contains(result, "search_entities") {
		t.Errorf("unknown tool result = %q, ok = %v", result, ok)
	}

	result, ok = executor.Execute(context.Background(), call("search_entities", `{not json`))
	if ok || !strings.Contains(result, "error:") {
		t.Errorf("malformed arguments result = %q, ok = %v", result, ok)
	}

	result, ok = executor.Execute(context.Background(), call("search_entities", `{}`))
	if ok || !strings.Contains(result, "query") {
		t.Errorf("missing argument result = %q, ok = %v", result, ok)
	}
}

func TestToolResultsAreCapped(t *testing.T) {
	store, err := storage.NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	long := strings.Repeat("the ancient bridge ", 200)
	if err := store.SaveEntity(&entity.Entity{ID: "long", Name: "Long Note", Type: "lore", Body: long}); err != nil {
		t.Fatalf("SaveEntity: %v", err)
	}

	executor := NewExecutor(store, 200)
	result, ok := executor.Execute(context.Background(), call("get_entity", `{"id_or_name":"long"}`))
	if !ok {
		t.Fatalf("Execute reported failure: %s", result)
	}
	if len([]rune(result)) > 280 {
		t.Errorf("result is %d runes, want it capped near 200 plus the marker", len([]rune(result)))
	}
	if !strings.Contains(result, "truncated") {
		t.Errorf("a cap that bites must say so: %s", result)
	}
}

func TestHybridSearchEntitiesTool(t *testing.T) {
	store, err := storage.NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	lair := &entity.Entity{
		ID:   "dragon-lair",
		Name: "Smoldering Hollow",
		Type: "location",
		Body: "A fiery mountain cavern filled with heaps of gold and bones.",
	}
	if err := store.SaveEntity(lair); err != nil {
		t.Fatalf("SaveEntity: %v", err)
	}

	provider := embeddings.NewBuiltinHashProjectionProvider(384)
	vecs, err := provider.Embed(context.Background(), []string{lair.Name + " " + lair.Body})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if err := store.SaveEmbedding("entity", lair.ID, "hash", provider.ID(), 0, vecs[0]); err != nil {
		t.Fatalf("SaveEmbedding: %v", err)
	}

	executor := NewExecutor(store, 4000)
	executor.SetEmbeddingsProvider(provider)

	// Query with terms matching concept/n-grams
	result, ok := executor.Execute(context.Background(), call("search_entities", `{"query":"fiery gold cavern"}`))
	if !ok {
		t.Fatalf("Execute reported failure: %s", result)
	}
	if !strings.Contains(result, "Smoldering Hollow") {
		t.Errorf("expected Smoldering Hollow in result, got: %s", result)
	}
}

func TestVoiceTools(t *testing.T) {
	executor := newTestExecutor(t)
	profiles := []config.VoiceProfile{
		{
			ID:          "aoede",
			Name:        "Aoede",
			Description: "Warm, narrative female voice for guides and wise allies.",
			Tags:        []string{"warm", "wise", "companion", "female"},
			Provider:    "native-os",
			VoiceID:     "voice-aoede",
		},
		{
			ID:          "fenrir",
			Name:        "Fenrir",
			Description: "Gravelly, deep warrior voice for brutes and hardened veterans.",
			Tags:        []string{"gravelly", "deep", "veteran", "male"},
			Provider:    "native-os",
			VoiceID:     "voice-fenrir",
		},
	}
	executor.SetVoiceProfiles(profiles)

	// 1. Search voice profiles
	res, ok := executor.Execute(context.Background(), call("search_voice_profiles", `{"query":"gravelly warrior"}`))
	if !ok {
		t.Fatalf("search_voice_profiles failed: %s", res)
	}
	if !strings.Contains(res, "fenrir") {
		t.Errorf("expected fenrir in results, got %s", res)
	}

	// 2. Assign voice to existing entity "warden"
	res, ok = executor.Execute(context.Background(), call("assign_voice", `{"entity":"The Warden","profile_id":"fenrir"}`))
	if !ok {
		t.Fatalf("assign_voice failed: %s", res)
	}
	if !strings.Contains(res, "assigned") {
		t.Errorf("expected assignment confirmation, got %s", res)
	}

	// Verify entity in store has updated voice
	ent, err := executor.store.GetEntity("warden")
	if err != nil || ent == nil {
		t.Fatalf("failed to get warden: %v", err)
	}
	if ent.Voice == nil || ent.Voice.VoiceID != "voice-fenrir" {
		t.Errorf("expected warden voice to be voice-fenrir, got %+v", ent.Voice)
	}

	// 3. Assign voice to unknown entity (stages it)
	res, ok = executor.Execute(context.Background(), call("assign_voice", `{"entity":"new-scout","profile_id":"aoede"}`))
	if !ok {
		t.Fatalf("assign_voice for staged entity failed: %s", res)
	}
	if !strings.Contains(res, "staged") {
		t.Errorf("expected staged confirmation, got %s", res)
	}
	staged := executor.AssignedVoices()
	if staged["new-scout"].ID != "aoede" {
		t.Errorf("expected staged voice for new-scout to be aoede, got %+v", staged["new-scout"])
	}
}


package tools

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/embeddings"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func newMemoryToolStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestSearchMemoriesTool(t *testing.T) {
	store := newMemoryToolStore(t)
	if _, err := store.SaveMemory(&entity.Memory{Turn: 2, Kind: entity.MemoryEvent, EntityRefs: []string{"kae"}, Text: "Found a silver locket.", Importance: 4, Source: entity.SourceGM}); err != nil {
		t.Fatal(err)
	}
	exec := NewExecutor(store, 4000)
	out, ok := exec.Execute(t.Context(), harness.ToolCall{Name: "search_memories", Arguments: `{"query":"silver locket"}`})
	if !ok || !strings.Contains(out, "locket") {
		t.Fatalf("search_memories = %q, ok=%v", out, ok)
	}
}

func TestGetEntityTimelineTool(t *testing.T) {
	store := newMemoryToolStore(t)
	if _, err := store.SaveMemory(&entity.Memory{Turn: 5, Kind: entity.MemoryRelationship, EntityRefs: []string{"kae"}, Text: "Distrusts the warden.", Importance: 3, Source: entity.SourceGM}); err != nil {
		t.Fatal(err)
	}
	exec := NewExecutor(store, 4000)
	out, ok := exec.Execute(t.Context(), harness.ToolCall{Name: "get_entity_timeline", Arguments: `{"entity":"kae"}`})
	if !ok || !strings.Contains(out, "Distrusts the warden") {
		t.Fatalf("get_entity_timeline = %q, ok=%v", out, ok)
	}
}

func TestHybridSearchMemoriesTool(t *testing.T) {
	store := newMemoryToolStore(t)
	memID, err := store.SaveMemory(&entity.Memory{
		Turn:       3,
		Kind:       entity.MemoryDiscovery,
		EntityRefs: []string{"kae"},
		Text:       "Discovered ancient parchment detailing dragon lore and hidden mountain gold.",
		Importance: 4,
		Source:     entity.SourceGM,
	})
	if err != nil {
		t.Fatal(err)
	}

	provider := embeddings.NewBuiltinHashProjectionProvider(384)
	vecs, err := provider.Embed(t.Context(), []string{"Discovered ancient parchment detailing dragon lore and hidden mountain gold."})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveEmbedding("memory", strconv.FormatInt(memID, 10), "hash", provider.ID(), 0, vecs[0]); err != nil {
		t.Fatal(err)
	}

	exec := NewExecutor(store, 4000)
	exec.SetEmbeddingsProvider(provider)

	out, ok := exec.Execute(t.Context(), harness.ToolCall{
		Name:      "search_memories",
		Arguments: `{"query":"ancient mountain dragon gold"}`,
	})
	if !ok || !strings.Contains(out, "parchment") {
		t.Fatalf("search_memories = %q, ok=%v", out, ok)
	}
}

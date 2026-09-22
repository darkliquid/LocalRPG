package harness

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestContextAssembler(t *testing.T) {
	tempDir := t.TempDir()
	store, err := storage.NewStore(filepath.Join(tempDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// 1. Scene location
	tavern := &entity.Entity{
		ID:        "alden-tavern",
		Name:      "Alden Tavern",
		Type:      "location",
		Body:      "A warm tavern smelling of ale.",
		Wikilinks: []string{"arc-siege", "lady-evelyn"},
		Hash:      "hash-tavern",
	}
	store.SaveEntity(tavern)

	// 2. Active NPC
	npc := &entity.Entity{
		ID:       "lady-evelyn",
		Name:     "Lady Evelyn",
		Type:     "character",
		Location: "[[alden-tavern]]",
		Body:     "Guarded former lieutenant.",
		Hash:     "hash-evelyn",
	}
	store.SaveEntity(npc)

	// 3. Living World Arc
	arc := &entity.Entity{
		ID:   "arc-siege",
		Name: "The Iron Siege",
		Type: "arc",
		Body: "Food supplies are depleted in the lower quarter.",
		Hash: "hash-arc",
	}
	store.SaveEntity(arc)

	assembler := NewContextAssembler(store)
	ctxPrompt, err := assembler.AssembleContext("alden-tavern", "player", "I speak with Evelyn")
	if err != nil {
		t.Fatalf("AssembleContext failed: %v", err)
	}

	// Verify all layers are assembled
	if !strings.Contains(ctxPrompt, "Alden Tavern") {
		t.Errorf("missing location in context")
	}
	if !strings.Contains(ctxPrompt, "Lady Evelyn") {
		t.Errorf("missing NPC in context")
	}
	if !strings.Contains(ctxPrompt, "The Iron Siege") {
		t.Errorf("missing living world arc in context")
	}
}

func TestContextAssemblerWithRulesAndLore(t *testing.T) {
	tempDir := t.TempDir()
	store, err := storage.NewStore(filepath.Join(tempDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	assembler := NewContextAssembler(store)
	rulesPrompt := "Resolution: 10+ Success, 7-9 Mixed, 6- Failure."
	lorePrompt := "Atmosphere: Cold mist and distant bells."

	prompt, err := assembler.AssembleContextWithRules("loc1", "p1", "I inspect the door", rulesPrompt, lorePrompt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(prompt, "## SYSTEM RULES & RESOLUTION MECHANICS") || !strings.Contains(prompt, rulesPrompt) {
		t.Errorf("expected rules prompt in context, got: %s", prompt)
	}
	if !strings.Contains(prompt, "## WORLD LORE & ATMOSPHERE") || !strings.Contains(prompt, lorePrompt) {
		t.Errorf("expected lore prompt in context, got: %s", prompt)
	}
}

func TestContextAssembler_WithVoiceProfiles(t *testing.T) {
	tempDir := t.TempDir()
	store, err := storage.NewStore(filepath.Join(tempDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	assembler := NewContextAssembler(store)
	profiles := []config.VoiceProfile{
		{ID: "elder_sage", Description: "Ancient wizards and wise hermits"},
		{ID: "young_scout", Description: "Agile rangers and scouts"},
	}

	prompt, err := assembler.AssembleContextWithProfiles("loc1", "p1", "I greet the elders", "", "", profiles, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(prompt, "AVAILABLE NPC VOICE PROFILES") || !strings.Contains(prompt, "elder_sage") {
		t.Errorf("expected voice profiles section in system prompt, got: %s", prompt)
	}
}

func TestAssembleContextAlwaysAsksForAttributableSpeech(t *testing.T) {
	store := newTestEntityStore(t)
	assembler := NewContextAssembler(store)

	// No rules prompt, no lore prompt: the instruction must not depend on a system
	// or world shipping anything.
	prompt, err := assembler.AssembleContextWithProfiles("", "", "I listen", "", "", nil, "")
	if err != nil {
		t.Fatalf("AssembleContextWithProfiles failed: %v", err)
	}

	if !strings.Contains(prompt, "## SPEECH FORMATTING") {
		t.Errorf("expected the speech formatting section, got %q", prompt)
	}
	if !strings.Contains(prompt, `Name: "the words spoken"`) {
		t.Errorf("expected the instruction to show the shape it wants, got %q", prompt)
	}
	if !strings.Contains(prompt, "leave the words in the narration") {
		t.Errorf("expected guidance for the case the model cannot name a speaker, got %q", prompt)
	}
}

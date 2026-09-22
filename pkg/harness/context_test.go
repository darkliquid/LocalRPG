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

	result, err := assembler.AssembleContextWithProfiles("loc1", "p1", "I greet the elders", "", "", profiles, nil)
	prompt := result.Prompt
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
	result, err := assembler.AssembleContextWithProfiles("", "", "I listen", "", "", nil, nil)
	prompt := result.Prompt
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

func TestRecentTurnsAreRecalledWithinTheWindow(t *testing.T) {
	assembler := NewContextAssembler(newTestEntityStore(t))

	recent := []RecentTurn{
		{Number: 1, Mode: "Opening", Narration: "Rain hammers the market."},
		{Number: 2, Mode: "Say", Input: "Late for what?", Narration: "The bell tolls once."},
	}

	result, err := assembler.AssembleContextWithProfiles("", "", "I listen", "", "", nil, recent)
	if err != nil {
		t.Fatalf("AssembleContextWithProfiles failed: %v", err)
	}
	if !strings.Contains(result.Prompt, "Late for what?") || !strings.Contains(result.Prompt, "Rain hammers the market.") {
		t.Errorf("recall lost a prior turn: %q", result.Prompt)
	}

	assembler.SetLimits(ContextLimits{RecentTurns: 1})
	result, err = assembler.AssembleContextWithProfiles("", "", "I listen", "", "", nil, recent)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Prompt, "Rain hammers the market.") {
		t.Errorf("window of 1 should drop the older turn, got %q", result.Prompt)
	}
	if !strings.Contains(result.Prompt, "Late for what?") {
		t.Errorf("window of 1 should keep the newest turn, got %q", result.Prompt)
	}
}

func TestContextBudgetTrimDropsRecallBeforeRules(t *testing.T) {
	assembler := NewContextAssembler(newTestEntityStore(t))

	longNarration := strings.Repeat("the mist rolls in over the drowned cathedral ", 200)
	recent := []RecentTurn{
		{Number: 1, Mode: "Do", Narration: longNarration},
		{Number: 2, Mode: "Do", Narration: longNarration},
		{Number: 3, Mode: "Do", Narration: longNarration},
	}

	unbounded, err := assembler.AssembleContextWithProfiles("", "", "I listen", "RULES", "LORE", nil, recent)
	if err != nil {
		t.Fatal(err)
	}

	assembler.SetLimits(ContextLimits{TokenBudget: 400})
	trimmed, err := assembler.AssembleContextWithProfiles("", "", "I listen", "RULES", "LORE", nil, recent)
	if err != nil {
		t.Fatal(err)
	}

	if trimmed.EstimatedTokens >= unbounded.EstimatedTokens {
		t.Errorf("expected trimming to shrink the prompt: %d vs %d", trimmed.EstimatedTokens, unbounded.EstimatedTokens)
	}
	if len(trimmed.Trimmed) == 0 {
		t.Errorf("expected the trim to be reported")
	}

	// What must survive: the rules, the lore, the scene, and the actual request.
	for _, required := range []string{"SYSTEM RULES", "WORLD LORE", "IMMEDIATE SCENE", "PLAYER ACTION", "I listen"} {
		if !strings.Contains(trimmed.Prompt, required) {
			t.Errorf("trimming dropped %q, which is never expendable", required)
		}
	}
}

func TestTrimmingIsDrivenByTheBudgetNotTheCharCap(t *testing.T) {
	// Trailing space trimmed, because recall trims each turn's text.
	longNarration := strings.TrimSpace(strings.Repeat("the mist rolls in ", 500))
	recent := []RecentTurn{{Number: 1, Mode: "Do", Narration: longNarration}}

	// An unbounded budget with a generous cap keeps everything, and reports no
	// trimming: the two limits are independent, and only the budget trims.
	assembler := NewContextAssembler(newTestEntityStore(t))
	assembler.SetLimits(ContextLimits{TokenBudget: 0, RecentTurnChars: 1 << 20})
	result, err := assembler.AssembleContextWithProfiles("", "", "I listen", "", "", nil, recent)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Trimmed) != 0 {
		t.Errorf("expected an unbounded budget to trim nothing, got %v", result.Trimmed)
	}
	if !strings.Contains(result.Prompt, longNarration) {
		t.Errorf("expected the whole narration to survive a generous cap")
	}

	// The default cap shortens one turn without being a budget decision.
	capped := NewContextAssembler(newTestEntityStore(t))
	cappedResult, err := capped.AssembleContextWithProfiles("", "", "I listen", "", "", nil, recent)
	if err != nil {
		t.Fatal(err)
	}
	if len(cappedResult.Trimmed) != 0 {
		t.Errorf("a configured cap is not trimming, got %v", cappedResult.Trimmed)
	}
	if strings.Contains(cappedResult.Prompt, longNarration) {
		t.Errorf("expected the default char cap to shorten a very long turn")
	}
}

func TestTruncateRunesMarksWhatItCut(t *testing.T) {
	if got := TruncateRunes("abcdef", 3); got != "abc..." {
		t.Errorf("TruncateRunes = %q, want abc...", got)
	}
	if got := TruncateRunes("abc", 3); got != "abc" {
		t.Errorf("TruncateRunes left a short string alone? got %q", got)
	}
}

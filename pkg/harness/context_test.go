package harness

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/state"
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
	assembled, err := assembler.Assemble(ContextRequest{
		LocationID: "alden-tavern",
		PlayerID:   "player",
		Action:     "I speak with Evelyn",
	})
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}
	ctxPrompt := assembled.Prompt

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

	result, err := assembler.Assemble(ContextRequest{
		LocationID:  "loc1",
		PlayerID:    "p1",
		Action:      "I inspect the door",
		RulesPrompt: rulesPrompt,
		LorePrompt:  lorePrompt,
	})
	prompt := result.Prompt
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

	result, err := assembler.Assemble(ContextRequest{
		LocationID: "loc1",
		PlayerID:   "p1",
		Action:     "I greet the elders",
		Profiles:   profiles,
	})
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
	result, err := assembler.Assemble(ContextRequest{Action: "I listen"})
	prompt := result.Prompt
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
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

	result, err := assembler.Assemble(ContextRequest{Action: "I listen", Recent: recent})
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}
	if !strings.Contains(result.Prompt, "Late for what?") || !strings.Contains(result.Prompt, "Rain hammers the market.") {
		t.Errorf("recall lost a prior turn: %q", result.Prompt)
	}

	assembler.SetLimits(ContextLimits{RecentTurns: 1})
	result, err = assembler.Assemble(ContextRequest{Action: "I listen", Recent: recent})
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

	unbounded, err := assembler.Assemble(ContextRequest{Action: "I listen", RulesPrompt: "RULES", LorePrompt: "LORE", Recent: recent})
	if err != nil {
		t.Fatal(err)
	}

	assembler.SetLimits(ContextLimits{TokenBudget: 400})
	trimmed, err := assembler.Assemble(ContextRequest{Action: "I listen", RulesPrompt: "RULES", LorePrompt: "LORE", Recent: recent})
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
	result, err := assembler.Assemble(ContextRequest{Action: "I listen", Recent: recent})
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
	cappedResult, err := capped.Assemble(ContextRequest{Action: "I listen", Recent: recent})
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

func TestAssembleReportsEverySectionAndKeepsTheActionLast(t *testing.T) {
	assembler := NewContextAssembler(newTestEntityStore(t))

	result, err := assembler.Assemble(ContextRequest{
		Action:      "I listen at the door",
		RulesPrompt: "RULES",
		LorePrompt:  "LORE",
		TurnNumber:  4,
	})
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	// The action is the request the model answers; everything above is context.
	if !strings.HasSuffix(result.Prompt, "I listen at the door\n") {
		t.Errorf("the action must be last in the prompt")
	}
	if !strings.Contains(result.Prompt, "## PLAYER ACTION") {
		t.Errorf("expected a player action section")
	}

	names := make([]string, 0, len(result.Sections))
	for _, section := range result.Sections {
		names = append(names, section.Name)
		if !section.Included && section.Tokens != 0 {
			t.Errorf("section %s is excluded but reports tokens", section.Name)
		}
	}
	for _, wanted := range []string{"rules", "lore", "instructions", "canon", "recent", "action"} {
		if !containsString(names, wanted) {
			t.Errorf("expected a %q section, got %v", wanted, names)
		}
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// saveEntity indexes a note the way the syncer does, without a file on disk.
func saveEntity(t *testing.T, store *storage.Store, ent *entity.Entity) {
	t.Helper()
	if err := store.SaveEntity(ent); err != nil {
		t.Fatalf("save entity %s: %v", ent.ID, err)
	}
}

func TestRenderStateIsDeterministic(t *testing.T) {
	raw := map[string]interface{}{"brazier_lit": true, "danger_level": float64(2), "name": "The Ashen Bastion"}

	first := RenderState(raw)
	second := RenderState(raw)
	if first != second {
		t.Errorf("RenderState must be stable, got %q then %q", first, second)
	}
	if first != "brazier_lit=true, danger_level=2, name=The Ashen Bastion" {
		t.Errorf("unexpected rendering: %q", first)
	}
	if RenderState(nil) != "" {
		t.Errorf("expected an empty rendering for no state")
	}
}

func TestCanonRendersStateAndKeepsItWhenTiny(t *testing.T) {
	store := newTestEntityStore(t)
	saveEntity(t, store, &entity.Entity{
		ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Body: "Salt air.",
		State: state.NewState(map[string]interface{}{"danger_level": float64(2), "brazier_lit": true}),
		Hash:  "h1",
	})

	assembler := NewContextAssembler(store)
	assembler.SetLimits(ContextLimits{TokenBudget: 5})
	result, err := assembler.Assemble(ContextRequest{LocationID: "aldon-harbour", Action: "I look around", TurnNumber: 2})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result.Prompt, "danger_level=2") || !strings.Contains(result.Prompt, "brazier_lit=true") {
		t.Errorf("canon state must reach the prompt:\n%s", result.Prompt)
	}
	for _, section := range result.Sections {
		if section.Name == "canon" && !section.Included {
			t.Errorf("canon was trimmed, which must never happen")
		}
	}
}

func TestEstablishedNamesListsWhatIsInPlay(t *testing.T) {
	store := newTestEntityStore(t)
	saveEntity(t, store, &entity.Entity{ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Hash: "h1"})
	saveEntity(t, store, &entity.Entity{ID: "guard-kael", Name: "Guard Kael", Type: "character", Hash: "h2"})
	saveEntity(t, store, &entity.Entity{ID: "distant-city", Name: "Distant City", Type: "location", Hash: "h3"})
	if err := store.SaveTurn(storage.TurnRecord{
		Number: 1, Timestamp: time.Now(), Mode: "Do", Input: "x", Narration: "y",
		Entities: []storage.TurnEntityRef{{EntityID: "guard-kael", Mention: "wikilink"}},
	}); err != nil {
		t.Fatal(err)
	}

	assembler := NewContextAssembler(store)
	result, err := assembler.Assemble(ContextRequest{
		LocationID: "aldon-harbour",
		Action:     "I wait",
		Recent:     []RecentTurn{{Number: 1, Mode: "Do", Narration: "y"}},
		TurnNumber: 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result.Prompt, "Guard Kael") {
		t.Errorf("a character mentioned in the recall window must be named:\n%s", result.Prompt)
	}
	if strings.Contains(result.Prompt, "Distant City") {
		t.Errorf("an entity that is neither present nor mentioned must not be named")
	}
}

func TestSceneRecallRemembersThisLocationOnly(t *testing.T) {
	store := newTestEntityStore(t)
	saveEntity(t, store, &entity.Entity{ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Hash: "h1"})
	saveEntity(t, store, &entity.Entity{ID: "oakhaven-tavern", Name: "Oakhaven Tavern", Type: "location", Hash: "h2"})
	for _, turn := range []storage.TurnRecord{
		{Number: 1, Timestamp: time.Now(), Mode: "Do", Narration: "The brazier guttered.", Location: "aldon-harbour"},
		{Number: 2, Timestamp: time.Now(), Mode: "Do", Narration: "Ale and noise.", Location: "oakhaven-tavern"},
		{Number: 3, Timestamp: time.Now(), Mode: "Do", Narration: "Kael refused the gate.", Location: "aldon-harbour"},
	} {
		if err := store.SaveTurn(turn); err != nil {
			t.Fatal(err)
		}
	}

	assembler := NewContextAssembler(store)
	result, err := assembler.Assemble(ContextRequest{
		LocationID: "aldon-harbour",
		Action:     "I approach",
		TurnNumber: 4,
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result.Prompt, "WHAT HAPPENED HERE") {
		t.Fatalf("expected a scene recall section:\n%s", result.Prompt)
	}
	if !strings.Contains(result.Prompt, "The brazier guttered.") || !strings.Contains(result.Prompt, "Kael refused the gate.") {
		t.Errorf("expected both turns at this location:\n%s", result.Prompt)
	}
	if strings.Contains(result.Prompt, "Ale and noise.") {
		t.Errorf("a turn at another location must not be recalled here")
	}
}

func TestSceneRecallExcludesTurnsAlreadyInTheWindow(t *testing.T) {
	store := newTestEntityStore(t)
	saveEntity(t, store, &entity.Entity{ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Hash: "h1"})
	if err := store.SaveTurn(storage.TurnRecord{
		Number: 1, Timestamp: time.Now(), Mode: "Do", Narration: "The bell tolled.", Location: "aldon-harbour",
	}); err != nil {
		t.Fatal(err)
	}

	assembler := NewContextAssembler(store)
	result, err := assembler.Assemble(ContextRequest{
		LocationID: "aldon-harbour",
		Action:     "I wait",
		Recent:     []RecentTurn{{Number: 1, Mode: "Do", Narration: "The bell tolled."}},
		TurnNumber: 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Count(result.Prompt, "The bell tolled.") != 1 {
		t.Errorf("a turn in the window must not be recalled twice:\n%s", result.Prompt)
	}
}

func TestRelevantHistoryFindsTheTurnOutsideTheWindow(t *testing.T) {
	store := newTestEntityStore(t)
	saveEntity(t, store, &entity.Entity{ID: "guard-kael", Name: "Guard Kael", Type: "character", Hash: "h1"})
	saveEntity(t, store, &entity.Entity{ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Hash: "h2"})

	// The promise is made in turn 1, long before the recall window.
	for _, turn := range []storage.TurnRecord{
		{Number: 1, Timestamp: time.Now(), Mode: "Do", Narration: "Kael mentioned the oil was low.", Location: "oakhaven-tavern",
			Entities: []storage.TurnEntityRef{{EntityID: "guard-kael", Mention: "wikilink"}}},
		// Turn 2 mentions him too, which is what puts him in play, but it is already
		// in the window so it cannot be the answer.
		{Number: 2, Timestamp: time.Now(), Mode: "Do", Narration: "Kael: \"Ask me again.\"", Location: "aldon-harbour",
			Entities: []storage.TurnEntityRef{{EntityID: "guard-kael", Mention: "speech"}}},
	} {
		if err := store.SaveTurn(turn); err != nil {
			t.Fatal(err)
		}
	}

	assembler := NewContextAssembler(store)
	result, err := assembler.Assemble(ContextRequest{
		LocationID: "aldon-harbour",
		Action:     "I ask Kael about supplies",
		// Turn 2 is in the window, and it does not mention him. Turn 1 is the one
		// the window has lost, and the mention of him is the thread back to it.
		Recent:     []RecentTurn{{Number: 2, Mode: "Do", Narration: "Kael: \"Ask me again.\""}},
		TurnNumber: 3,
		PlayerID:   "player",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result.Prompt, "RELEVANT HISTORY") {
		t.Fatalf("expected a retrieval section:\n%s", result.Prompt)
	}
	if !strings.Contains(result.Prompt, "the oil was low") {
		t.Errorf("retrieval must recover the turn the window lost:\n%s", result.Prompt)
	}
}

func TestRelevantHistoryExcludesTheLocationAndThePlayer(t *testing.T) {
	store := newTestEntityStore(t)
	saveEntity(t, store, &entity.Entity{ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Hash: "h1"})

	if err := store.SaveTurn(storage.TurnRecord{
		Number: 1, Timestamp: time.Now(), Mode: "Do", Narration: "The harbour at dawn.", Location: "aldon-harbour",
		Entities: []storage.TurnEntityRef{{EntityID: "aldon-harbour", Mention: "location"}},
	}); err != nil {
		t.Fatal(err)
	}

	assembler := NewContextAssembler(store)
	result, err := assembler.Assemble(ContextRequest{
		LocationID: "aldon-harbour",
		Action:     "I listen",
		Recent:     []RecentTurn{{Number: 2, Mode: "Do", Narration: "I listen."}},
		TurnNumber: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, section := range result.Sections {
		if section.Name == "retrieval" && section.Included {
			t.Errorf("the location alone must not trigger retrieval, since scene recall covers it")
		}
	}
}

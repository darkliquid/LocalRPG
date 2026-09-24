package engine

import (
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/state"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func checkContinuity(store *storage.Store, turn *Turn, locationID, playerID string) []ContinuityFinding {
	return CheckContinuity(ContinuityInput{
		Store:      store,
		Turn:       turn,
		LocationID: locationID,
		PlayerID:   playerID,
	})
}

func continuityStore(t *testing.T) *storage.Store {
	t.Helper()

	store := newTestStore(t)
	mustSave := func(ent *entity.Entity) {
		t.Helper()
		if err := store.SaveEntity(ent); err != nil {
			t.Fatal(err)
		}
	}

	mustSave(&entity.Entity{ID: "guard-kael", Name: "Guard Kael", Type: "character", Body: "A warden."})
	mustSave(&entity.Entity{ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Body: "Salt."})
	mustSave(&entity.Entity{ID: "oakhaven-tavern", Name: "Oakhaven Tavern", Type: "location", Body: "Ale."})
	mustSave(&entity.Entity{
		ID: "the-bastion", Name: "The Ashen Bastion", Type: "location", Body: "A sanctuary.",
		State: state.NewState(map[string]interface{}{"brazier_lit": true}),
	})
	return store
}

func ruleNames(findings []ContinuityFinding) []string {
	names := make([]string, 0, len(findings))
	for _, finding := range findings {
		names = append(names, finding.Rule)
	}
	return names
}

func hasRule(findings []ContinuityFinding, rule string) bool {
	for _, finding := range findings {
		if finding.Rule == rule {
			return true
		}
	}
	return false
}

func TestContinuityFlagsANameIntroducedWithoutANote(t *testing.T) {
	store := continuityStore(t)

	turn := &Turn{Number: 1, Narration: "A figure steps forward.\nThe Pale Sister: \"You are late.\""}
	findings := checkContinuity(store, turn, "aldon-harbour", "player")

	if !hasRule(findings, "unknown-entity") {
		t.Errorf("expected an unknown entity finding, got %v", ruleNames(findings))
	}

	// A capitalised phrase that claims nothing is not evidence.
	quiet := &Turn{Number: 1, Narration: "The rain fell on the harbour and the gulls cried."}
	if hasRule(checkContinuity(store, quiet, "aldon-harbour", "player"), "unknown-entity") {
		t.Errorf("plain prose must not be flagged")
	}
}

func TestContinuityFlagsALocationThePartyIsNotAt(t *testing.T) {
	store := continuityStore(t)

	turn := &Turn{Number: 1, Narration: "She remembers Oakhaven Tavern fondly."}
	findings := checkContinuity(store, turn, "aldon-harbour", "player")

	if !hasRule(findings, "location-drift") {
		t.Errorf("expected a location drift finding, got %v", ruleNames(findings))
	}

	// Naming where they are is not drift.
	here := &Turn{Number: 1, Narration: "Aldon Harbour is quiet tonight."}
	if hasRule(checkContinuity(store, here, "aldon-harbour", "player"), "location-drift") {
		t.Errorf("naming the current location must not be flagged")
	}
}

func TestContinuityFlagsSpeechThatResolvedToNobody(t *testing.T) {
	store := continuityStore(t)

	turn := &Turn{Number: 1, Narration: "Someone whispers.\nA Nameless Voice: \"Behind you.\""}
	findings := checkContinuity(store, turn, "aldon-harbour", "player")

	if !hasRule(findings, "unresolved-speaker") {
		t.Errorf("expected an unresolved speaker finding, got %v", ruleNames(findings))
	}

	// A line that resolves is not a finding.
	known := &Turn{Number: 1, Narration: "Guard Kael: \"Behind you.\""}
	if hasRule(checkContinuity(store, known, "aldon-harbour", "player"), "unresolved-speaker") {
		t.Errorf("a resolved speaker must not be flagged")
	}
}

func TestContinuityFlagsReintroducingSomeoneKnown(t *testing.T) {
	store := continuityStore(t)

	turn := &Turn{Number: 2, Narration: "A stranger named Guard Kael waits by the water."}
	findings := checkContinuity(store, turn, "aldon-harbour", "player")

	if !hasRule(findings, "reintroduction") {
		t.Errorf("expected a reintroduction finding, got %v", ruleNames(findings))
	}
}

func TestContinuityFlagsAContradictedState(t *testing.T) {
	store := continuityStore(t)

	// The note says the brazier is lit, and the prose says it is out.
	turn := &Turn{Number: 2, Narration: "The brazier was dark and the bastion felt cold.", Location: "the-bastion"}
	findings := checkContinuity(store, turn, "the-bastion", "player")

	if !hasRule(findings, "state-contradiction") {
		t.Errorf("expected a state contradiction finding, got %v", ruleNames(findings))
	}

	// Consistent prose is silent.
	lit := &Turn{Number: 2, Narration: "The brazier burned steadily, keeping the mist back.", Location: "the-bastion"}
	if hasRule(checkContinuity(store, lit, "the-bastion", "player"), "state-contradiction") {
		t.Errorf("consistent prose must not be flagged")
	}
}

func TestContinuityFindingsSayWhatTheySaw(t *testing.T) {
	store := continuityStore(t)

	turn := &Turn{Number: 1, Narration: "She remembers Oakhaven Tavern fondly."}
	findings := checkContinuity(store, turn, "aldon-harbour", "player")

	for _, finding := range findings {
		if strings.TrimSpace(finding.Note) == "" {
			t.Errorf("finding %q carries no explanation", finding.Rule)
		}
	}
}

func TestContinuityFlagsUnknownNamedEntity(t *testing.T) {
	findings := CheckContinuity(ContinuityInput{
		Narration: "Captain Kaelen nods, and Seraphine Vex steps forward.",
		Context:   harness.TurnContext{Refs: []harness.Ref{{Kind: harness.RefEntity, ID: "captain-kaelen"}}},
	})
	if !hasRule(findings, RuleUnknownEntity) {
		t.Fatalf("expected an unknown-entity finding, got %+v", findings)
	}
}

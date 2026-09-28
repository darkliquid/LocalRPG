package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/trace"
)

func engagementOrchestrator(t *testing.T) (*TurnOrchestrator, *rules.JSEngine) {
	t.Helper()

	tempDir := t.TempDir()
	store := newTestStore(t)
	paths := core.NewPathResolver(tempDir)
	if err := os.MkdirAll(paths.GameDir("campaign-01"), 0755); err != nil {
		t.Fatal(err)
	}
	timeline := NewTimeline(paths, store, NewHistoryLogger(filepath.Join(paths.GameDir("campaign-01"), "history.jsonl")), "campaign-01")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "tavern", Name: "Alden Tavern", Type: "location", Body: "Cozy."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "player", Name: "Sean", Type: "character", Body: "A traveller.", Location: "[[tavern]]"})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	jsEngine := rules.NewJSEngine(rules.NewHostBridge(store, nil, "player"))
	orchestrator := NewTurnOrchestrator(store, timeline, jsEngine, harness.NewRouter(), "tavern", "player")
	orchestrator.SetLogger(trace.Nop())
	return orchestrator, jsEngine
}

func TestWorldTickDue(t *testing.T) {
	cases := []struct {
		cadence int
		turn    int
		want    bool
	}{
		{0, 1, false},
		{1, 1, true},
		{2, 1, true},
		{2, 2, false},
		{2, 3, true},
		{3, 4, true},
	}
	for _, tc := range cases {
		o := &TurnOrchestrator{worldTickTurns: tc.cadence}
		if got := o.worldTickDue(tc.turn); got != tc.want {
			t.Errorf("cadence %d turn %d: due = %v, want %v", tc.cadence, tc.turn, got, tc.want)
		}
	}
}

func TestApplyDirectivesPrependsInOrder(t *testing.T) {
	got := applyDirectives("[MECHANICS RESULT: hit]", []string{"[TICK: the tide turns]", "[TURN: dawn]"})
	want := "[TICK: the tide turns]\n[TURN: dawn]\n[MECHANICS RESULT: hit]"
	if got != want {
		t.Fatalf("applyDirectives = %q, want %q", got, want)
	}
}

func TestHealthOutcomeResolvesZeroEffect(t *testing.T) {
	o, jsEngine := engagementOrchestrator(t)
	if err := jsEngine.HostAPI().SetStat("player", "hp", 0); err != nil {
		t.Fatal(err)
	}
	o.SetHealthSpec(&core.HealthSpec{Stat: "hp", ZeroEffect: "You collapse."})

	if got := o.healthOutcome(); got != "You collapse." {
		t.Fatalf("healthOutcome = %q, want the declared effect", got)
	}
}

func TestHealthOutcomePositiveIsEmpty(t *testing.T) {
	o, jsEngine := engagementOrchestrator(t)
	if err := jsEngine.HostAPI().SetStat("player", "hp", 5); err != nil {
		t.Fatal(err)
	}
	o.SetHealthSpec(&core.HealthSpec{Stat: "hp", ZeroEffect: "You collapse."})

	if got := o.healthOutcome(); got != "" {
		t.Fatalf("healthOutcome = %q, want empty for positive health", got)
	}
}

func TestHealthOutcomeWithoutSpecIsEmpty(t *testing.T) {
	o, jsEngine := engagementOrchestrator(t)
	if err := jsEngine.HostAPI().SetStat("player", "hp", 0); err != nil {
		t.Fatal(err)
	}

	if got := o.healthOutcome(); got != "" {
		t.Fatalf("healthOutcome = %q, want empty without a health schema", got)
	}
}

func TestMechanicsInstructionIncludesPlayerStats(t *testing.T) {
	o, jsEngine := engagementOrchestrator(t)
	if err := jsEngine.HostAPI().SetStat("player", "body", 3); err != nil {
		t.Fatal(err)
	}
	o.SetMechanicsSchema(&core.MechanicsSpec{
		Stats: []core.StatSpec{{ID: "body", Label: "Body"}},
	}, "auto")

	if instruction := o.mechanicsInstruction(); !strings.Contains(instruction, "Player stats: Body 3") {
		t.Fatalf("instruction missing the player's stats: %q", instruction)
	}
}

func TestWorldTickHookInjectsDirective(t *testing.T) {
	o, jsEngine := engagementOrchestrator(t)
	if err := jsEngine.LoadScript(`onWorldTick(function (ctx) { injectGMDirection("The tide turns."); });`); err != nil {
		t.Fatalf("load script: %v", err)
	}
	o.SetWorldTickTurns(1)

	o.runWorldTick(1, "tavern")

	directives := o.drainDirectives()
	if len(directives) != 1 || !strings.Contains(directives[0], "tide turns") {
		t.Fatalf("drained directives = %v, want the world-tick directive", directives)
	}
}

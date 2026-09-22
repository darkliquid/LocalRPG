package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// recallingProvider answers differently on its first call, so a later prompt can
// only contain the first reply if something recovered it.
type recallingProvider struct {
	mu        sync.Mutex
	calls     int
	onRequest func(harness.GenerateRequest)
}

func (p *recallingProvider) ID() string { return "recalling" }

func (p *recallingProvider) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	return &harness.GenerateResponse{Text: "The gate stays shut."}, nil
}

func (p *recallingProvider) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)

	if p.onRequest != nil {
		p.onRequest(req)
	}

	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()

	// The opening turn carries a fact no later turn repeats. He speaks in the turns
	// that follow, which is what keeps him in play once the fact leaves the window:
	// a turn's recorded entities are its wikilinks and its speakers.
	text := "Garrick: \"Ask me again.\""
	if call == 1 {
		text = "[[Garrick]] mentioned the oil was low."
	}
	out <- harness.StreamChunk{Text: text, Done: true}
	return nil
}

// TestAFactSurvivesTheRecallWindow is the coherence contract in one test: a fact
// established in turn 1 is still in the prompt at turn 9, carried by a character
// who is still in play, long after the recall window has moved past it.
func TestAFactSurvivesTheRecallWindow(t *testing.T) {
	var prompts []string
	provider := &recallingProvider{
		onRequest: func(req harness.GenerateRequest) {
			prompts = append(prompts, req.Prompt)
		},
	}

	orchestrator, timeline, store := streamingOrchestrator(t, provider)

	// A fence with a long memory, and two places to have met him. The fact is learnt
	// at the harbour and the party then spends its turns at the tavern, which is the
	// situation only retrieval can serve: the harbour turn is neither in the window
	// nor at the location they are standing in.
	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{
		ID: "garrick", Name: "Garrick", Type: "character", Body: "A fence with a long memory.",
	})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{
		ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Body: "Salt air and gulls.",
	})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{
		ID: "alden-tavern", Name: "Alden Tavern", Type: "location", Body: "Warm, and he keeps a table here.",
	})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}
	movePlayerTo(t, entitiesDir, store, "aldon-harbour")

	if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look around", nil); err != nil {
		t.Fatalf("turn 1 failed: %v", err)
	}

	// The party moves on, and the fact stays behind.
	movePlayerTo(t, entitiesDir, store, "alden-tavern")
	for i := 0; i < 8; i++ {
		if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I wait for an answer", nil); err != nil {
			t.Fatalf("turn %d failed: %v", i+2, err)
		}
	}

	if len(prompts) != 9 {
		t.Fatalf("expected nine prompts, got %d", len(prompts))
	}

	last := prompts[len(prompts)-1]
	historyAt := strings.Index(last, "RELEVANT HISTORY")
	factAt := strings.Index(last, "the oil was low")
	if historyAt == -1 {
		t.Fatalf("expected the last prompt to retrieve relevant history:\n%s", last)
	}
	if factAt == -1 {
		t.Fatalf("the fact from turn 1 was lost by turn 9:\n%s", last)
	}
	// It arrives through retrieval, not through the window, which no longer holds it.
	if factAt < historyAt {
		t.Errorf("the fact should come from the retrieval section, not the window")
	}

	// The window at turn 9 covers the recent tavern turns, and the location section
	// covers what happened here; neither carries the harbour turn.
	windowAt := strings.Index(last, "## RECENT EVENTS")
	if windowAt == -1 || windowAt > historyAt {
		t.Errorf("expected the window to precede the retrieval section")
	}
	before := last[windowAt:historyAt]
	if strings.Contains(before, "the oil was low") {
		t.Errorf("the fixture is wrong: the fact is still above the retrieval section:\n%s", before)
	}
}

// movePlayerTo rewrites the player note's location and reindexes it, which is how
// the orchestrator learns where the party is standing.
func movePlayerTo(t *testing.T, entitiesDir string, store *storage.Store, locationID string) {
	t.Helper()

	note := "---\nid: player\nname: Sean\ntype: character\nlocation: \"[[" + locationID + "]]\"\n---\nA traveller."
	if err := os.WriteFile(filepath.Join(entitiesDir, "player.md"), []byte(note), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}
}

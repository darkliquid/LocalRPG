package gui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// mechanicsFixture is turnFixture plus a system script that reports it ran for
// "do" actions, so a test can observe whether the hook was registered.
func mechanicsFixture(t *testing.T) (string, *Service) {
	t.Helper()
	gameID, svc := turnFixture(t)
	path := filepath.Join(svc.GetResolver().SystemDir("freeform"), "mechanics.js")
	script := `onAction("do", function(ctx) { return { success: true, outcome: "system-ran" }; });`
	if err := os.WriteFile(path, []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	return gameID, svc
}

func TestMechanicsHookRunsOnEveryTurn(t *testing.T) {
	gameID, svc := mechanicsFixture(t)

	for turnNumber := 1; turnNumber <= 2; turnNumber++ {
		session, err := svc.BeginTurn(gameID)
		if err != nil {
			t.Fatalf("BeginTurn %d failed: %v", turnNumber, err)
		}
		var outcome string
		err = session.Run(context.Background(), TurnRequest{Mode: "Do", Input: "I try something risky"}, func(ev TurnEvent) error {
			if ev.Type == "turn" && ev.Turn != nil {
				outcome = ev.Turn.Outcome
			}
			return nil
		})
		session.Close()
		if err != nil {
			t.Fatalf("turn %d failed: %v", turnNumber, err)
		}
		if outcome != "system-ran" {
			t.Errorf("turn %d outcome = %q, want system-ran (mechanics hook did not run)", turnNumber, outcome)
		}
	}
}

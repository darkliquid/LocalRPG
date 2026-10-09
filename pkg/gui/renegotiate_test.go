package gui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

// writeResolvedTurn records a turn whose pending check has already been rolled, so
// a late counter can be refused.
func writeResolvedTurn(t *testing.T, svc *Service, gameID string) {
	t.Helper()
	record := `{"number":1,"timestamp":"2026-10-05T10:00:00Z","mode":"roll","input":"","narration":"The lock gives.","resolves_check_ref":"check-1"}` + "\n"
	path := filepath.Join(svc.GetResolver().GameDir(gameID), "history.jsonl")
	if err := os.WriteFile(path, []byte(record), 0644); err != nil {
		t.Fatal(err)
	}
}

// withScriptedGM replaces the turn router with one scripted GM, so a
// renegotiation can be exercised without a model.
func withScriptedGM(t *testing.T, text string) {
	t.Helper()
	original := turnRouterFactory
	t.Cleanup(func() { turnRouterFactory = original })
	turnRouterFactory = stubRouterFor(&scriptedProvider{id: "gm", text: text})
}

func TestRenegotiateUpdatesPending(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writePendingTurn(t, svc, gameID)
	withScriptedGM(t, `{"ruling":"accept","stakes":"the bar lifts free","difficulty":"risky","reason":"the bar is not a lock"}`)

	result, err := svc.Renegotiate(context.Background(), gameID, 1, RenegotiateRequestDTO{
		PendingRef: "check-1",
		Counter:    harness.CounterProposal{Approach: "lift the bar, not pick the lock"},
	})
	if err != nil {
		t.Fatalf("Renegotiate: %v", err)
	}
	if result.Ruling.Ruling != harness.RulingAccept {
		t.Fatalf("ruling = %+v", result.Ruling)
	}
	if result.PendingCheck == nil || result.PendingCheck.Request.Stakes != "the bar lifts free" {
		t.Fatalf("pending = %+v, want the agreed stakes", result.PendingCheck)
	}
	if result.PendingCheck.Request.Difficulty != "risky" {
		t.Fatalf("difficulty = %q, want the agreed difficulty", result.PendingCheck.Request.Difficulty)
	}
	if len(result.Negotiations) != 1 || result.Negotiations[0].Ruling.Ruling != harness.RulingAccept {
		t.Fatalf("negotiations = %+v", result.Negotiations)
	}

	// The turn on disk carries the agreed terms and the exchange.
	turns, err := svc.cachedHistory(gameID)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0].PendingCheck == nil {
		t.Fatalf("turns = %+v", turns)
	}
	if turns[0].PendingCheck.Request.Stakes != "the bar lifts free" {
		t.Fatalf("the recorded check was not updated: %+v", turns[0].PendingCheck.Request)
	}
	if len(turns[0].Negotiations) != 1 {
		t.Fatalf("the negotiation was not recorded: %+v", turns[0].Negotiations)
	}
}

func TestRenegotiateHoldKeepsTerms(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writePendingTurn(t, svc, gameID)
	withScriptedGM(t, `{"ruling":"hold","difficulty":"trivial","reason":"the lock is the lock"}`)

	result, err := svc.Renegotiate(context.Background(), gameID, 1, RenegotiateRequestDTO{
		PendingRef: "check-1",
		Counter:    harness.CounterProposal{Approach: "just open it"},
	})
	if err != nil {
		t.Fatalf("Renegotiate: %v", err)
	}
	if result.Ruling.Ruling != harness.RulingHold || result.Ruling.Reason == "" {
		t.Fatalf("ruling = %+v", result.Ruling)
	}
	if result.PendingCheck == nil || result.PendingCheck.Request.Difficulty != "" {
		t.Fatalf("a hold must not change the check: %+v", result.PendingCheck)
	}
	// The exchange is still recorded, so the chronicle can show why.
	if len(result.Negotiations) != 1 {
		t.Fatalf("negotiations = %+v", result.Negotiations)
	}
}

func TestRenegotiateRefusesAResolvedCheck(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeResolvedTurn(t, svc, gameID)
	withScriptedGM(t, `{"ruling":"accept"}`)

	_, err := svc.Renegotiate(context.Background(), gameID, 1, RenegotiateRequestDTO{
		PendingRef: "check-1",
		Counter:    harness.CounterProposal{Approach: "too late"},
	})
	if !errors.Is(err, ErrCheckResolved) {
		t.Fatalf("expected a resolved-check refusal, got %v", err)
	}
}

func TestRenegotiateCapsTheArguments(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writePendingTurn(t, svc, gameID)
	withScriptedGM(t, `{"ruling":"adjust","stakes":"adjusted","reason":"partly right"}`)

	for i := 0; i < maxNegotiationsPerCheck; i++ {
		if _, err := svc.Renegotiate(context.Background(), gameID, 1, RenegotiateRequestDTO{
			PendingRef: "check-1",
			Counter:    harness.CounterProposal{Approach: "again"},
		}); err != nil {
			t.Fatalf("counter %d: %v", i+1, err)
		}
	}
	_, err := svc.Renegotiate(context.Background(), gameID, 1, RenegotiateRequestDTO{
		PendingRef: "check-1",
		Counter:    harness.CounterProposal{Approach: "and again"},
	})
	if !errors.Is(err, ErrNegotiationLimit) {
		t.Fatalf("expected the counter cap, got %v", err)
	}
}

func TestRenegotiateRefusesAnEmptyCounter(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writePendingTurn(t, svc, gameID)
	withScriptedGM(t, `{"ruling":"accept"}`)

	if _, err := svc.Renegotiate(context.Background(), gameID, 1, RenegotiateRequestDTO{PendingRef: "check-1"}); err == nil {
		t.Fatal("a counter that says nothing should be refused")
	}
}

func TestRenegotiateRejectsMismatchedRef(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writePendingTurn(t, svc, gameID)
	withScriptedGM(t, `{"ruling":"accept"}`)

	_, err := svc.Renegotiate(context.Background(), gameID, 1, RenegotiateRequestDTO{
		PendingRef: "other",
		Counter:    harness.CounterProposal{Approach: "x"},
	})
	if !errors.Is(err, ErrPendingCheckMismatch) {
		t.Fatalf("expected a mismatch error, got %v", err)
	}
}

func TestRenegotiateRoutePatternIsNamed(t *testing.T) {
	got := routePattern("/api/game/campaign-1/turn/3/renegotiate")
	if got == "http.request" {
		t.Fatalf("renegotiate falls back to the generic span name: %q", got)
	}
}

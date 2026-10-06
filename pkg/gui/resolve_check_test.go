package gui

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func writePendingTurn(t *testing.T, svc *Service, gameID string) {
	t.Helper()
	record := `{"number":1,"timestamp":"2026-10-05T10:00:00Z","mode":"ask","input":"I try the lock","narration":"The lock resists.","pending_check":{"ref":"check-1","request":{"actor":"player","check_kind":"do","stakes":"the lock gives","outcomes":{"pass":"open","fail":"stuck"}}}}` + "\n"
	path := filepath.Join(svc.GetResolver().GameDir(gameID), "history.jsonl")
	if err := os.WriteFile(path, []byte(record), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestBeginResolveCheckRejectsUnknownTurn(t *testing.T) {
	gameID, svc := setupTestGame(t)
	if _, _, err := svc.BeginResolveCheck(gameID, 1, ResolveCheckRequestDTO{}); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected a not-found error, got %v", err)
	}
}

func TestBeginResolveCheckRejectsMismatchedRef(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writePendingTurn(t, svc, gameID)
	if _, _, err := svc.BeginResolveCheck(gameID, 1, ResolveCheckRequestDTO{PendingRef: "other"}); !errors.Is(err, ErrPendingCheckMismatch) {
		t.Fatalf("expected a mismatch error, got %v", err)
	}
}

func TestResolveCheckRoutePatternIsNamed(t *testing.T) {
	got := routePattern("/api/game/campaign-1/turn/3/resolve-check")
	if got == "http.request" {
		t.Fatalf("resolve-check falls back to the generic span name: %q", got)
	}
}

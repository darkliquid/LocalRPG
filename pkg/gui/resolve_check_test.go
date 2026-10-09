package gui

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/harness"
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

func TestPendingCheckDTOCarriesArithmetic(t *testing.T) {
	gameID, svc := setupTestGame(t)
	turn := engine.Turn{Number: 1, PendingCheck: &harness.PendingCheck{Ref: "r",
		Request: harness.CheckRequest{Actor: "player-elena", Notation: "2d6", Stat: "hp"}}}
	dto := svc.turnDTO(turn, mustStore(t, svc, gameID), svc.Config(), gameID, "")
	if dto.PendingCheck == nil || dto.PendingCheck.Notation != "2d6" {
		t.Fatalf("pending = %+v", dto.PendingCheck)
	}
	if dto.PendingCheck.ActorValues["hp"] != 24 {
		t.Fatalf("actor values = %+v, want hp 24", dto.PendingCheck.ActorValues)
	}
}

func TestResolveCheckRequestCarriesManualDice(t *testing.T) {
	var req ResolveCheckRequestDTO
	if err := json.Unmarshal([]byte(`{"pending_check_ref":"r","manual_dice":[4,3]}`), &req); err != nil {
		t.Fatal(err)
	}
	if len(req.ManualDice) != 2 || req.ManualDice[0] != 4 || req.ManualDice[1] != 3 {
		t.Fatalf("request = %+v", req)
	}
}

func TestResolveCheckRequestCarriesAManualTotal(t *testing.T) {
	var req ResolveCheckRequestDTO
	if err := json.Unmarshal([]byte(`{"pending_check_ref":"r","manual_result":9}`), &req); err != nil {
		t.Fatal(err)
	}
	if req.ManualResult == nil || *req.ManualResult != 9 {
		t.Fatalf("request = %+v", req)
	}
	if len(req.ManualDice) != 0 {
		t.Fatalf("a total entry should carry no dice: %+v", req)
	}
}

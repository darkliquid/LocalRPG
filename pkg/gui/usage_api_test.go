package gui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestUsageEndpointReturnsTurnRows(t *testing.T) {
	gameID, svc := turnFixture(t)
	svc.RecordUsage(gameID, 1, "gm", harness.Usage{Provider: "gemini", InputTokens: 5})
	server := NewServer(svc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/game/"+gameID+"/usage", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var dto UsageDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
		t.Fatal(err)
	}
	if len(dto.Rows) != 1 || dto.Rows[0].Role != "gm" {
		t.Fatalf("rows = %+v", dto.Rows)
	}
}

func TestLimitsEndpointReturnsBlocks(t *testing.T) {
	_, svc := turnFixture(t)
	svc.limits.Block("gemini", "gm", time.Now().Add(time.Hour))
	server := NewServer(svc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/limits", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var dto LimitsDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
		t.Fatal(err)
	}
	if len(dto.Blocks) != 1 || dto.Blocks[0].Provider != "gemini" {
		t.Fatalf("blocks = %+v", dto.Blocks)
	}
}

func TestGlobalUsageIncludesTheSharedScope(t *testing.T) {
	gameID, svc := turnFixture(t)
	svc.RecordUsage(gameID, 1, "gm", harness.Usage{Provider: "gemini"})
	svc.RecordUsageGlobal("image", harness.Usage{Provider: "imagen", Requests: 1})

	dto, err := svc.GlobalUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, campaign := range dto.Campaigns {
		if campaign.GameID == UsageScopeGlobal {
			found = true
		}
	}
	if !found {
		t.Fatalf("the shared scope is missing from %+v", dto.Campaigns)
	}
	if dto.TotalCost != 0 && len(dto.ByRole) == 0 {
		t.Fatalf("expected a role breakdown, got %+v", dto)
	}
}

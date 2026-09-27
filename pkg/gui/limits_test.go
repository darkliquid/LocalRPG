package gui

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func TestRateLimitBlocksTurnsForThatRoleOnly(t *testing.T) {
	gameID, svc := turnFixture(t)
	key := svc.providerKeyForRole("gm")
	svc.limits.Block(key, "gm", time.Now().Add(time.Hour))

	if _, err := svc.BeginTurn(gameID); err == nil {
		t.Fatal("expected BeginTurn to be blocked")
	} else {
		var limited *harness.ErrRateLimitedUntil
		if !errors.As(err, &limited) || limited.RetryAfter() <= 0 {
			t.Fatalf("want ErrRateLimitedUntil, got %v", err)
		}
	}

	if _, blocked := svc.limits.Blocked(svc.providerKeyForRole("tts"), "tts"); blocked {
		t.Fatal("tts must not be blocked by a gm block")
	}
}

func TestChangingTheProviderClearsTheBlock(t *testing.T) {
	_, svc := turnFixture(t)
	key := svc.providerKeyForRole("gm")
	svc.limits.Block(key, "gm", time.Now().Add(time.Hour))
	svc.limits.Clear(key, "gm")
	if _, ok := svc.limits.Blocked(key, "gm"); ok {
		t.Fatal("clearing must lift the block")
	}
}

func TestNoteFailureBlocksTheProvider(t *testing.T) {
	_, svc := turnFixture(t)
	key := svc.providerKeyForRole("gm")
	svc.noteFailure("gm", provider.RateLimitedf(2*time.Second, "slow down"))
	until, ok := svc.limits.Blocked(key, "gm")
	if !ok || time.Until(until) <= 0 {
		t.Fatalf("blocked = %v, until %v; want a live block", ok, until)
	}

	svc.noteFailure("gm", provider.InsufficientFundsf("out of credits"))
	states := svc.Limits()
	found := false
	for _, state := range states {
		if state.Role == "gm" && state.FundsFailure != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("funds failure not recorded: %+v", states)
	}
	svc.noteSuccess("gm")
	for _, state := range svc.Limits() {
		if state.Role == "gm" && state.FundsFailure != "" {
			t.Fatalf("a later success must clear the funds failure: %+v", state)
		}
	}
}

func TestBlockedTurnReturns429(t *testing.T) {
	gameID, svc := turnFixture(t)
	svc.limits.Block(svc.providerKeyForRole("gm"), "gm", time.Now().Add(time.Hour))

	server := NewServer(svc, nil)
	body := strings.NewReader(`{"mode":"Do","input":"look around"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/game/"+gameID+"/turn", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("a 429 must carry Retry-After")
	}
}

package gui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/sysgen"
)

// malformedProvider answers twice with JSON that cannot be read, so the retry is
// exhausted and the pipeline fails.
func malformedProvider() *sequencedProvider {
	return &sequencedProvider{id: "gen", responses: []string{
		`{"proposals": not json}`,
		`{"proposals": still not json}`,
	}}
}

func TestEnhanceSystemReportsAnUnreadableReply(t *testing.T) {
	svc := sysGenService(t, malformedProvider())
	saveEnhanceFixture(t, svc, "unreadable")

	if _, err := svc.EnhanceSystem(context.Background(), "unreadable", SystemEnhanceRequestDTO{Instruction: "add sanity"}); !errors.Is(err, sysgen.ErrMalformedReply) {
		t.Fatalf("err = %v, want ErrMalformedReply", err)
	}
}

func TestSystemEnhanceRouteReportsAnUnreadableReply(t *testing.T) {
	svc := sysGenService(t, malformedProvider())
	saveEnhanceFixture(t, svc, "unreadable-route")
	srv := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/system/unreadable-route/enhance", strings.NewReader(`{"instruction":"add sanity"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "invalid character") {
		t.Fatalf("the raw parse error should not be the headline: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "could not be read") {
		t.Fatalf("body = %s, want the friendly message", rec.Body.String())
	}
}
package gui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTTSBatchJobsEmptyForFreshCampaign(t *testing.T) {
	gameID, svc := setupTestGame(t)
	jobs, err := svc.TTSBatchJobs(gameID)
	if err != nil {
		t.Fatalf("TTSBatchJobs: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("expected no jobs, got %#v", jobs)
	}
}

func TestStartTTSBatchWithoutBatchProviderFails(t *testing.T) {
	gameID, svc := setupTestGame(t)
	if _, err := svc.StartTTSBatch(context.Background(), gameID); err == nil {
		t.Errorf("expected an error when the provider has no batch API")
	}
}

func TestAllTTSBatchJobsEmpty(t *testing.T) {
	_, svc := setupTestGame(t)
	jobs, err := svc.AllTTSBatchJobs(context.Background())
	if err != nil {
		t.Fatalf("AllTTSBatchJobs: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("expected no jobs, got %#v", jobs)
	}
}

func TestCancelTTSBatchWithoutBatchProviderFails(t *testing.T) {
	gameID, svc := setupTestGame(t)
	if err := svc.CancelTTSBatch(context.Background(), gameID, "job"); err == nil {
		t.Errorf("expected an error when the provider has no batch API")
	}
}

// A batch failure must say why it failed, so the manager can show the reason
// rather than a bare status.
func TestStartTTSBatchRouteSurfacesTheReason(t *testing.T) {
	gameID, svc := setupTestGame(t)
	server := NewServer(svc, http.NotFoundHandler())

	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/game/"+gameID+"/tts/batch", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "batch API") {
		t.Errorf("expected the reason in the body, got %q", rec.Body.String())
	}
}

func TestListAllTTSBatchJobsRouteIsEmpty(t *testing.T) {
	_, svc := setupTestGame(t)
	server := NewServer(svc, http.NotFoundHandler())

	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tts/batch", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
		t.Errorf("body = %q, want an empty list", body)
	}
}

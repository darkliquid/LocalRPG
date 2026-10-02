package gui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/storage"
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
	if _, err := svc.StartTTSBatch(context.Background(), gameID, false); err == nil {
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

func TestBatchJobActiveCoversPhasesAndLegacySucceeded(t *testing.T) {
	cases := []struct {
		name string
		job  storage.TTSJob
		want bool
	}{
		{name: "queued", job: storage.TTSJob{Status: "queued"}, want: true},
		{name: "processing", job: storage.TTSJob{Status: "processing"}, want: true},
		{name: "downloading", job: storage.TTSJob{Status: "downloading"}, want: true},
		{name: "storing", job: storage.TTSJob{Status: "storing"}, want: true},
		{name: "legacy submitted", job: storage.TTSJob{Status: "submitted"}, want: true},
		{name: "legacy running", job: storage.TTSJob{Status: "running"}, want: true},
		{name: "completed", job: storage.TTSJob{Status: "completed"}, want: false},
		{name: "failed", job: storage.TTSJob{Status: "failed"}, want: false},
		{name: "cancelled", job: storage.TTSJob{Status: "cancelled"}, want: false},
		// Legacy: the provider finished but the results were never stored.
		{name: "legacy succeeded unstored", job: storage.TTSJob{Status: "succeeded", RequestCount: 8, Completed: 0}, want: true},
		{name: "legacy succeeded stored", job: storage.TTSJob{Status: "succeeded", RequestCount: 8, Completed: 8}, want: false},
	}
	for _, tc := range cases {
		if got := batchJobActive(tc.job); got != tc.want {
			t.Errorf("%s: batchJobActive = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestActiveBatchJobFindsTheInFlightJob(t *testing.T) {
	gameID, svc := setupTestGame(t)
	store, err := svc.store(gameID)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if _, ok := activeBatchJob(store, gameID); ok {
		t.Errorf("expected no active job for a fresh campaign")
	}
	if err := store.UpsertTTSJob(storage.TTSJob{ID: "j1", GameID: gameID, Status: "completed", RequestCount: 1, Completed: 1}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, ok := activeBatchJob(store, gameID); ok {
		t.Errorf("a completed job must not be treated as active")
	}
	if err := store.UpsertTTSJob(storage.TTSJob{ID: "j2", GameID: gameID, Status: "processing", RequestCount: 3}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	job, ok := activeBatchJob(store, gameID)
	if !ok || job.ID != "j2" {
		t.Errorf("expected j2 to be active, got %#v / %v", job, ok)
	}
}

func TestDeleteAndClearTTSBatch(t *testing.T) {
	gameID, svc := setupTestGame(t)
	store, err := svc.store(gameID)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if err := store.UpsertTTSJob(storage.TTSJob{ID: "done", GameID: gameID, Status: "completed", RequestCount: 1, Completed: 1}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := store.UpsertTTSJob(storage.TTSJob{ID: "running", GameID: gameID, Status: "processing", RequestCount: 1}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// An in-flight job must be cancelled before it can be removed.
	if err := svc.DeleteTTSBatch(context.Background(), gameID, "running"); err == nil {
		t.Errorf("expected deleting an in-flight job to fail")
	}

	if err := svc.DeleteTTSBatch(context.Background(), gameID, "done"); err != nil {
		t.Fatalf("DeleteTTSBatch: %v", err)
	}
	if gone, _ := store.GetTTSJob("done"); gone != nil {
		t.Errorf("expected the finished job to be deleted")
	}

	if err := store.UpsertTTSJob(storage.TTSJob{ID: "failed", GameID: gameID, Status: "failed"}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	removed, err := svc.ClearTTSBatch(context.Background(), gameID)
	if err != nil {
		t.Fatalf("ClearTTSBatch: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
	if live, _ := store.GetTTSJob("running"); live == nil {
		t.Errorf("expected the in-flight job to survive a clear")
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

func TestResumePendingBatchesWithoutBatchProviderIsNoop(t *testing.T) {
	_, svc := setupTestGame(t)
	svc.ResumePendingBatches(context.Background())
}

// A batch poll can wait hours; closing the app must cancel it rather than block
// shutdown, because the job is resumable next launch.
func TestCloseCancelsBackgroundWork(t *testing.T) {
	_, svc := setupTestGame(t)

	observed := make(chan struct{})
	svc.goBackground(func() {
		<-svc.bgCtx.Done()
		close(observed)
	})

	closed := make(chan struct{})
	go func() {
		svc.Close()
		close(closed)
	}()

	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatalf("Close did not cancel background work")
	}

	select {
	case <-observed:
	case <-time.After(time.Second):
		t.Errorf("expected the background work to have observed cancellation")
	}
}

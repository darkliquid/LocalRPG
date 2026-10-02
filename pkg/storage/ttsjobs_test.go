package storage

import (
	"path/filepath"
	"testing"
)

func TestTTSJobRoundTrip(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	job := TTSJob{
		ID:           "job-1",
		GameID:       "game",
		Provider:     "tts:gemini",
		Model:        "gemini-3.8-flash-tts",
		Status:       "submitted",
		InputURI:     "files/abc",
		RequestCount: 3,
	}
	if err := store.UpsertTTSJob(job); err != nil {
		t.Fatalf("UpsertTTSJob: %v", err)
	}

	got, err := store.GetTTSJob("job-1")
	if err != nil {
		t.Fatalf("GetTTSJob: %v", err)
	}
	if got == nil || got.Status != "submitted" || got.RequestCount != 3 || got.InputURI != "files/abc" {
		t.Fatalf("unexpected job %#v", got)
	}

	if err := store.UpdateTTSJobStatus("job-1", "succeeded", 2, []string{"deadbeef"}); err != nil {
		t.Fatalf("UpdateTTSJobStatus: %v", err)
	}
	got, err = store.GetTTSJob("job-1")
	if err != nil {
		t.Fatalf("GetTTSJob after update: %v", err)
	}
	if got.Status != "succeeded" || got.Completed != 2 || len(got.FailedKeys) != 1 || got.FailedKeys[0] != "deadbeef" {
		t.Errorf("unexpected updated job %#v", got)
	}

	jobs, err := store.ListTTSJobs("game")
	if err != nil {
		t.Fatalf("ListTTSJobs: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != "job-1" {
		t.Errorf("unexpected jobs %#v", jobs)
	}

	if missing, err := store.GetTTSJob("nope"); err != nil || missing != nil {
		t.Errorf("expected nil for a missing job, got %#v / %v", missing, err)
	}
}

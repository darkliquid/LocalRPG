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

	if err := store.SetTTSJobError("job-1", "batch output file is empty"); err != nil {
		t.Fatalf("SetTTSJobError: %v", err)
	}
	got, err = store.GetTTSJob("job-1")
	if err != nil {
		t.Fatalf("GetTTSJob after error: %v", err)
	}
	if got.LastError != "batch output file is empty" {
		t.Errorf("LastError = %q, want the recorded reason", got.LastError)
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

func TestTTSJobDeleteAndClearFinished(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	jobs := []TTSJob{
		{ID: "done", GameID: "g", Status: "completed", RequestCount: 2, Completed: 2},
		{ID: "failed", GameID: "g", Status: "failed"},
		{ID: "running", GameID: "g", Status: "processing", RequestCount: 3},
		{ID: "other", GameID: "h", Status: "completed", RequestCount: 1, Completed: 1},
	}
	for _, job := range jobs {
		if err := store.UpsertTTSJob(job); err != nil {
			t.Fatalf("upsert %s: %v", job.ID, err)
		}
	}

	if err := store.DeleteTTSJob("done"); err != nil {
		t.Fatalf("DeleteTTSJob: %v", err)
	}
	if gone, _ := store.GetTTSJob("done"); gone != nil {
		t.Errorf("expected done to be deleted")
	}

	removed, err := store.DeleteFinishedTTSJobs("g")
	if err != nil {
		t.Fatalf("DeleteFinishedTTSJobs: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1 (only the failed job)", removed)
	}
	if live, _ := store.GetTTSJob("running"); live == nil {
		t.Errorf("expected the in-flight job to survive a clear")
	}
	if other, _ := store.GetTTSJob("other"); other == nil {
		t.Errorf("expected another campaign's job to survive a clear of g")
	}

	all, err := store.DeleteFinishedTTSJobs("")
	if err != nil {
		t.Fatalf("DeleteFinishedTTSJobs all: %v", err)
	}
	if all != 1 {
		t.Errorf("removed = %d, want 1 (the other campaign's job)", all)
	}
}

package ttsbatch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// fakeBatch is a BatchTTSClient with scripted poll states.
type fakeBatch struct {
	states    []string
	polls     int
	submitted []media.BatchRequest
	results   []media.BatchResult
}

func (f *fakeBatch) SubmitBatch(_ context.Context, reqs []media.BatchRequest) (media.BatchJobHandle, error) {
	f.submitted = reqs
	return media.BatchJobHandle{ID: "job"}, nil
}

func (f *fakeBatch) PollBatch(context.Context, media.BatchJobHandle) (media.BatchStatus, error) {
	state := "succeeded"
	if f.polls < len(f.states) {
		state = f.states[f.polls]
	}
	f.polls++
	return media.BatchStatus{State: state}, nil
}

func (f *fakeBatch) FetchBatch(context.Context, media.BatchJobHandle) ([]media.BatchResult, error) {
	return f.results, nil
}

func (f *fakeBatch) CancelBatch(context.Context, media.BatchJobHandle) error { return nil }

// memJobs is an in-memory JobStore.
type memJobs struct {
	updated map[string]storage.TTSJob
}

func (m *memJobs) UpsertTTSJob(job storage.TTSJob) error {
	if m.updated == nil {
		m.updated = map[string]storage.TTSJob{}
	}
	m.updated[job.ID] = job
	return nil
}

func (m *memJobs) UpdateTTSJobStatus(id, status string, completed int, failed []string) error {
	job := m.updated[id]
	job.Status = status
	job.Completed = completed
	job.FailedKeys = failed
	m.updated[id] = job
	return nil
}

func TestEngineRunStoresResultsAndRecordsFailures(t *testing.T) {
	client := &fakeBatch{
		states: []string{"running", "succeeded"},
		results: []media.BatchResult{
			{Key: "k1", Audio: media.GenerateToneWAV(440, 0.02)},
			{Key: "k2", Err: errors.New("boom")},
		},
	}
	jobs := &memJobs{}
	cache := media.NewContentCache(t.TempDir())
	engine := New(client, cache, jobs)
	engine.sleep = func(context.Context, time.Duration) error { return nil }

	groups := []media.ClipGroup{{Key: "k1"}, {Key: "k2"}, {Key: "cached", Cached: true}}
	job, err := engine.Run(context.Background(), Options{GameID: "game", Provider: "tts:gemini"}, groups)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if job == nil || job.Completed != 1 {
		t.Fatalf("unexpected job %#v", job)
	}
	if len(job.FailedKeys) != 1 || job.FailedKeys[0] != "k2" {
		t.Errorf("expected k2 to be recorded as failed, got %#v", job.FailedKeys)
	}
	if len(client.submitted) != 2 {
		t.Errorf("expected the cached group to be skipped, got %d requests", len(client.submitted))
	}
	if !cache.Exists("audio", "k1.opus") {
		t.Errorf("expected k1's clip to be written to the cache")
	}
	if jobs.updated["job"].Status != "succeeded" {
		t.Errorf("expected the job to be recorded as succeeded, got %#v", jobs.updated["job"])
	}
}

func TestEngineRunSkipsWhenEverythingIsCached(t *testing.T) {
	engine := New(&fakeBatch{}, media.NewContentCache(t.TempDir()), &memJobs{})
	job, err := engine.Run(context.Background(), Options{}, []media.ClipGroup{{Key: "k", Cached: true}})
	if err != nil || job != nil {
		t.Fatalf("expected no job when everything is cached, got %#v / %v", job, err)
	}
}

func TestEngineRunRespectsMaxPerJob(t *testing.T) {
	client := &fakeBatch{states: []string{"succeeded"}}
	engine := New(client, media.NewContentCache(t.TempDir()), &memJobs{})
	engine.sleep = func(context.Context, time.Duration) error { return nil }

	groups := []media.ClipGroup{{Key: "a"}, {Key: "b"}, {Key: "c"}}
	job, err := engine.Run(context.Background(), Options{MaxPerJob: 2}, groups)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if job.RequestCount != 2 {
		t.Errorf("expected 2 requests under MaxPerJob, got %d", job.RequestCount)
	}
}

// A cancelled context must end a poll at once, so shutdown does not wait out the
// interval (which can be hours for a batch).
func TestEnginePollStopsOnCancellation(t *testing.T) {
	client := &fakeBatch{states: []string{"running", "running", "running"}}
	engine := New(client, media.NewContentCache(t.TempDir()), &memJobs{})
	// Never returns on its own, so only cancellation can end the wait.
	engine.sleep = func(ctx context.Context, _ time.Duration) error {
		<-ctx.Done()
		return ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	if _, err := engine.Run(ctx, Options{}, []media.ClipGroup{{Key: "k"}}); err == nil {
		t.Fatalf("expected a cancelled run to stop with an error")
	}
}

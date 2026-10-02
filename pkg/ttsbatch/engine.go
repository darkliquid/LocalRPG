// Package ttsbatch backfills a campaign's speech offline through a provider's
// batch API, which is cheaper than the interactive path and does not spend
// interactive rate-limit quota. The clip cache is the source of truth for what is
// done, so a run resumes cleanly after a restart.
package ttsbatch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/media/opus"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/trace"
)

const (
	defaultPollInterval = 30 * time.Second
	defaultMaxWait      = 24 * time.Hour
	defaultMaxPerJob    = 100
)

// JobStore is the persistence a run needs.
type JobStore interface {
	UpsertTTSJob(job storage.TTSJob) error
	UpdateTTSJobStatus(id, status string, completed int, failedKeys []string) error
}

// Options configures one backfill run.
type Options struct {
	GameID       string
	Provider     string
	Model        string
	MaxPerJob    int
	PollInterval time.Duration
	MaxWait      time.Duration
	// Force re-renders every group, overwriting the cached clips, rather than
	// only backfilling what is missing.
	Force bool
}

func (o Options) pollInterval() time.Duration {
	if o.PollInterval > 0 {
		return o.PollInterval
	}
	return defaultPollInterval
}

func (o Options) maxWait() time.Duration {
	if o.MaxWait > 0 {
		return o.MaxWait
	}
	return defaultMaxWait
}

func (o Options) maxPerJob() int {
	if o.MaxPerJob > 0 {
		return o.MaxPerJob
	}
	return defaultMaxPerJob
}

// Engine runs batch backfills.
type Engine struct {
	client      media.BatchTTSClient
	cache       *media.ContentCache
	jobs        JobStore
	opusBitrate int
	logger      trace.Logger
	// now and sleep are injectable so a test does not wait on a real clock. sleep
	// is context-aware so a cancelled run stops at once instead of at the end of
	// its interval.
	now   func() time.Time
	sleep func(context.Context, time.Duration) error
}

// New builds an engine over a batch client, the shared clip cache, and a job
// store.
func New(client media.BatchTTSClient, cache *media.ContentCache, jobs JobStore) *Engine {
	return &Engine{
		client:      client,
		cache:       cache,
		jobs:        jobs,
		opusBitrate: opus.DefaultBitrate,
		now:         time.Now,
		sleep:       sleepCtx,
	}
}

// sleepCtx waits for a duration or until the context is cancelled.
func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// SetLogger attaches a trace sink. A nil logger records nothing.
func (e *Engine) SetLogger(logger trace.Logger) { e.logger = trace.OrNil(logger) }

// SetOpusBitrate selects the on-disk Opus bitrate. Out-of-range values fall back
// to the default.
func (e *Engine) SetOpusBitrate(bitrate int) {
	if bitrate < opus.MinBitrate || bitrate > opus.MaxBitrate {
		bitrate = opus.DefaultBitrate
	}
	e.opusBitrate = bitrate
}

// Run submits a job for the uncached groups and, when it finishes, writes each
// result to the cache. It returns the job record, or nil when every group was
// already cached. A run is bounded by MaxPerJob so one submission stays within
// the provider's limits; call it again to continue.
func (e *Engine) Run(ctx context.Context, opts Options, groups []media.ClipGroup) (*storage.TTSJob, error) {
	job, err := e.Submit(ctx, opts, groups)
	if err != nil || job == nil {
		return job, err
	}
	return e.finish(ctx, opts, job)
}

// Submit starts a job for the uncached groups and returns its record without
// waiting. Resume finishes it later, which is what a CLI that does not want to
// block for hours uses.
func (e *Engine) Submit(ctx context.Context, opts Options, groups []media.ClipGroup) (*storage.TTSJob, error) {
	reqs := make([]media.BatchRequest, 0, len(groups))
	for _, group := range groups {
		if group.Key == "" {
			continue
		}
		if group.Cached && !opts.Force {
			continue
		}
		reqs = append(reqs, media.BatchRequest{Key: group.Key, Lines: group.Lines})
	}
	if len(reqs) == 0 {
		return nil, nil
	}
	if max := opts.maxPerJob(); len(reqs) > max {
		reqs = reqs[:max]
	}

	handle, err := e.client.SubmitBatch(ctx, reqs)
	if err != nil {
		return nil, fmt.Errorf("submit tts batch: %w", err)
	}

	job := &storage.TTSJob{
		ID:           handle.ID,
		GameID:       opts.GameID,
		Provider:     opts.Provider,
		Model:        opts.Model,
		Status:       "queued",
		RequestCount: len(reqs),
	}
	if err := e.jobs.UpsertTTSJob(*job); err != nil {
		return job, err
	}
	trace.OrNil(e.logger).Event("media.tts.batch_submitted", map[string]interface{}{
		"job":      handle.ID,
		"requests": len(reqs),
	})
	return job, nil
}

// Resume waits for a submitted job and writes its results to the cache.
func (e *Engine) Resume(ctx context.Context, opts Options, jobID string) (*storage.TTSJob, error) {
	job := &storage.TTSJob{ID: jobID, GameID: opts.GameID, Provider: opts.Provider, Model: opts.Model}
	return e.finish(ctx, opts, job)
}

// finish waits for a job and stores its results, walking the job through the
// lifecycle phases so the manager can say what stage it is at.
func (e *Engine) finish(ctx context.Context, opts Options, job *storage.TTSJob) (*storage.TTSJob, error) {
	handle := media.BatchJobHandle{ID: job.ID, Model: job.Model}
	status, err := e.poll(ctx, handle, opts, func(phase string) {
		_ = e.jobs.UpdateTTSJobStatus(job.ID, phase, job.Completed, job.FailedKeys)
	})
	if err != nil {
		// Keep the last known phase so a cancelled poll leaves the job resumable
		// rather than blanking it.
		if status.State != "" {
			_ = e.jobs.UpdateTTSJobStatus(job.ID, providerPhase(status.State), job.Completed, job.FailedKeys)
		}
		return job, err
	}

	// The provider is done: the results exist but are not in our cache yet. This
	// is "processed", not "completed" — the download and store still have to run,
	// and they start now rather than waiting for a restart.
	_ = e.jobs.UpdateTTSJobStatus(job.ID, "processed", job.Completed, job.FailedKeys)
	trace.OrNil(e.logger).Event("media.tts.batch_processed", map[string]interface{}{
		"job":      job.ID,
		"requests": job.RequestCount,
	})

	_ = e.jobs.UpdateTTSJobStatus(job.ID, "downloading", job.Completed, job.FailedKeys)
	results, err := e.fetchWithRetry(ctx, handle, opts)
	if err != nil {
		return job, err
	}

	_ = e.jobs.UpdateTTSJobStatus(job.ID, "storing", job.Completed, job.FailedKeys)
	completed := 0
	failed := make([]string, 0)
	for _, result := range results {
		if result.Err != nil {
			failed = append(failed, result.Key)
			_ = e.jobs.UpdateTTSJobStatus(job.ID, "storing", completed, failed)
			continue
		}
		if err := e.storeResult(result); err != nil {
			failed = append(failed, result.Key)
			_ = e.jobs.UpdateTTSJobStatus(job.ID, "storing", completed, failed)
			continue
		}
		completed++
		// Report progress as clips land, so the manager shows the store moving
		// rather than a single jump at the end.
		_ = e.jobs.UpdateTTSJobStatus(job.ID, "storing", completed, failed)
	}

	job.Status = "completed"
	job.Completed = completed
	job.FailedKeys = failed
	if err := e.jobs.UpdateTTSJobStatus(job.ID, "completed", completed, failed); err != nil {
		return job, err
	}
	trace.OrNil(e.logger).Event("media.tts.batch_result", map[string]interface{}{
		"job":       job.ID,
		"completed": completed,
		"failed":    len(failed),
	})
	return job, nil
}

// poll waits for a job to finish, checking between waits. onState is called with
// the job's phase whenever the provider's state changes.
func (e *Engine) poll(ctx context.Context, handle media.BatchJobHandle, opts Options, onState func(string)) (media.BatchStatus, error) {
	deadline := e.now().Add(opts.maxWait())
	var status media.BatchStatus
	lastPhase := ""
	for {
		current, err := e.client.PollBatch(ctx, handle)
		if err != nil {
			return status, fmt.Errorf("poll tts batch: %w", err)
		}
		status = current
		if onState != nil {
			if phase := providerPhase(current.State); phase != lastPhase {
				onState(phase)
				lastPhase = phase
			}
		}
		switch current.State {
		case "succeeded":
			return status, nil
		case "failed", "cancelled", "expired":
			return status, fmt.Errorf("tts batch job %s: %s", handle.ID, current.State)
		}
		if !e.now().Before(deadline) {
			return status, fmt.Errorf("tts batch job %s timed out after %s", handle.ID, opts.maxWait())
		}
		if err := e.sleep(ctx, opts.pollInterval()); err != nil {
			return status, fmt.Errorf("tts batch job %s interrupted: %w", handle.ID, err)
		}
	}
}

// fetchWithRetry downloads a finished job's output, retrying while it is empty or
// unreadable: the File API can report a job succeeded before its output file is
// committed, so an empty result is a state to wait out rather than a failure.
func (e *Engine) fetchWithRetry(ctx context.Context, handle media.BatchJobHandle, opts Options) ([]media.BatchResult, error) {
	const attempts = 4
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		results, err := e.client.FetchBatch(ctx, handle)
		if err == nil && len(results) > 0 {
			return results, nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = errors.New("batch output was empty")
		}
		if attempt < attempts-1 {
			if err := e.sleep(ctx, opts.pollInterval()); err != nil {
				return nil, fmt.Errorf("tts batch job %s interrupted: %w", handle.ID, err)
			}
		}
	}
	return nil, fmt.Errorf("fetch tts batch: %w", lastErr)
}

// providerPhase maps a provider batch state to the job phase we record while a
// job is in flight: a job the provider has not started is queued, one it is
// working on is processing, and one it has finished is processed — its output is
// ready but not yet downloaded. The download, store, and completed phases are
// ours to write.
func providerPhase(state string) string {
	switch state {
	case "running":
		return "processing"
	case "succeeded":
		return "processed"
	case "failed", "cancelled", "expired":
		return state
	default:
		return "queued"
	}
}

// storeResult normalises a batch result to the cache's one format and writes it
// under its key, so the app plays the backfilled audio with no further work.
func (e *Engine) storeResult(result media.BatchResult) error {
	pcm, rate, channels, err := media.DecodeProviderAudio(result.Audio, "")
	if err != nil {
		return fmt.Errorf("normalise batch clip %s: %w", result.Key, err)
	}
	encoded, err := opus.Encode(pcm, rate, channels, e.opusBitrate)
	if err != nil {
		return fmt.Errorf("encode batch clip %s: %w", result.Key, err)
	}
	if _, err := e.cache.Put("audio", result.Key+".opus", encoded); err != nil {
		return fmt.Errorf("store batch clip %s: %w", result.Key, err)
	}
	return nil
}

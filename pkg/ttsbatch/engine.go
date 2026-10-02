// Package ttsbatch backfills a campaign's speech offline through a provider's
// batch API, which is cheaper than the interactive path and does not spend
// interactive rate-limit quota. The clip cache is the source of truth for what is
// done, so a run resumes cleanly after a restart.
package ttsbatch

import (
	"context"
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
	// now and sleep are injectable so a test does not wait on a real clock.
	now   func() time.Time
	sleep func(time.Duration)
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
		sleep:       time.Sleep,
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
	reqs := make([]media.BatchRequest, 0, len(groups))
	for _, group := range groups {
		if group.Cached || group.Key == "" {
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
		Status:       "submitted",
		RequestCount: len(reqs),
	}
	if err := e.jobs.UpsertTTSJob(*job); err != nil {
		return job, err
	}
	trace.OrNil(e.logger).Event("media.tts.batch_submitted", map[string]interface{}{
		"job":      handle.ID,
		"requests": len(reqs),
	})

	status, err := e.poll(ctx, handle, opts)
	if err != nil {
		_ = e.jobs.UpdateTTSJobStatus(handle.ID, status.State, status.Completed, nil)
		return job, err
	}

	results, err := e.client.FetchBatch(ctx, handle)
	if err != nil {
		return job, fmt.Errorf("fetch tts batch: %w", err)
	}

	completed := 0
	failed := make([]string, 0)
	for _, result := range results {
		if result.Err != nil {
			failed = append(failed, result.Key)
			continue
		}
		if err := e.storeResult(result); err != nil {
			failed = append(failed, result.Key)
			continue
		}
		completed++
	}

	job.Status = "succeeded"
	job.Completed = completed
	job.FailedKeys = failed
	if err := e.jobs.UpdateTTSJobStatus(handle.ID, "succeeded", completed, failed); err != nil {
		return job, err
	}
	trace.OrNil(e.logger).Event("media.tts.batch_result", map[string]interface{}{
		"job":       handle.ID,
		"completed": completed,
		"failed":    len(failed),
	})
	return job, nil
}

// poll waits for a job to finish, checking between waits.
func (e *Engine) poll(ctx context.Context, handle media.BatchJobHandle, opts Options) (media.BatchStatus, error) {
	deadline := e.now().Add(opts.maxWait())
	var status media.BatchStatus
	for {
		current, err := e.client.PollBatch(ctx, handle)
		if err != nil {
			return status, fmt.Errorf("poll tts batch: %w", err)
		}
		status = current
		switch current.State {
		case "succeeded":
			return status, nil
		case "failed", "cancelled", "expired":
			return status, fmt.Errorf("tts batch job %s: %s", handle.ID, current.State)
		}
		if !e.now().Before(deadline) {
			return status, fmt.Errorf("tts batch job %s timed out after %s", handle.ID, opts.maxWait())
		}
		e.sleep(opts.pollInterval())
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

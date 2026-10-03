package media

import "context"

// BatchRequest is one group to render offline, named by its content-addressed
// key so the result can be written to the cache the app reads.
type BatchRequest struct {
	Key   string
	Lines []SpeakerLine
}

// BatchJobHandle identifies a submitted batch job.
type BatchJobHandle struct {
	ID    string
	Model string
}

// BatchStatus is a batch job's progress.
type BatchStatus struct {
	// State is one of pending, running, succeeded, failed, cancelled, or expired.
	State     string
	Total     int
	Completed int
}

// BatchResult is one rendered group, or the error that stopped it. A failed
// result is recorded and retried in a later job; the others are already cached.
type BatchResult struct {
	Key   string
	Audio []byte
	Err   error
}

// BatchTTSClient is implemented by a provider that can render groups offline
// through a batch API, which is cheaper than the interactive path and does not
// spend interactive rate-limit quota.
type BatchTTSClient interface {
	SubmitBatch(ctx context.Context, reqs []BatchRequest) (BatchJobHandle, error)
	PollBatch(ctx context.Context, h BatchJobHandle) (BatchStatus, error)
	FetchBatch(ctx context.Context, h BatchJobHandle) ([]BatchResult, error)
	CancelBatch(ctx context.Context, h BatchJobHandle) error
}

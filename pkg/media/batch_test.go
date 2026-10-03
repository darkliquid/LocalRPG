package media

import (
	"context"
	"testing"
)

// fakeBatchClient satisfies BatchTTSClient without a network.
type fakeBatchClient struct{}

func (fakeBatchClient) SubmitBatch(context.Context, []BatchRequest) (BatchJobHandle, error) {
	return BatchJobHandle{ID: "job", Model: "model"}, nil
}

func (fakeBatchClient) PollBatch(context.Context, BatchJobHandle) (BatchStatus, error) {
	return BatchStatus{State: "succeeded", Total: 1, Completed: 1}, nil
}

func (fakeBatchClient) FetchBatch(context.Context, BatchJobHandle) ([]BatchResult, error) {
	return []BatchResult{{Key: "key", Audio: []byte("audio")}}, nil
}

func (fakeBatchClient) CancelBatch(context.Context, BatchJobHandle) error { return nil }

func TestBatchTTSClientIsSatisfiable(t *testing.T) {
	var client BatchTTSClient = fakeBatchClient{}

	handle, err := client.SubmitBatch(context.Background(), []BatchRequest{{Key: "key"}})
	if err != nil || handle.ID != "job" {
		t.Fatalf("SubmitBatch: handle %#v, err %v", handle, err)
	}
	status, err := client.PollBatch(context.Background(), handle)
	if err != nil || status.State != "succeeded" {
		t.Fatalf("PollBatch: status %#v, err %v", status, err)
	}
	results, err := client.FetchBatch(context.Background(), handle)
	if err != nil || len(results) != 1 || results[0].Key != "key" {
		t.Fatalf("FetchBatch: results %#v, err %v", results, err)
	}
}

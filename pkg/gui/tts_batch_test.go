package gui

import (
	"context"
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

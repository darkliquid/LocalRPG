package ttsgemini

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/genai"

	"github.com/darkliquid/localrpg/pkg/media"
)

// batchInputLine is one JSONL request: a key naming the group and a
// generateContent request, which the batch API echoes back with its response.
type batchInputLine struct {
	Key     string               `json:"key"`
	Request batchGenerateRequest `json:"request"`
}

type batchGenerateRequest struct {
	Contents         []*genai.Content             `json:"contents"`
	GenerationConfig *genai.GenerateContentConfig `json:"generation_config,omitempty"`
}

// batchOutputLine is one JSONL response: the key and the model's reply.
type batchOutputLine struct {
	Key      string                         `json:"key"`
	Response *genai.GenerateContentResponse `json:"response"`
	Error    *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// SubmitBatch writes the requests to a JSONL file, uploads it through the File
// API, and starts a batch job. The 50% discount and freedom from interactive
// rate limits are why this path exists.
func (c *GeminiTTSClient) SubmitBatch(ctx context.Context, reqs []media.BatchRequest) (media.BatchJobHandle, error) {
	if c.client == nil {
		return media.BatchJobHandle{}, errors.New("gemini tts: client not initialized")
	}
	if len(reqs) == 0 {
		return media.BatchJobHandle{}, errors.New("gemini tts: a batch needs at least one request")
	}

	var payload bytes.Buffer
	for _, req := range reqs {
		line, err := c.batchInputLine(req)
		if err != nil {
			return media.BatchJobHandle{}, err
		}
		payload.Write(line)
		payload.WriteByte('\n')
	}
	file, err := c.client.Files.Upload(ctx, bytes.NewReader(payload.Bytes()), &genai.UploadFileConfig{
		MIMEType:    "jsonl",
		DisplayName: "localrpg-tts-batch",
	})
	if err != nil {
		return media.BatchJobHandle{}, fmt.Errorf("gemini tts: upload batch input: %w", mapGeminiTTSError(err, c.model))
	}

	// The Developer API takes only the uploaded file: "format" is a Vertex-only
	// field, and sending it is rejected outright.
	job, err := c.client.Batches.Create(ctx, c.model, &genai.BatchJobSource{
		FileName: file.Name,
	}, nil)
	if err != nil {
		return media.BatchJobHandle{}, fmt.Errorf("gemini tts: create batch job: %w", mapGeminiTTSError(err, c.model))
	}
	return media.BatchJobHandle{ID: job.Name, Model: c.model}, nil
}

// batchInputLine encodes one JSONL request: a key naming the group and a
// GenerateContentRequest, which is the file format the batch API expects.
func (c *GeminiTTSClient) batchInputLine(req media.BatchRequest) ([]byte, error) {
	text, config, _, err := c.groupRequest(req.Lines)
	if err != nil {
		return nil, fmt.Errorf("gemini tts: build batch request %q: %w", req.Key, err)
	}
	encoded, err := json.Marshal(batchInputLine{
		Key: req.Key,
		Request: batchGenerateRequest{
			Contents:         []*genai.Content{genai.NewContentFromText(text, genai.RoleUser)},
			GenerationConfig: config,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("gemini tts: encode batch request %q: %w", req.Key, err)
	}
	return encoded, nil
}

// PollBatch reports a job's state and progress.
func (c *GeminiTTSClient) PollBatch(ctx context.Context, h media.BatchJobHandle) (media.BatchStatus, error) {
	if c.client == nil {
		return media.BatchStatus{}, errors.New("gemini tts: client not initialized")
	}
	job, err := c.client.Batches.Get(ctx, h.ID, nil)
	if err != nil {
		return media.BatchStatus{}, fmt.Errorf("gemini tts: get batch job: %w", err)
	}
	status := media.BatchStatus{State: batchState(job.State)}
	if job.CompletionStats != nil {
		status.Completed = int(job.CompletionStats.SuccessfulCount)
		status.Total = int(job.CompletionStats.SuccessfulCount + job.CompletionStats.FailedCount)
	}
	return status, nil
}

// FetchBatch downloads a finished job's output and decodes one result per group.
func (c *GeminiTTSClient) FetchBatch(ctx context.Context, h media.BatchJobHandle) ([]media.BatchResult, error) {
	if c.client == nil {
		return nil, errors.New("gemini tts: client not initialized")
	}
	job, err := c.client.Batches.Get(ctx, h.ID, nil)
	if err != nil {
		return nil, fmt.Errorf("gemini tts: get batch job: %w", err)
	}
	if job.Dest == nil || strings.TrimSpace(job.Dest.FileName) == "" {
		return nil, errors.New("gemini tts: batch job has no output file")
	}

	file, err := c.client.Files.Get(ctx, job.Dest.FileName, nil)
	if err != nil {
		return nil, fmt.Errorf("gemini tts: get batch output: %w", err)
	}
	data, err := c.client.Files.Download(ctx, genai.NewDownloadURIFromFile(file), nil)
	if err != nil {
		return nil, fmt.Errorf("gemini tts: download batch output: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		// The File API can report a job succeeded before its output is committed,
		// so an empty file is a retryable state rather than an empty result.
		return nil, errors.New("gemini tts: batch output file is empty")
	}
	return c.decodeBatchOutput(data)
}

// CancelBatch cancels a running job.
func (c *GeminiTTSClient) CancelBatch(ctx context.Context, h media.BatchJobHandle) error {
	if c.client == nil {
		return errors.New("gemini tts: client not initialized")
	}
	if err := c.client.Batches.Cancel(ctx, h.ID, nil); err != nil {
		return fmt.Errorf("gemini tts: cancel batch job: %w", err)
	}
	return nil
}

// decodeBatchOutput reads the output JSONL into one result per group. A line
// that fails to decode or carries an error becomes a failed result, so the
// others are still returned and cached.
func (c *GeminiTTSClient) decodeBatchOutput(data []byte) ([]media.BatchResult, error) {
	results := make([]media.BatchResult, 0)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var decoded batchOutputLine
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			results = append(results, media.BatchResult{Err: fmt.Errorf("decode batch output: %w", err)})
			continue
		}
		if decoded.Error != nil && decoded.Error.Message != "" {
			results = append(results, media.BatchResult{Key: decoded.Key, Err: errors.New(decoded.Error.Message)})
			continue
		}
		audio, err := extractAudio(decoded.Response)
		if err != nil {
			results = append(results, media.BatchResult{Key: decoded.Key, Err: err})
			continue
		}
		results = append(results, media.BatchResult{Key: decoded.Key, Audio: audio})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read batch output: %w", err)
	}
	if len(results) == 0 {
		return nil, errors.New("gemini tts: batch output contained no responses")
	}
	return results, nil
}

// batchState maps the SDK job state onto the media layer's state names.
func batchState(state genai.JobState) string {
	switch state {
	case genai.JobStateQueued, genai.JobStatePending:
		return "pending"
	case genai.JobStateRunning, genai.JobStatePaused, genai.JobStateUpdating:
		return "running"
	case genai.JobStateSucceeded:
		return "succeeded"
	case genai.JobStateFailed, genai.JobStatePartiallySucceeded:
		return "failed"
	case genai.JobStateCancelling, genai.JobStateCancelled:
		return "cancelled"
	case genai.JobStateExpired:
		return "expired"
	default:
		return strings.ToLower(strings.TrimPrefix(string(state), "JOB_STATE_"))
	}
}

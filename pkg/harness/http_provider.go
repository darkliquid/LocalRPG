package harness

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/trace"
)

type HTTPProvider struct {
	id                 string
	endpoint           string
	model              string
	apiKey             string
	opts               GenerationOptions
	client             *http.Client
	logger             trace.Logger
	chunkLimitOverride int
}

func NewHTTPProvider(id, endpoint, model, apiKey string) *HTTPProvider {
	return NewHTTPProviderWithOptions(id, endpoint, model, apiKey, GenerationOptions{})
}

// NewHTTPProviderWithLogger is NewHTTPProviderWithOptions with a trace sink.
func NewHTTPProviderWithLogger(id, endpoint, model, apiKey string, opts GenerationOptions, logger trace.Logger) *HTTPProvider {
	provider := NewHTTPProviderWithOptions(id, endpoint, model, apiKey, opts)
	provider.SetLogger(logger)
	return provider
}

// SetLogger attaches a trace sink. A nil logger records nothing.
func (h *HTTPProvider) SetLogger(logger trace.Logger) {
	h.logger = trace.OrNil(logger)
}

// SetChunkLimit bounds how many wire events one call records.
func (h *HTTPProvider) SetChunkLimit(limit int) {
	h.chunkLimitOverride = limit
}

func (h *HTTPProvider) chunkLimit() int {
	if h.chunkLimitOverride <= 0 {
		return 500
	}
	return h.chunkLimitOverride
}

func NewHTTPProviderWithOptions(id, endpoint, model, apiKey string, opts GenerationOptions) *HTTPProvider {
	return &HTTPProvider{
		id:       id,
		endpoint: strings.TrimRight(endpoint, "/"),
		model:    model,
		apiKey:   apiKey,
		opts:     opts,
		client:   &http.Client{},
	}
}

func (h *HTTPProvider) ID() string {
	return h.id
}

type openAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Stream      bool            `json:"stream"`
	Temperature float64         `json:"temperature,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Stop        []string        `json:"stop,omitempty"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func (h *HTTPProvider) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	out := make(chan StreamChunk, 20)
	var sb strings.Builder

	errCh := make(chan error, 1)
	go func() {
		errCh <- h.Stream(ctx, req, out)
	}()

	for chunk := range out {
		if chunk.Error != nil {
			return nil, chunk.Error
		}
		sb.WriteString(chunk.Text)
	}

	if err := <-errCh; err != nil {
		return nil, err
	}

	return &GenerateResponse{Text: sb.String()}, nil
}

func (h *HTTPProvider) Stream(ctx context.Context, req GenerateRequest, out chan<- StreamChunk) error {
	defer close(out)

	messages := make([]openAIMessage, 0, 2)
	if req.System != "" {
		messages = append(messages, openAIMessage{Role: "system", Content: req.System})
	}
	messages = append(messages, openAIMessage{Role: "user", Content: req.Prompt})

	temperature := req.Temperature
	if temperature == 0 {
		temperature = h.opts.Temperature
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = h.opts.MaxTokens
	}

	payload := openAIChatRequest{
		Model:       h.model,
		Messages:    messages,
		Stream:      true,
		Temperature: temperature,
		MaxTokens:   maxTokens,
		Stop:        h.opts.Stop,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	start := time.Now()
	var firstToken time.Duration
	chunkCount := 0
	byteCount := 0
	wireLines := 0

	// The prompt is recorded once, on context.assembled; the hash and length here
	// let a reader tie the two together without a second copy of the text.
	h.logger = trace.OrNil(h.logger)
	h.logger.Event("provider.request", map[string]interface{}{
		"role":          h.id,
		"kind":          "http",
		"model":         h.model,
		"endpoint":      h.endpoint,
		"temperature":   temperature,
		"max_tokens":    maxTokens,
		"stream":        true,
		"auth_set":      h.apiKey != "",
		"prompt_sha256": hashPrompt(req.Prompt),
		"prompt_chars":  len([]rune(req.Prompt)),
	})

	url := h.endpoint
	if !strings.HasSuffix(url, "/chat/completions") && !strings.HasSuffix(url, "/v1") {
		url = url + "/v1/chat/completions"
	} else if strings.HasSuffix(url, "/v1") {
		url = url + "/chat/completions"
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("new http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if h.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+h.apiKey)
	}

	resp, err := h.client.Do(httpReq)
	if err != nil {
		h.logger.Event("provider.error", map[string]interface{}{"role": h.id, "error": err.Error()})
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		h.logger.Event("provider.error", map[string]interface{}{
			"role":   h.id,
			"status": resp.Status,
			"url":    url,
		})
		return fmt.Errorf("http error %s from %s", resp.Status, url)
	}

	var finishReason string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		if h.logger.Enabled(trace.LevelFull) && wireLines < h.chunkLimit() {
			wireLines++
			h.logger.Event("provider.wire", map[string]interface{}{
				"role":      h.id,
				"direction": "recv",
				"line":      line,
			})
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		eventData := strings.TrimPrefix(line, "data: ")
		if strings.TrimSpace(eventData) == "[DONE]" {
			break
		}

		var chunk openAIChatChunk
		if err := json.Unmarshal([]byte(eventData), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) > 0 {
			if chunk.Choices[0].Delta.Content != "" {
				if firstToken == 0 {
					firstToken = time.Since(start)
				}
				chunkCount++
				byteCount += len(chunk.Choices[0].Delta.Content)
				out <- StreamChunk{Text: chunk.Choices[0].Delta.Content}
			}
			if chunk.Choices[0].FinishReason != "" {
				finishReason = chunk.Choices[0].FinishReason
			}
		}
	}

	if err := scanner.Err(); err != nil {
		h.logger.Event("provider.error", map[string]interface{}{"role": h.id, "error": err.Error()})
		return err
	}
	if finishReason == "" {
		finishReason = "stop"
	}
	h.logger.Event("provider.response", map[string]interface{}{
		"role":           h.id,
		"finish_reason":  finishReason,
		"first_token_ms": firstToken.Milliseconds(),
		"total_ms":       time.Since(start).Milliseconds(),
		"chunks":         chunkCount,
		"bytes":          byteCount,
	})
	out <- StreamChunk{Done: true, FinishReason: finishReason}
	return nil
}

// hashPrompt identifies a prompt without recording it twice. The prompt itself is
// recorded once, on context.assembled.
func hashPrompt(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])
}

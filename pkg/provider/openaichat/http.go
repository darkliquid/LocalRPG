package openaichat

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/telemetry"
	"github.com/darkliquid/localrpg/pkg/trace"
)

type HTTPProvider struct {
	id                 string
	endpoint           string
	model              string
	apiKey             string
	opts               harness.GenerationOptions
	client             *http.Client
	logger             trace.Logger
	chunkLimitOverride int
}

func NewHTTPProvider(id, endpoint, model, apiKey string) *HTTPProvider {
	return NewHTTPProviderWithOptions(id, endpoint, model, apiKey, harness.GenerationOptions{})
}

// NewHTTPProviderWithLogger is NewHTTPProviderWithOptions with a trace sink.
func NewHTTPProviderWithLogger(id, endpoint, model, apiKey string, opts harness.GenerationOptions, logger trace.Logger) *HTTPProvider {
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

func NewHTTPProviderWithOptions(id, endpoint, model, apiKey string, opts harness.GenerationOptions) *HTTPProvider {
	return &HTTPProvider{
		id:       id,
		endpoint: strings.TrimRight(endpoint, "/"),
		model:    model,
		apiKey:   apiKey,
		opts:     opts,
		client:   &http.Client{Transport: telemetry.HTTPTransport(nil)},
	}
}

func (h *HTTPProvider) ID() string {
	return h.id
}

// ToolCallerCapable marks a provider that can accept a tools field and return
// tool calls. It is what "auto" checks, so a provider type that cannot is never
// offered a surface it would ignore or reject.
func (h *HTTPProvider) ToolCallerCapable() bool { return true }

type openAIChatRequest struct {
	Model       string           `json:"model"`
	Messages    []openAIMessage  `json:"messages"`
	Stream      bool             `json:"stream"`
	Temperature float64          `json:"temperature,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Stop        []string         `json:"stop,omitempty"`
	Tools       []openAIToolSpec `json:"tools,omitempty"`
	ToolChoice  string           `json:"tool_choice,omitempty"`
}

type openAIToolSpec struct {
	Type     string             `json:"type"`
	Function openAIFunctionSpec `json:"function"`
}

type openAIFunctionSpec struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIChatChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// toolCallAccumulator reassembles streamed tool calls. Arguments arrive split
// across frames and are identified only by index, so the engine is handed whole
// calls and never a vendor's fragment format.
type toolCallAccumulator struct {
	order []int
	calls map[int]*harness.ToolCall
}

func newToolCallAccumulator() *toolCallAccumulator {
	return &toolCallAccumulator{calls: make(map[int]*harness.ToolCall)}
}

func (a *toolCallAccumulator) add(index int, id, name, arguments string) {
	call, ok := a.calls[index]
	if !ok {
		call = &harness.ToolCall{}
		a.calls[index] = call
		a.order = append(a.order, index)
	}
	if id != "" {
		call.ID = id
	}
	if name != "" {
		call.Name = name
	}
	call.Arguments += arguments
}

func (a *toolCallAccumulator) result() []harness.ToolCall {
	if len(a.order) == 0 {
		return nil
	}
	calls := make([]harness.ToolCall, 0, len(a.order))
	for _, index := range a.order {
		calls = append(calls, *a.calls[index])
	}
	return calls
}

func (h *HTTPProvider) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	out := make(chan harness.StreamChunk, 20)
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

	return &harness.GenerateResponse{Text: sb.String()}, nil
}

func (h *HTTPProvider) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)
	return h.streamOnce(ctx, req, out, true)
}

// streamOnce sends one request. When a server rejects the tools field it is
// retried once without it, because a rejection is a provider limitation rather
// than a turn failure; the trace records why. The channel is closed by Stream, so
// a retry cannot close it twice.
func (h *HTTPProvider) streamOnce(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk, allowTools bool) error {
	messages := h.buildMessages(req)
	tools := h.buildTools(req, allowTools)

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
		Tools:       tools,
		ToolChoice:  req.ToolChoice,
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
		"prompt_sha256": hashPrompt(req.PromptText()),
		"prompt_chars":  len([]rune(req.PromptText())),
		"tools":         len(tools),
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
		body, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
		detail := provider.TruncateDetail(body)
		h.logger.Event("provider.error", map[string]interface{}{
			"role":   h.id,
			"status": resp.Status,
			"url":    url,
			"detail": detail,
		})
		if allowTools && len(req.Tools) > 0 && resp.StatusCode == http.StatusBadRequest {
			h.logger.Event("provider.tools", map[string]interface{}{
				"role":     h.id,
				"offered":  len(req.Tools),
				"rejected": true,
				"reason":   "the provider rejected the tools field",
			})
			return h.streamOnce(ctx, req, out, false)
		}
		if detail != "" {
			return fmt.Errorf("http error %s from %s: %s", resp.Status, url, detail)
		}
		return fmt.Errorf("http error %s from %s", resp.Status, url)
	}

	var finishReason string
	accumulator := newToolCallAccumulator()
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
			for _, call := range chunk.Choices[0].Delta.ToolCalls {
				accumulator.add(call.Index, call.ID, call.Function.Name, call.Function.Arguments)
			}
			if chunk.Choices[0].Delta.Content != "" {
				if firstToken == 0 {
					firstToken = time.Since(start)
				}
				chunkCount++
				byteCount += len(chunk.Choices[0].Delta.Content)
				out <- harness.StreamChunk{Text: chunk.Choices[0].Delta.Content}
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
	out <- harness.StreamChunk{Done: true, FinishReason: finishReason, ToolCalls: accumulator.result()}
	return nil
}

// buildMessages maps a request onto the wire's message shape. Messages win when
// set; otherwise the request's Prompt is sent as one user turn, which is what
// every caller did before the contract grew a conversation.
func (h *HTTPProvider) buildMessages(req harness.GenerateRequest) []openAIMessage {
	if len(req.Messages) == 0 {
		messages := make([]openAIMessage, 0, 2)
		if req.System != "" {
			messages = append(messages, openAIMessage{Role: "system", Content: req.System})
		}
		messages = append(messages, openAIMessage{Role: "user", Content: req.PromptText()})
		return messages
	}

	messages := make([]openAIMessage, 0, len(req.Messages))
	for _, message := range req.Messages {
		role := message.Role
		if role == "tool" && message.ToolCallID == "" {
			role = "user"
		}
		mapped := openAIMessage{Role: role, Content: message.Content, ToolCallID: message.ToolCallID}
		for _, call := range message.ToolCalls {
			wire := openAIToolCall{ID: call.ID, Type: "function"}
			wire.Function.Name = call.Name
			wire.Function.Arguments = call.Arguments
			mapped.ToolCalls = append(mapped.ToolCalls, wire)
		}
		messages = append(messages, mapped)
	}
	return messages
}

// buildTools maps the offered tools, or returns none when tools are not allowed.
func (h *HTTPProvider) buildTools(req harness.GenerateRequest, allowTools bool) []openAIToolSpec {
	if !allowTools || len(req.Tools) == 0 {
		return nil
	}
	tools := make([]openAIToolSpec, 0, len(req.Tools))
	for _, spec := range req.Tools {
		wire := openAIToolSpec{Type: "function"}
		wire.Function.Name = spec.Name
		wire.Function.Description = spec.Description
		wire.Function.Parameters = spec.Parameters
		tools = append(tools, wire)
	}
	return tools
}

// hashPrompt identifies a prompt without recording it twice. The prompt itself is
// recorded once, on context.assembled.
func hashPrompt(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])
}

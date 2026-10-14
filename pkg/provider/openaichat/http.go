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
	authScheme         string
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
		id:         id,
		endpoint:   strings.TrimRight(endpoint, "/"),
		model:      model,
		apiKey:     apiKey,
		authScheme: "Bearer",
		opts:       opts,
		client:     &http.Client{Transport: telemetry.HTTPTransport(nil)},
	}
}

// NewHTTPProviderWithAuth is NewHTTPProviderWithOptions with an explicit
// Authorization scheme, so a compatible gateway that expects a different prefix
// (Inworld uses Basic) reuses this implementation rather than a copy. An empty
// scheme keeps Bearer.
func NewHTTPProviderWithAuth(id, endpoint, model, apiKey, authScheme string, opts harness.GenerationOptions) *HTTPProvider {
	provider := NewHTTPProviderWithOptions(id, endpoint, model, apiKey, opts)
	if strings.TrimSpace(authScheme) != "" {
		provider.authScheme = authScheme
	}
	return provider
}

func (h *HTTPProvider) ID() string {
	return h.id
}

// ToolCallerCapable marks a provider that can accept a tools field and return
// tool calls. It is what "auto" checks, so a provider type that cannot is never
// offered a surface it would ignore or reject.
func (h *HTTPProvider) ToolCallerCapable() bool { return true }

type openAIChatRequest struct {
	Model         string                   `json:"model"`
	Messages      []openAIMessage          `json:"messages"`
	Stream        bool                     `json:"stream"`
	Temperature   float64                  `json:"temperature,omitempty"`
	MaxTokens     int                      `json:"max_tokens,omitempty"`
	Stop          []string                 `json:"stop,omitempty"`
	Tools         []openAIToolSpec         `json:"tools,omitempty"`
	ToolChoice    string                   `json:"tool_choice,omitempty"`
	StreamOptions *openAIChatStreamOptions `json:"stream_options,omitempty"`
}

// openAIChatStreamOptions asks the provider to send a final usage frame.
type openAIChatStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type openAIToolSpec struct {
	Type     string             `json:"type"`
	Function openAIFunctionSpec `json:"function"`
}

type openAIFunctionSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
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

type openAIChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
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
	Usage *openAIChatUsage `json:"usage,omitempty"`
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
	var usage *harness.Usage

	errCh := make(chan error, 1)
	go func() {
		errCh <- h.Stream(ctx, req, out)
	}()

	for chunk := range out {
		if chunk.Error != nil {
			return nil, chunk.Error
		}
		sb.WriteString(chunk.Text)
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}

	if err := <-errCh; err != nil {
		return nil, err
	}

	return &harness.GenerateResponse{Text: sb.String(), Usage: usage}, nil
}

func (h *HTTPProvider) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)
	return h.streamOnce(ctx, req, out, true)
}

// streamOnce sends one request. When a server rejects the tools field it is
// retried once without it, because a rejection is a provider limitation rather
// than a turn failure; the trace records why. The channel is closed by Stream, so
// a retry cannot close it twice.
// streamState is what one streamed response builds up as it is read: the stop the provider
// gave, the usage it billed, the tool calls it accumulated, and the timing a trace records
// beside them.
type streamState struct {
	start        time.Time
	firstToken   time.Duration
	chunkCount   int
	byteCount    int
	wireLines    int
	finishReason string
	usage        *harness.Usage
	toolCalls    *toolCallAccumulator
}

func newStreamState() *streamState {
	return &streamState{start: time.Now(), toolCalls: newToolCallAccumulator()}
}

func (h *HTTPProvider) streamOnce(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk, allowTools bool) error {
	messages := h.buildMessages(req)
	tools := h.buildTools(req, allowTools)
	temperature, maxTokens := h.chatLimits(req)

	payload := openAIChatRequest{
		Model:         h.model,
		Messages:      messages,
		Stream:        true,
		Temperature:   temperature,
		MaxTokens:     maxTokens,
		Stop:          h.opts.Stop,
		Tools:         tools,
		ToolChoice:    req.ToolChoice,
		StreamOptions: &openAIChatStreamOptions{IncludeUsage: true},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	h.logChatRequest(req, tools, temperature, maxTokens)
	url := h.chatURL()

	state := newStreamState()

	resp, err := h.postChatRequest(ctx, url, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
		detail := provider.TruncateDetail(body)
		h.logger.Event("provider.error", map[string]any{
			"role":   h.id,
			"status": resp.Status,
			"url":    url,
			"detail": detail,
		})
		if allowTools && len(req.Tools) > 0 && resp.StatusCode == http.StatusBadRequest {
			h.logger.Event("provider.tools", map[string]any{
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

	if err := h.consumeChatStream(resp.Body, state, out); err != nil {
		return err
	}

	h.logChatResponse(state)
	out <- harness.StreamChunk{Done: true, FinishReason: state.finishReason, ToolCalls: state.toolCalls.result(), Usage: state.usage}
	return nil
}

// chatLimits is the temperature and the token cap a request runs with: the request's own
// when it names one, and the provider's configured default otherwise.
func (h *HTTPProvider) chatLimits(req harness.GenerateRequest) (temperature float64, maxTokens int) {
	temperature = req.Temperature
	if temperature == 0 {
		temperature = h.opts.Temperature
	}
	maxTokens = req.MaxTokens
	if maxTokens == 0 {
		maxTokens = h.opts.MaxTokens
	}
	return temperature, maxTokens
}

// chatURL is the chat completions endpoint: the configured endpoint when it already names
// one, and otherwise the route appended to it.
func (h *HTTPProvider) chatURL() string {
	url := h.endpoint
	if !strings.HasSuffix(url, "/chat/completions") && !strings.HasSuffix(url, "/v1") {
		return url + "/v1/chat/completions"
	}
	if strings.HasSuffix(url, "/v1") {
		return url + "/chat/completions"
	}
	return url
}

// logChatRequest records what was sent. The prompt itself is recorded once, on
// context.assembled; the hash and the length here let a reader tie the two together
// without a second copy of the text.
func (h *HTTPProvider) logChatRequest(req harness.GenerateRequest, tools []openAIToolSpec, temperature float64, maxTokens int) {
	h.logger = trace.OrNil(h.logger)
	h.logger.Event("provider.request", map[string]any{
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
}

// postChatRequest sends the payload, with the provider's own authentication scheme when an
// API key is set.
func (h *HTTPProvider) postChatRequest(ctx context.Context, url string, data []byte) (*http.Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("new http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if h.apiKey != "" {
		scheme := h.authScheme
		if scheme == "" {
			scheme = "Bearer"
		}
		httpReq.Header.Set("Authorization", scheme+" "+h.apiKey)
	}

	resp, err := h.client.Do(httpReq)
	if err != nil {
		h.logger.Event("provider.error", map[string]any{"role": h.id, "error": err.Error()})
		return nil, fmt.Errorf("http request: %w", err)
	}
	return resp, nil
}

// consumeChatStream reads the server-sent events until the provider says it has stopped,
// forwarding text as it arrives. A line that carries nothing is skipped rather than failing
// the turn, because the wire carries keep-alives alongside the events.
func (h *HTTPProvider) consumeChatStream(body io.Reader, state *streamState, out chan<- harness.StreamChunk) error {
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()

		if h.logger.Enabled(trace.LevelFull) && state.wireLines < h.chunkLimit() {
			state.wireLines++
			h.logger.Event("provider.wire", map[string]any{
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

		if chunk.Usage != nil {
			state.usage = &harness.Usage{
				Model:        h.model,
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
			}
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		for _, call := range chunk.Choices[0].Delta.ToolCalls {
			state.toolCalls.add(call.Index, call.ID, call.Function.Name, call.Function.Arguments)
		}
		if chunk.Choices[0].Delta.Content != "" {
			if state.firstToken == 0 {
				state.firstToken = time.Since(state.start)
			}
			state.chunkCount++
			state.byteCount += len(chunk.Choices[0].Delta.Content)
			out <- harness.StreamChunk{Text: chunk.Choices[0].Delta.Content}
		}
		if chunk.Choices[0].FinishReason != "" {
			state.finishReason = chunk.Choices[0].FinishReason
		}
	}

	if err := scanner.Err(); err != nil {
		h.logger.Event("provider.error", map[string]any{"role": h.id, "error": err.Error()})
		return err
	}
	if state.finishReason == "" {
		state.finishReason = "stop"
	}
	return nil
}

// logChatResponse records what came back, with the timing a reader needs to tell a slow
// first token from a slow stream.
func (h *HTTPProvider) logChatResponse(state *streamState) {
	h.logger.Event("provider.response", map[string]any{
		"role":           h.id,
		"finish_reason":  state.finishReason,
		"first_token_ms": state.firstToken.Milliseconds(),
		"total_ms":       time.Since(state.start).Milliseconds(),
		"chunks":         state.chunkCount,
		"bytes":          state.byteCount,
	})
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

package openairesponses

import (
	"bufio"
	"bytes"
	"context"
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

// Options configures one Responses-API provider. Endpoint defaults to the
// public OpenAI API; Effort, when set, switches the request to a reasoning
// model's shape, because reasoning models reject temperature.
type Options struct {
	Endpoint    string
	Model       string
	APIKey      string
	Temperature float64
	MaxTokens   int
	Effort      string
}

// ResponsesProvider talks to OpenAI's Responses API. It streams every call,
// including the ones Generate collects.
type ResponsesProvider struct {
	id                 string
	endpoint           string
	model              string
	apiKey             string
	opts               Options
	client             *http.Client
	logger             trace.Logger
	chunkLimitOverride int
}

// NewResponsesProvider builds the provider for one model on one endpoint.
func NewResponsesProvider(id string, opts Options) *ResponsesProvider {
	endpoint := strings.TrimRight(opts.Endpoint, "/")
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1"
	}
	opts.Endpoint = endpoint
	return &ResponsesProvider{
		id:       id,
		endpoint: endpoint,
		model:    opts.Model,
		apiKey:   opts.APIKey,
		opts:     opts,
		client:   &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: 5 * time.Minute},
		logger:   trace.Nop(),
	}
}

// ID is the configured provider id.
func (p *ResponsesProvider) ID() string { return p.id }

// ToolCallerCapable marks the adapter as able to accept tools and return calls.
func (p *ResponsesProvider) ToolCallerCapable() bool { return true }

// SetLogger attaches a trace sink. A nil logger records nothing.
func (p *ResponsesProvider) SetLogger(logger trace.Logger) { p.logger = trace.OrNil(logger) }

// SetChunkLimit bounds how many wire events one call records.
func (p *ResponsesProvider) SetChunkLimit(limit int) { p.chunkLimitOverride = limit }

func (p *ResponsesProvider) chunkLimit() int {
	if p.chunkLimitOverride <= 0 {
		return 500
	}
	return p.chunkLimitOverride
}

type responsesRequest struct {
	Model           string             `json:"model"`
	Input           []responseItem     `json:"input"`
	Instructions    string             `json:"instructions,omitempty"`
	Stream          bool               `json:"stream"`
	Store           bool               `json:"store"`
	Temperature     float64            `json:"temperature,omitempty"`
	MaxOutputTokens int                `json:"max_output_tokens,omitempty"`
	Tools           []responseTool     `json:"tools,omitempty"`
	ToolChoice      string             `json:"tool_choice,omitempty"`
	Reasoning       *responseReasoning `json:"reasoning,omitempty"`
}

type responseItem struct {
	Type      string            `json:"type"`
	Role      string            `json:"role,omitempty"`
	Content   []responseContent `json:"content,omitempty"`
	CallID    string            `json:"call_id,omitempty"`
	Name      string            `json:"name,omitempty"`
	Arguments string            `json:"arguments,omitempty"`
	Output    string            `json:"output,omitempty"`
}

type responseContent struct {
	Type string `json:"type"` // "input_text" for user turns, "output_text" for assistant turns
	Text string `json:"text"`
}

type responseTool struct {
	Type        string         `json:"type"` // "function"
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
	Strict      bool           `json:"strict"`
}

type responseReasoning struct {
	Effort string `json:"effort,omitempty"`
}

// buildInstructions is the system text: the request's System plus any
// system-role messages, which the engine sometimes folds into the conversation.
func buildInstructions(req harness.GenerateRequest) string {
	var parts []string
	if strings.TrimSpace(req.System) != "" {
		parts = append(parts, req.System)
	}
	for _, m := range req.Messages {
		if m.Role == "system" && strings.TrimSpace(m.Content) != "" {
			parts = append(parts, m.Content)
		}
	}
	return strings.Join(parts, "\n\n")
}

// buildInput maps the conversation onto Responses input items. A caller that
// set only Prompt gets one user item, matching the single-prompt path every
// other adapter serves.
func buildInput(req harness.GenerateRequest) []responseItem {
	userItem := func(text string) responseItem {
		return responseItem{Type: "message", Role: "user", Content: []responseContent{{Type: "input_text", Text: text}}}
	}
	if len(req.Messages) == 0 {
		return []responseItem{userItem(req.PromptText())}
	}
	items := make([]responseItem, 0, len(req.Messages)+1)
	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			continue
		case "assistant":
			item := responseItem{Type: "message", Role: "assistant"}
			if strings.TrimSpace(m.Content) != "" {
				item.Content = []responseContent{{Type: "output_text", Text: m.Content}}
			}
			items = append(items, item)
			for _, call := range m.ToolCalls {
				items = append(items, responseItem{Type: "function_call", CallID: call.ID, Name: call.Name, Arguments: call.Arguments})
			}
		case "tool":
			if m.ToolCallID == "" {
				items = append(items, userItem(m.Content))
				continue
			}
			items = append(items, responseItem{Type: "function_call_output", CallID: m.ToolCallID, Output: m.Content})
		default:
			items = append(items, userItem(m.Content))
		}
	}
	return items
}

// buildTools maps the offered tools, or returns none when they are withdrawn.
func buildTools(req harness.GenerateRequest, allowTools bool) []responseTool {
	if !allowTools || len(req.Tools) == 0 {
		return nil
	}
	tools := make([]responseTool, 0, len(req.Tools))
	for _, spec := range req.Tools {
		tools = append(tools, responseTool{
			Type: "function", Name: spec.Name, Description: spec.Description,
			Parameters: spec.Parameters, Strict: false,
		})
	}
	return tools
}

// requestFor builds the wire payload. store is always false, so nothing
// persists on OpenAI's side, and stream is always true. Temperature is omitted
// whenever effort is set, because reasoning models reject the pair;
// allowTemperature is the retry ladder's escape when a model rejects it anyway.
func (p *ResponsesProvider) requestFor(req harness.GenerateRequest, allowTools, allowTemperature bool) responsesRequest {
	payload := responsesRequest{
		Model:        p.model,
		Input:        buildInput(req),
		Instructions: buildInstructions(req),
		Stream:       true,
		Store:        false,
		Tools:        buildTools(req, allowTools),
	}
	if maxTokens := max(req.MaxTokens, p.opts.MaxTokens); maxTokens > 0 {
		payload.MaxOutputTokens = maxTokens
	}
	if p.opts.Effort != "" {
		payload.Reasoning = &responseReasoning{Effort: p.opts.Effort}
	} else if allowTemperature {
		payload.Temperature = req.Temperature
		if payload.Temperature == 0 {
			payload.Temperature = p.opts.Temperature
		}
	}
	switch req.ToolChoice {
	case "required":
		if len(payload.Tools) > 0 {
			payload.ToolChoice = "required"
		}
	case "none":
		payload.Tools = nil
	}
	return payload
}

func (p *ResponsesProvider) postRequest(ctx context.Context, data []byte) (*http.Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/responses", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("new http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.client.Do(httpReq)
	if err != nil {
		p.logger.Event("provider.error", map[string]any{"role": p.id, "error": err.Error()})
		return nil, fmt.Errorf("http request: %w", err)
	}
	return resp, nil
}

// Generate streams the reply and collects it, so a caller that wants no chunks
// still takes the streaming path and its timeout behaviour.
func (p *ResponsesProvider) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	out := make(chan harness.StreamChunk, 20)
	var sb strings.Builder
	var usage *harness.Usage
	var cached int

	errCh := make(chan error, 1)
	go func() {
		defer close(out)
		errCh <- p.streamWithUsage(ctx, req, out, &cached)
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
	return &harness.GenerateResponse{Text: sb.String(), Usage: usage, CachedTokens: cached}, nil
}

// Stream forwards each delta as it arrives.
func (p *ResponsesProvider) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)
	var cached int
	return p.streamWithUsage(ctx, req, out, &cached)
}

// streamWithUsage is the single send: it posts the payload and drains the SSE
// stream, recording the cache-read count for Generate.
func (p *ResponsesProvider) streamWithUsage(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk, cached *int) error {
	return p.streamOnce(ctx, req, out, cached, true, true)
}

func (p *ResponsesProvider) streamOnce(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk, cached *int, allowTools, allowTemperature bool) error {
	payload := p.requestFor(req, allowTools, allowTemperature)
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	resp, err := p.postRequest(ctx, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
		detail := provider.TruncateDetail(body)
		if resp.StatusCode == http.StatusBadRequest {
			if allowTools && len(req.Tools) > 0 && strings.Contains(detail, "tools") {
				p.logger.Event("provider.tools", map[string]any{
					"role":     p.id,
					"offered":  len(req.Tools),
					"rejected": true,
					"reason":   "the provider rejected the tools field",
				})
				return p.streamOnce(ctx, req, out, cached, false, allowTemperature)
			}
			if allowTemperature && payload.Temperature != 0 && strings.Contains(detail, "temperature") {
				return p.streamOnce(ctx, req, out, cached, allowTools, false)
			}
		}
		if detail != "" {
			return fmt.Errorf("http error %s from %s/responses: %s", resp.Status, p.endpoint, detail)
		}
		return fmt.Errorf("http error %s from %s/responses", resp.Status, p.endpoint)
	}

	state := &streamState{toolCalls: newToolCallAccumulator()}
	if err := p.consumeStream(resp.Body, state, out); err != nil {
		return err
	}
	*cached = state.cached
	if state.finishReason == "" {
		state.finishReason = "stop"
	}
	out <- harness.StreamChunk{Done: true, FinishReason: state.finishReason, ToolCalls: state.toolCalls.result(), Usage: state.usage}
	return nil
}

var _ harness.ModelProvider = (*ResponsesProvider)(nil)

// streamState is what one streamed response builds up as it is read.
type streamState struct {
	finishReason string
	usage        *harness.Usage
	cached       int
	toolCalls    *toolCallAccumulator
	wireLines    int
}

// toolCallAccumulator reassembles streamed function calls, which arrive as an
// item announcement plus argument deltas keyed by the item id.
type toolCallAccumulator struct {
	order []string
	calls map[string]*harness.ToolCall
}

func newToolCallAccumulator() *toolCallAccumulator {
	return &toolCallAccumulator{calls: make(map[string]*harness.ToolCall)}
}

func (a *toolCallAccumulator) open(itemID, callID, name string) {
	if _, ok := a.calls[itemID]; !ok {
		a.calls[itemID] = &harness.ToolCall{ID: callID, Name: name}
		a.order = append(a.order, itemID)
	}
}

func (a *toolCallAccumulator) appendArgs(itemID, delta string) {
	call, ok := a.calls[itemID]
	if !ok {
		a.open(itemID, "", "")
		call = a.calls[itemID]
	}
	call.Arguments += delta
}

func (a *toolCallAccumulator) result() []harness.ToolCall {
	if len(a.order) == 0 {
		return nil
	}
	calls := make([]harness.ToolCall, 0, len(a.order))
	for _, id := range a.order {
		calls = append(calls, *a.calls[id])
	}
	return calls
}

// responseEvent is the union of the typed SSE events the adapter reads. Events
// it does not model are skipped rather than failing the turn.
type responseEvent struct {
	Type  string `json:"type"`
	Delta string `json:"delta"`
	Item  struct {
		ID     string `json:"id"`
		Type   string `json:"type"`
		CallID string `json:"call_id"`
		Name   string `json:"name"`
	} `json:"item"`
	ItemID   string `json:"item_id"`
	Response struct {
		Status string `json:"status"`
		Usage  *struct {
			InputTokens        int `json:"input_tokens"`
			OutputTokens       int `json:"output_tokens"`
			InputTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"input_tokens_details"`
		} `json:"usage"`
	} `json:"response"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// consumeStream reads the typed SSE events until response.completed. Text
// deltas stream straight through; function calls accumulate; the final
// response frame carries usage.
func (p *ResponsesProvider) consumeStream(body io.Reader, state *streamState, out chan<- harness.StreamChunk) error {
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()
		if p.logger.Enabled(trace.LevelFull) && state.wireLines < p.chunkLimit() {
			state.wireLines++
			p.logger.Event("provider.wire", map[string]any{"role": p.id, "direction": "recv", "line": line})
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev responseEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "response.output_text.delta":
			if ev.Delta != "" {
				out <- harness.StreamChunk{Text: ev.Delta}
			}
		case "response.output_item.added":
			if ev.Item.Type == "function_call" {
				state.toolCalls.open(ev.Item.ID, ev.Item.CallID, ev.Item.Name)
			}
		case "response.function_call_arguments.delta":
			state.toolCalls.appendArgs(ev.ItemID, ev.Delta)
		case "response.completed":
			if ev.Response.Usage != nil {
				state.usage = &harness.Usage{
					Model:        p.model,
					InputTokens:  ev.Response.Usage.InputTokens,
					OutputTokens: ev.Response.Usage.OutputTokens,
				}
				state.cached = ev.Response.Usage.InputTokensDetails.CachedTokens
			}
			if ev.Response.Status != "" && ev.Response.Status != "completed" {
				return fmt.Errorf("openai responses: status %q", ev.Response.Status)
			}
		case "error":
			if ev.Error != nil {
				return fmt.Errorf("openai responses: %s", ev.Error.Message)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		p.logger.Event("provider.error", map[string]any{"role": p.id, "error": err.Error()})
		return err
	}
	return nil
}

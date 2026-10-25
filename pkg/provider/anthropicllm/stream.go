package anthropicllm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// streamState is what one streamed response builds up as it is read.
type streamState struct {
	inputTokens   int
	cacheRead     int
	cacheCreation int
	outputTokens  int
	finishReason  string
	wireLines     int
	toolCalls     *toolAccumulator
}

// toolAccumulator reassembles streamed tool_use blocks, which arrive as a
// block announcement plus input JSON deltas keyed by the block index.
type toolAccumulator struct {
	order []int
	calls map[int]*harness.ToolCall
}

func newToolAccumulator() *toolAccumulator {
	return &toolAccumulator{calls: make(map[int]*harness.ToolCall)}
}

func (a *toolAccumulator) open(index int, id, name string) {
	if _, ok := a.calls[index]; !ok {
		a.calls[index] = &harness.ToolCall{ID: id, Name: name}
		a.order = append(a.order, index)
	}
}

func (a *toolAccumulator) appendArgs(index int, delta string) {
	call, ok := a.calls[index]
	if !ok {
		a.open(index, "", "")
		call = a.calls[index]
	}
	call.Arguments += delta
}

func (a *toolAccumulator) result() []harness.ToolCall {
	if len(a.order) == 0 {
		return nil
	}
	calls := make([]harness.ToolCall, 0, len(a.order))
	for _, index := range a.order {
		calls = append(calls, *a.calls[index])
	}
	return calls
}

// finishFor maps Anthropic's stop reasons onto the harness vocabulary the
// engine checks: a turn that stops on a tool call reads as tool_calls.
func finishFor(reason string) string {
	switch reason {
	case "end_turn", "stop_sequence":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	default:
		return "stop"
	}
}

// usage sums what the turn billed: uncached input, cache writes and cache
// reads all arrive as input tokens, and the read count is what Generate
// reports as CachedTokens.
func (p *AnthropicProvider) usage(state *streamState) *harness.Usage {
	return &harness.Usage{
		Model:        p.model,
		InputTokens:  state.inputTokens + state.cacheCreation + state.cacheRead,
		OutputTokens: state.outputTokens,
	}
}

// buildError shapes a non-200 body the way the other HTTP adapters do, so the
// failure region shows the provider's own words.
func buildError(status string, body []byte) error {
	if detail := provider.TruncateDetail(body); detail != "" {
		return fmt.Errorf("anthropic http error %s: %s", status, detail)
	}
	return fmt.Errorf("anthropic http error %s", status)
}

// Generate streams the reply and collects it, so non-streaming callers share
// the stream's timeout behaviour. The cache-read count rides out on CachedTokens.
func (p *AnthropicProvider) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	out := make(chan harness.StreamChunk, 20)
	var sb strings.Builder
	var usage *harness.Usage
	var cached int

	errCh := make(chan error, 1)
	go func() {
		defer close(out)
		errCh <- p.streamOnce(ctx, req, out, &cached)
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
func (p *AnthropicProvider) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)
	var cached int
	return p.streamOnce(ctx, req, out, &cached)
}

func (p *AnthropicProvider) streamOnce(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk, cached *int) error {
	payload := p.buildRequest(req)
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	p.logRequest(req)

	resp, err := p.postRequest(ctx, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
		return buildError(resp.Status, body)
	}

	state := &streamState{toolCalls: newToolAccumulator()}
	if err := p.consumeStream(resp.Body, state, out); err != nil {
		return err
	}
	if state.finishReason == "refusal" {
		return fmt.Errorf("anthropic: the model declined this request")
	}
	*cached = state.cacheRead
	out <- harness.StreamChunk{Done: true, FinishReason: finishFor(state.finishReason), ToolCalls: state.toolCalls.result(), Usage: p.usage(state)}
	return nil
}

// consumeStream reads Anthropic's event/data SSE frames until message_stop.
// Frames the adapter does not model are skipped rather than failing the turn,
// because the wire carries pings and progress markers alongside the content.
func (p *AnthropicProvider) consumeStream(body io.Reader, state *streamState, out chan<- harness.StreamChunk) error {
	scanner := bufio.NewScanner(body)
	var lastEvent string
	for scanner.Scan() {
		line := scanner.Text()
		if p.logger.Enabled(trace.LevelFull) && state.wireLines < p.chunkLimit() {
			state.wireLines++
			p.logger.Event("provider.wire", map[string]any{"role": p.id, "direction": "recv", "line": line})
		}
		if strings.HasPrefix(line, "event: ") {
			lastEvent = strings.TrimPrefix(line, "event: ")
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		switch lastEvent {
		case "message_start":
			var frame struct {
				Message struct {
					Usage struct {
						InputTokens              int `json:"input_tokens"`
						CacheReadInputTokens     int `json:"cache_read_input_tokens"`
						CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
					} `json:"usage"`
				} `json:"message"`
			}
			if err := json.Unmarshal([]byte(payload), &frame); err == nil {
				state.inputTokens = frame.Message.Usage.InputTokens
				state.cacheRead = frame.Message.Usage.CacheReadInputTokens
				state.cacheCreation = frame.Message.Usage.CacheCreationInputTokens
			}
		case "content_block_start":
			var frame struct {
				Index        int `json:"index"`
				ContentBlock struct {
					Type string `json:"type"`
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"content_block"`
			}
			if err := json.Unmarshal([]byte(payload), &frame); err == nil && frame.ContentBlock.Type == "tool_use" {
				state.toolCalls.open(frame.Index, frame.ContentBlock.ID, frame.ContentBlock.Name)
			}
		case "content_block_delta":
			var frame struct {
				Index int `json:"index"`
				Delta struct {
					Type        string `json:"type"`
					Text        string `json:"text"`
					PartialJSON string `json:"partial_json"`
				} `json:"delta"`
			}
			if err := json.Unmarshal([]byte(payload), &frame); err != nil {
				continue
			}
			switch frame.Delta.Type {
			case "text_delta":
				if frame.Delta.Text != "" {
					out <- harness.StreamChunk{Text: frame.Delta.Text}
				}
			case "input_json_delta":
				state.toolCalls.appendArgs(frame.Index, frame.Delta.PartialJSON)
			}
		case "message_delta":
			var frame struct {
				Delta struct {
					StopReason string `json:"stop_reason"`
				} `json:"delta"`
				Usage struct {
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal([]byte(payload), &frame); err == nil {
				if frame.Delta.StopReason != "" {
					state.finishReason = frame.Delta.StopReason
				}
				if frame.Usage.OutputTokens > 0 {
					state.outputTokens = frame.Usage.OutputTokens
				}
			}
		case "message_stop":
			return nil
		case "error":
			var frame struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(payload), &frame); err == nil && frame.Error.Message != "" {
				return fmt.Errorf("anthropic: %s", frame.Error.Message)
			}
		}
	}
	return scanner.Err()
}

package harness

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type HTTPProvider struct {
	id       string
	endpoint string
	model    string
	apiKey   string
	opts     GenerationOptions
	client   *http.Client
}

func NewHTTPProvider(id, endpoint, model, apiKey string) *HTTPProvider {
	return NewHTTPProviderWithOptions(id, endpoint, model, apiKey, GenerationOptions{})
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
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http error %s from %s", resp.Status, url)
	}

	var finishReason string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
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
				out <- StreamChunk{Text: chunk.Choices[0].Delta.Content}
			}
			if chunk.Choices[0].FinishReason != "" {
				finishReason = chunk.Choices[0].FinishReason
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	if finishReason == "" {
		finishReason = "stop"
	}
	out <- StreamChunk{Done: true, FinishReason: finishReason}
	return nil
}

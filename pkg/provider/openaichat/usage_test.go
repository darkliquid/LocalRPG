package openaichat

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestStreamReportsTokenUsage(t *testing.T) {
	var recordedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		recordedBody = string(data)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":4,\"total_tokens\":15}}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	provider := NewHTTPProvider("openai", server.URL, "gpt-4o", "")
	chunks := make(chan harness.StreamChunk, 8)
	if err := provider.Stream(context.Background(), harness.GenerateRequest{Prompt: "hi"}, chunks); err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var last *harness.Usage
	for chunk := range chunks {
		if chunk.Usage != nil {
			last = chunk.Usage
		}
	}
	if last == nil || last.InputTokens != 11 || last.OutputTokens != 4 {
		t.Fatalf("usage = %+v, want 11/4", last)
	}
	if !strings.Contains(recordedBody, `"include_usage":true`) {
		t.Fatalf("request omitted stream_options: %s", recordedBody)
	}
}

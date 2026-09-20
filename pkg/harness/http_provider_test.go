package harness

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPProviderStreaming(t *testing.T) {
	// Mock OpenAI/Ollama SSE server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}

		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Once upon \"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"a time.\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	provider := NewHTTPProvider("mock-ollama", server.URL, "llama3", "")
	req := GenerateRequest{Prompt: "Tell a story"}

	out := make(chan StreamChunk, 10)
	errCh := make(chan error, 1)
	go func() {
		errCh <- provider.Stream(ctx, req, out)
	}()

	var received []string
	for chunk := range out {
		if chunk.Text != "" {
			received = append(received, chunk.Text)
		}
	}

	if err := <-errCh; err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	result := strings.Join(received, "")
	if result != "Once upon a time." {
		t.Errorf("expected 'Once upon a time.', got %q", result)
	}
}

package harness

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestCLIProviderExecution(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use standard echo/sh command to test CLI harness runner
	provider := NewCLIProvider("test-cli", "sh", []string{"-c", "echo 'Hello from CLI harness:' $1", "--"})

	req := GenerateRequest{
		Prompt: "Adventurer",
	}

	res, err := provider.Generate(ctx, req)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if !strings.Contains(res.Text, "Hello from CLI harness: Adventurer") {
		t.Errorf("unexpected output: %q", res.Text)
	}
}

func TestCLIProviderStreaming(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	provider := NewCLIProvider("stream-cli", "sh", []string{"-c", "printf 'Line1 '; sleep 0.05; printf 'Line2'", "--"})

	req := GenerateRequest{Prompt: "test"}
	out := make(chan StreamChunk, 10)

	errCh := make(chan error, 1)
	go func() {
		errCh <- provider.Stream(ctx, req, out)
	}()

	var received []string
	for chunk := range out {
		if chunk.Error != nil {
			t.Fatalf("stream error: %v", chunk.Error)
		}
		if chunk.Text != "" {
			received = append(received, chunk.Text)
		}
	}

	if err := <-errCh; err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	full := strings.Join(received, "")
	if full != "Line1 Line2" {
		t.Errorf("expected 'Line1 Line2', got %q", full)
	}
}

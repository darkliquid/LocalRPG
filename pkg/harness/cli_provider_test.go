package harness

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/trace"
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

func TestCLIProviderReportsCompletionAndExposesOptions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	provider := NewCLIProviderWithOptions("stream-cli", "sh",
		[]string{"-c", "printf '%s-%s' \"$LOCALRPG_MAX_TOKENS\" \"$LOCALRPG_TEMPERATURE\"", "--"},
		GenerationOptions{Temperature: 0.5, MaxTokens: 512})

	out := make(chan StreamChunk, 10)
	errCh := make(chan error, 1)
	go func() {
		errCh <- provider.Stream(ctx, GenerateRequest{Prompt: "x"}, out)
	}()

	var received strings.Builder
	var done bool
	var finishReason string
	for chunk := range out {
		received.WriteString(chunk.Text)
		if chunk.Done {
			done = true
		}
		if chunk.FinishReason != "" {
			finishReason = chunk.FinishReason
		}
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	if !done || finishReason != "stop" {
		t.Errorf("done = %v, finish reason = %q; want true and stop", done, finishReason)
	}
	if got := received.String(); got != "512-0.5" {
		t.Errorf("options did not reach the process, got %q", got)
	}
}

func TestCLIProviderTracesTheCommandWithoutItsPromptArgument(t *testing.T) {
	memory := trace.NewMemory(trace.LevelFull)
	provider := NewCLIProviderWithLogger("gm", "sh", []string{"-c", "printf 'done'", "--"}, GenerationOptions{}, memory)

	out := make(chan StreamChunk, 10)
	errCh := make(chan error, 1)
	go func() {
		errCh <- provider.Stream(context.Background(), GenerateRequest{Prompt: "a secret prompt"}, out)
	}()
	for range out {
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	request, ok := memory.Find("provider.request")
	if !ok {
		t.Fatalf("expected a provider.request event, got %v", memory.Names())
	}
	if request.Fields["arg_count"] != 4 {
		t.Errorf("arg_count = %v, want the command's arguments plus the prompt", request.Fields["arg_count"])
	}
	if _, present := request.Fields["prompt"]; present {
		t.Errorf("the prompt is recorded once, on context.assembled, not here")
	}
	if request.Fields["prompt_chars"] != len([]rune("a secret prompt")) {
		t.Errorf("prompt_chars = %v, want its length", request.Fields["prompt_chars"])
	}

	response, ok := memory.Find("provider.response")
	if !ok {
		t.Fatalf("expected a provider.response event, got %v", memory.Names())
	}
	if response.Fields["exit_code"] != 0 || response.Fields["finish_reason"] != "stop" {
		t.Errorf("unexpected response fields: %+v", response.Fields)
	}
}

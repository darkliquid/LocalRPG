package harness

import (
	"context"
	"errors"
	"testing"
)

type stubProvider struct {
	id    string
	text  string
	err   error
	calls int
}

func (s *stubProvider) ID() string { return s.id }

func (s *stubProvider) Generate(context.Context, GenerateRequest) (*GenerateResponse, error) {
	s.calls++
	return &GenerateResponse{Text: s.text}, s.err
}

func (s *stubProvider) Stream(context.Context, GenerateRequest, chan<- StreamChunk) error { return nil }

func TestGenerateForRoleFallsBackOnEmpty(t *testing.T) {
	primary := &stubProvider{id: "primary", text: ""}
	fallback := &stubProvider{id: "fallback", text: "hello"}
	router := NewRouter()
	router.RegisterProvider(primary)
	router.RegisterProvider(fallback)
	router.AssignRole("gm", "primary")
	router.SetFallback("gm", "fallback")

	res, err := router.GenerateForRole(context.Background(), "gm", GenerateRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("GenerateForRole returned an error: %v", err)
	}
	if res.Text != "hello" {
		t.Fatalf("GenerateForRole text = %q, want the fallback text", res.Text)
	}
	if primary.calls != 1 || fallback.calls != 1 {
		t.Fatalf("calls = primary %d fallback %d, want both once", primary.calls, fallback.calls)
	}
}

func TestGenerateForRoleEmptyEverywhereIsFailure(t *testing.T) {
	router := NewRouter()
	router.RegisterProvider(&stubProvider{id: "primary", text: "   "})
	router.RegisterProvider(&stubProvider{id: "fallback", text: ""})
	router.AssignRole("gm", "primary")
	router.SetFallback("gm", "fallback")

	_, err := router.GenerateForRole(context.Background(), "gm", GenerateRequest{Prompt: "hi"})
	failure, ok := FailureFrom(err)
	if !ok {
		t.Fatalf("error = %v, want a *GenerationFailure", err)
	}
	if failure.Code != FailureEmptyResponse {
		t.Fatalf("code = %q, want %q", failure.Code, FailureEmptyResponse)
	}
	if len(failure.Attempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(failure.Attempts))
	}
}

func TestGenerateForRoleUnassignedIsUnavailable(t *testing.T) {
	_, err := NewRouter().GenerateForRole(context.Background(), "gm", GenerateRequest{Prompt: "hi"})
	failure, ok := FailureFrom(err)
	if !ok || failure.Code != FailureProviderUnavailable {
		t.Fatalf("error = %v, want provider_unavailable", err)
	}
}

func TestProviderIDForRole(t *testing.T) {
	router := NewRouter()
	router.RegisterProvider(&stubProvider{id: "p1", text: "x"})
	router.AssignRole("gm", "p1")
	if got := router.ProviderIDForRole("gm"); got != "p1" {
		t.Fatalf("ProviderIDForRole(gm) = %q, want p1", got)
	}
	if got := router.ProviderIDForRole("other"); got != "" {
		t.Fatalf("ProviderIDForRole(other) = %q, want empty", got)
	}
}

type chunkProvider struct {
	id     string
	chunks []StreamChunk
}

func (p *chunkProvider) ID() string { return p.id }

func (p *chunkProvider) Generate(context.Context, GenerateRequest) (*GenerateResponse, error) {
	return &GenerateResponse{}, nil
}

func (p *chunkProvider) Stream(_ context.Context, _ GenerateRequest, out chan<- StreamChunk) error {
	for _, chunk := range p.chunks {
		out <- chunk
	}
	close(out)
	return nil
}

func TestStreamForRoleFallsBackOnNoChunks(t *testing.T) {
	router := NewRouter()
	router.RegisterProvider(&chunkProvider{id: "primary"})
	router.RegisterProvider(&chunkProvider{id: "fallback", chunks: []StreamChunk{{Text: "hello"}}})
	router.AssignRole("gm", "primary")
	router.SetFallback("gm", "fallback")

	out := make(chan StreamChunk, 8)
	if err := router.StreamForRole(context.Background(), "gm", GenerateRequest{Prompt: "hi"}, out); err != nil {
		t.Fatalf("StreamForRole: %v", err)
	}
	var text string
	for chunk := range out {
		text += chunk.Text
	}
	if text != "hello" {
		t.Fatalf("text = %q, want the fallback text", text)
	}
}

func TestStreamForRoleEmptyEverywhereIsFailure(t *testing.T) {
	router := NewRouter()
	router.RegisterProvider(&chunkProvider{id: "primary"})
	router.RegisterProvider(&chunkProvider{id: "fallback"})
	router.AssignRole("gm", "primary")
	router.SetFallback("gm", "fallback")

	out := make(chan StreamChunk, 8)
	err := router.StreamForRole(context.Background(), "gm", GenerateRequest{Prompt: "hi"}, out)
	failure, ok := FailureFrom(err)
	if !ok || failure.Code != FailureEmptyResponse {
		t.Fatalf("error = %v, want empty_response", err)
	}
	if len(failure.Attempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(failure.Attempts))
	}
}

type errorFirstProvider struct{ err error }

func (p *errorFirstProvider) ID() string { return "error-first" }

func (p *errorFirstProvider) Generate(context.Context, GenerateRequest) (*GenerateResponse, error) {
	return &GenerateResponse{}, nil
}

func (p *errorFirstProvider) Stream(_ context.Context, _ GenerateRequest, out chan<- StreamChunk) error {
	out <- StreamChunk{Error: p.err}
	close(out)
	return nil
}

func TestStreamForRoleClassifiesFirstChunkError(t *testing.T) {
	router := NewRouter()
	router.RegisterProvider(&errorFirstProvider{err: errors.New("connection reset")})
	router.AssignRole("gm", "error-first")

	out := make(chan StreamChunk, 4)
	err := router.StreamForRole(context.Background(), "gm", GenerateRequest{Prompt: "hi"}, out)
	failure, ok := FailureFrom(err)
	if !ok || failure.Code != FailureProviderError {
		t.Fatalf("error = %v, want provider_error", err)
	}
}

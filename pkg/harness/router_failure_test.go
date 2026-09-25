package harness

import (
	"context"
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

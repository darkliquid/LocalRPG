package harness_test

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

type keyRecordingProvider struct{ id string }

func (p *keyRecordingProvider) ID() string { return p.id }
func (p *keyRecordingProvider) Generate(context.Context, harness.GenerateRequest) (*harness.GenerateResponse, error) {
	return &harness.GenerateResponse{Text: "ok", Usage: &harness.Usage{InputTokens: 3, Provider: "adapter-named-itself"}}, nil
}
func (p *keyRecordingProvider) Stream(context.Context, harness.GenerateRequest, chan<- harness.StreamChunk) error {
	return nil
}

type keySink struct{ got []harness.Usage }

func (s *keySink) RecordUsage(_ string, u harness.Usage) { s.got = append(s.got, u) }

func TestRouterStampsTheResolvedKey(t *testing.T) {
	router := harness.NewRouter()
	router.RegisterProvider(&keyRecordingProvider{id: "gm"})
	router.AssignRole("gm", "gm")
	router.AssignRoleKey("gm", "llm:openaichat@localhost:11434")

	sink := &keySink{}
	router.SetUsageRecorder(sink)
	if _, err := router.GenerateForRole(context.Background(), "gm", harness.GenerateRequest{}); err != nil {
		t.Fatalf("GenerateForRole: %v", err)
	}
	if len(sink.got) != 1 {
		t.Fatalf("recorded %d usages, want 1", len(sink.got))
	}
	if sink.got[0].Provider != "llm:openaichat@localhost:11434" {
		t.Errorf("provider = %q, want the resolved key", sink.got[0].Provider)
	}
}

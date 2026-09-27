package media

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

type usageTTS struct{ chars int }

func (u *usageTTS) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	return GenerateToneWAV(440, 0.02), nil
}

func (u *usageTTS) LastUsage() Usage { return Usage{Characters: u.chars} }

func TestPipelineReportsUsageOnMissOnly(t *testing.T) {
	client := &usageTTS{chars: 42}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))
	voice := &entity.VoiceConfig{Provider: "stub", VoiceID: "v1"}

	if _, err := pipeline.SynthesizeUtterance(context.Background(), "s", voice, "hello"); err != nil {
		t.Fatal(err)
	}
	if got := pipeline.LastUsage().Characters; got != 42 {
		t.Fatalf("usage = %d, want 42", got)
	}

	// A second call is a cache hit and must report no usage.
	if _, err := pipeline.SynthesizeUtterance(context.Background(), "s", voice, "hello"); err != nil {
		t.Fatal(err)
	}
	if got := pipeline.LastUsage().Characters; got != 0 {
		t.Fatalf("cache hit reported usage %d, want 0", got)
	}
}

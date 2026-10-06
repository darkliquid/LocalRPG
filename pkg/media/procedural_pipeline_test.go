package media

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/state"
)

func TestSceneRequestDerivesHints(t *testing.T) {
	loc := &entity.Entity{
		ID:    "forest-clearing",
		Tags:  []string{"forest"},
		State: state.NewState(map[string]interface{}{"mood": "grim", "weather": "rain"}),
	}
	req := sceneRequest(loc, "dark fantasy", "hash1")
	if req.Genre != "fantasy" || req.Mood != "grim" || req.Weather != "rain" || req.Seed == "" {
		t.Fatalf("hints = %+v", req)
	}
}

func TestPipelineUsesSceneHintProvider(t *testing.T) {
	hint := &fakeHint{}
	pipeline := NewImagePipeline(hint, NewContentCache(t.TempDir()))
	if _, err := pipeline.GenerateLocationImage(context.Background(), locationFixture(), "dark fantasy", "builtin:", false); err != nil {
		t.Fatalf("GenerateLocationImage: %v", err)
	}
	if hint.got.Seed == "" {
		t.Fatal("the hint-aware provider was not used")
	}
}

func TestPipelineFallsBackToPrompt(t *testing.T) {
	stub := &stubImageClient{body: []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)}
	pipeline := NewImagePipeline(stub, NewContentCache(t.TempDir()))
	if _, err := pipeline.GenerateLocationImage(context.Background(), locationFixture(), "dark fantasy", "builtin:", false); err != nil {
		t.Fatalf("GenerateLocationImage: %v", err)
	}
	if stub.calls != 1 {
		t.Fatalf("expected the prompt path, got %d calls", stub.calls)
	}
}

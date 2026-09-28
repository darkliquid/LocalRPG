package scene

import (
	"context"
	"testing"
)

func TestCompileEmitsTurnProgress(t *testing.T) {
	var updates []Progress
	compiler := NewCompiler(twoLocationSource())

	_, err := compiler.Compile(context.Background(), "campaign-01", Options{
		Progress: func(p Progress) { updates = append(updates, p) },
	})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	var turnUpdates []Progress
	for _, p := range updates {
		if p.Phase == "compile" && p.Total > 0 {
			turnUpdates = append(turnUpdates, p)
		}
	}
	if len(turnUpdates) != 3 {
		t.Fatalf("expected 3 turn updates, got %d: %+v", len(turnUpdates), turnUpdates)
	}
	for i, p := range turnUpdates {
		if p.Total != 3 {
			t.Errorf("update %d: Total = %d, want 3", i, p.Total)
		}
		if p.Done != i+1 {
			t.Errorf("update %d: Done = %d, want %d", i, p.Done, i+1)
		}
	}
}

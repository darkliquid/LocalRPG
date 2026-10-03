package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
)

type mockSceneGen struct {
	data []byte
	err  error
}

func (m *mockSceneGen) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	return m.data, m.err
}

func TestBuildScenePrompt(t *testing.T) {
	loc := &entity.Entity{
		ID:         "tavern",
		Name:       "The Rusty Nail",
		Appearance: "Old wooden beams and a cracked hearth.",
	}
	cue := "Ten years later, the courtyard is quiet and mossy."
	prompt := BuildScenePrompt(cue, loc, "moody oil painting")

	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}
	if !strings.Contains(prompt, cue) {
		t.Errorf("expected prompt to contain cue, got %q", prompt)
	}
	if !strings.Contains(prompt, "The Rusty Nail") {
		t.Errorf("expected prompt to contain location name, got %q", prompt)
	}
}

func TestSceneWorker_GeneratesTurnSceneIllustration(t *testing.T) {
	tempDir := t.TempDir()
	resolver := core.NewPathResolver(tempDir)
	gameID := "test-game"
	_ = os.MkdirAll(filepath.Join(resolver.GameDir(gameID), "assets", "scenes"), 0755)

	gen := &mockSceneGen{data: []byte("<svg>scene turn 10</svg>")}
	worker := NewSceneWorker(resolver, gen)

	readyCh := make(chan string, 1)
	worker.SetOnReady(func(gID string, turnNum int, relPath string) {
		if gID == gameID && turnNum == 10 {
			readyCh <- relPath
		}
	})

	worker.Enqueue(gameID, 10, "A dramatic autumn scene")

	select {
	case relPath := <-readyCh:
		expected := filepath.Join("assets", "scenes", "turn-10.svg")
		if relPath != expected {
			t.Errorf("expected relPath %s, got %s", expected, relPath)
		}
		fullPath := filepath.Join(resolver.GameDir(gameID), relPath)
		if _, err := os.Stat(fullPath); err != nil {
			t.Fatalf("expected scene file on disk: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for scene image generation")
	}
}

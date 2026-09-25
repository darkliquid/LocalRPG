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

func TestBuildPortraitPrompt(t *testing.T) {
	prompt := BuildPortraitPrompt("Elena Vance", "female", "32", "Athletic pilot with silver hair and blast goggles.", "gritty retro sci-fi watercolor")

	if !strings.HasPrefix(prompt, "3/4 bust portrait, looking slightly to the right") {
		t.Errorf("prompt must start with 3/4 bust facing right framing, got: %s", prompt)
	}
	if !strings.Contains(prompt, "gritty retro sci-fi watercolor") {
		t.Errorf("prompt must contain world art style, got: %s", prompt)
	}
	if !strings.Contains(prompt, "female") || !strings.Contains(prompt, "32 years old") {
		t.Errorf("prompt must contain gender and age, got: %s", prompt)
	}
	if !strings.Contains(prompt, "silver hair and blast goggles") {
		t.Errorf("prompt must contain appearance details, got: %s", prompt)
	}
}

type mockPortraitGenerator struct {
	calledWithPrompt string
	returnBytes      []byte
}

func (m *mockPortraitGenerator) GenerateImage(ctx context.Context, kind, prompt string) ([]byte, error) {
	m.calledWithPrompt = prompt
	return m.returnBytes, nil
}

func TestPortraitWorker_Enqueue(t *testing.T) {
	tmpDir := t.TempDir()
	resolver := core.NewPathResolver(tmpDir)

	gameID := "test-game"
	gameDir := resolver.GameDir(gameID)
	entitiesDir := filepath.Join(gameDir, "entities")
	if err := os.MkdirAll(entitiesDir, 0755); err != nil {
		t.Fatal(err)
	}

	char := &entity.Entity{
		ID:         "elena",
		Name:       "Elena",
		Type:       "character",
		Gender:     "female",
		Age:        "28",
		Appearance: "Silver hair, goggles",
	}
	data, err := char.SerializeMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entitiesDir, "elena.md"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Fake PNG image bytes (PNG signature)
	pngBytes := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}
	gen := &mockPortraitGenerator{returnBytes: pngBytes}
	worker := NewPortraitWorker(resolver, nil, gen)

	worker.Enqueue(gameID, char, "oil painting")

	// Wait briefly for goroutine
	deadline := time.Now().Add(2 * time.Second)
	portraitPath := filepath.Join(gameDir, "assets", "portraits", "elena.png")
	for time.Now().Before(deadline) {
		if _, err := os.Stat(portraitPath); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if _, err := os.Stat(portraitPath); err != nil {
		t.Fatalf("expected portrait image file to be created at %s", portraitPath)
	}

	updatedNote, err := os.ReadFile(filepath.Join(entitiesDir, "elena.md"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := entity.ParseMarkdownEntity(updatedNote)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Portrait != filepath.Join("assets", "portraits", "elena.png") {
		t.Errorf("expected portrait path to be updated in entity note, got: %s", parsed.Portrait)
	}
}

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

func (m *mockPortraitGenerator) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
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
	portraitPath := filepath.Join(gameDir, "assets", "portraits", "elena-v1.png")
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
	if parsed.Portrait != filepath.Join("assets", "portraits", "elena-v1.png") {
		t.Errorf("expected portrait path to be updated in entity note, got: %s", parsed.Portrait)
	}
	if parsed.PortraitVersion != 1 {
		t.Errorf("expected PortraitVersion = 1, got %d", parsed.PortraitVersion)
	}
}

func TestPortraitWorker_RegenerateIncrementsVersion(t *testing.T) {
	tmpDir := t.TempDir()
	resolver := core.NewPathResolver(tmpDir)
	gameID := "regen-game"
	gameDir := resolver.GameDir(gameID)
	entitiesDir := filepath.Join(gameDir, "entities")
	if err := os.MkdirAll(entitiesDir, 0755); err != nil {
		t.Fatal(err)
	}
	portraitsDir := filepath.Join(gameDir, "assets", "portraits")
	if err := os.MkdirAll(portraitsDir, 0755); err != nil {
		t.Fatal(err)
	}

	char := &entity.Entity{
		ID: "elena", Name: "Elena", Type: "character",
		Gender: "female", Age: "28", Appearance: "Silver hair",
		Portrait: filepath.Join("assets", "portraits", "elena.jpg"),
	}
	data, err := char.SerializeMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entitiesDir, "elena.md"), data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(portraitsDir, "elena.jpg"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	pngBytes := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}
	gen := &mockPortraitGenerator{returnBytes: pngBytes}
	worker := NewPortraitWorker(resolver, nil, gen)

	relPath, err := worker.Regenerate(context.Background(), gameID, char, "oil painting")
	if err != nil {
		t.Fatalf("Regenerate failed: %v", err)
	}
	if relPath != filepath.Join("assets", "portraits", "elena-v2.png") {
		t.Fatalf("relPath = %q, want elena-v2.png", relPath)
	}
	if _, err := os.Stat(filepath.Join(gameDir, relPath)); err != nil {
		t.Fatalf("expected the new portrait on disk: %v", err)
	}
	// Historical elena.jpg must NOT be deleted!
	if _, err := os.Stat(filepath.Join(portraitsDir, "elena.jpg")); err != nil {
		t.Fatalf("expected previous elena.jpg to be preserved, stat err = %v", err)
	}

	updatedNote, _ := os.ReadFile(filepath.Join(entitiesDir, "elena.md"))
	parsed, _ := entity.ParseMarkdownEntity(updatedNote)
	if parsed.PortraitVersion != 2 {
		t.Errorf("expected PortraitVersion = 2, got %d", parsed.PortraitVersion)
	}
	if len(parsed.PortraitHistory) != 1 || parsed.PortraitHistory[0] != filepath.Join("assets", "portraits", "elena.jpg") {
		t.Errorf("expected PortraitHistory [%s], got %v", filepath.Join("assets", "portraits", "elena.jpg"), parsed.PortraitHistory)
	}
}

func TestPortraitWorker_SetOnReady(t *testing.T) {
	tmpDir := t.TempDir()
	resolver := core.NewPathResolver(tmpDir)
	gameID := "ready-game"
	gameDir := resolver.GameDir(gameID)
	entitiesDir := filepath.Join(gameDir, "entities")
	if err := os.MkdirAll(entitiesDir, 0755); err != nil {
		t.Fatal(err)
	}

	char := &entity.Entity{
		ID: "garrick", Name: "Garrick", Type: "character",
		Gender: "male", Age: "35", Appearance: "Scarred warrior",
	}
	data, err := char.SerializeMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entitiesDir, "garrick.md"), data, 0644); err != nil {
		t.Fatal(err)
	}

	pngBytes := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}
	gen := &mockPortraitGenerator{returnBytes: pngBytes}
	worker := NewPortraitWorker(resolver, nil, gen)

	readyCalled := false
	var readyGame, readyChar, readyRel string
	worker.SetOnReady(func(gID, charID, relPath string) {
		readyCalled = true
		readyGame = gID
		readyChar = charID
		readyRel = relPath
	})

	worker.Enqueue(gameID, char, "sketch")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !readyCalled {
		time.Sleep(10 * time.Millisecond)
	}

	if !readyCalled {
		t.Fatal("expected SetOnReady callback to be called")
	}
	if readyGame != gameID || readyChar != "garrick" || readyRel != filepath.Join("assets", "portraits", "garrick-v1.png") {
		t.Fatalf("unexpected callback args: game=%s, char=%s, rel=%s", readyGame, readyChar, readyRel)
	}
}

func TestPortraitWorker_VersionedPortraitsAndHistoryRetention(t *testing.T) {
	tempDir := t.TempDir()
	resolver := core.NewPathResolver(tempDir)
	gameID := "test-game"
	gameDir := resolver.GameDir(gameID)
	_ = os.MkdirAll(filepath.Join(gameDir, "entities"), 0755)

	gen := &mockPortraitGenerator{returnBytes: []byte("<svg>portrait v1</svg>")}
	worker := NewPortraitWorker(resolver, nil, gen)

	vera := &entity.Entity{
		ID:         "vera",
		Name:       "Vera",
		Type:       "character",
		Appearance: "Young scout",
	}

	// 1. Initial generation creates v1
	path1, err := worker.Regenerate(context.Background(), gameID, vera, "fantasy art")
	if err != nil {
		t.Fatalf("first Regenerate failed: %v", err)
	}
	if path1 != filepath.Join("assets", "portraits", "vera-v1.svg") {
		t.Errorf("expected path assets/portraits/vera-v1.svg, got %s", path1)
	}

	fullPath1 := filepath.Join(gameDir, path1)
	if _, err := os.Stat(fullPath1); err != nil {
		t.Fatalf("v1 file does not exist: %v", err)
	}

	notePath := filepath.Join(gameDir, "entities", "vera.md")
	noteBytes, _ := os.ReadFile(notePath)
	parsedVera, _ := entity.ParseMarkdownEntity(noteBytes)
	if parsedVera.PortraitVersion != 1 {
		t.Errorf("expected PortraitVersion 1, got %d", parsedVera.PortraitVersion)
	}

	// 2. Second generation (evolution) creates v2 and preserves v1
	gen.returnBytes = []byte("<svg>portrait v2</svg>")
	path2, err := worker.Regenerate(context.Background(), gameID, parsedVera, "fantasy art")
	if err != nil {
		t.Fatalf("second Regenerate failed: %v", err)
	}
	if path2 != filepath.Join("assets", "portraits", "vera-v2.svg") {
		t.Errorf("expected path assets/portraits/vera-v2.svg, got %s", path2)
	}

	if _, err := os.Stat(fullPath1); err != nil {
		t.Errorf("v1 file was deleted! Must be retained: %v", err)
	}
	fullPath2 := filepath.Join(gameDir, path2)
	if _, err := os.Stat(fullPath2); err != nil {
		t.Fatalf("v2 file does not exist: %v", err)
	}

	noteBytes2, _ := os.ReadFile(notePath)
	parsedVera2, _ := entity.ParseMarkdownEntity(noteBytes2)
	if parsedVera2.PortraitVersion != 2 {
		t.Errorf("expected PortraitVersion 2, got %d", parsedVera2.PortraitVersion)
	}
	if len(parsedVera2.PortraitHistory) != 1 || parsedVera2.PortraitHistory[0] != path1 {
		t.Errorf("expected PortraitHistory [%s], got %v", path1, parsedVera2.PortraitHistory)
	}
}


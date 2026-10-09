package gui

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestGetCharacterPortraitEndpoint_ProceduralFallback(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)
	setupFreeformSystem(t, svc)

	game, err := svc.CreateGame(context.Background(), CreateGameRequestDTO{
		Name:       "Test Portrait Game",
		SystemID:   "freeform",
		WorldID:    "harbour-realm",
		PlayerName: "Hero Vance",
		Player: PlayerCharacterDTO{
			Appearance: "A tall adventurer.",
			Age:        "30",
		},
	})
	if err != nil {
		t.Fatalf("CreateGame failed: %v", err)
	}

	server := NewServer(svc, nil)
	req := httptest.NewRequest("GET", "/api/game/"+game.ID+"/character/hero-vance/portrait", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "image/svg+xml") {
		t.Errorf("expected image/svg+xml fallback, got %s", w.Header().Get("Content-Type"))
	}
	if !strings.Contains(w.Body.String(), "<svg") {
		t.Errorf("expected SVG body, got %s", w.Body.String())
	}
}

func TestGetCharacterPortraitEndpoint_ExistingPortraitFile(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)
	setupFreeformSystem(t, svc)

	game, err := svc.CreateGame(context.Background(), CreateGameRequestDTO{
		Name:       "Test Portrait Game 2",
		SystemID:   "freeform",
		WorldID:    "harbour-realm",
		PlayerName: "Hero Vance",
		Player: PlayerCharacterDTO{
			Appearance: "A tall adventurer.",
			Age:        "30",
		},
	})
	if err != nil {
		t.Fatalf("CreateGame failed: %v", err)
	}

	// Write a fake portrait image file and update entity frontmatter
	gameDir := svc.GetResolver().GameDir(game.ID)
	portraitsDir := filepath.Join(gameDir, "assets", "portraits")
	if err := os.MkdirAll(portraitsDir, 0755); err != nil {
		t.Fatal(err)
	}
	fakePNG := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}
	portraitRelPath := filepath.Join("assets", "portraits", "hero-vance.png")
	if err := os.WriteFile(filepath.Join(gameDir, portraitRelPath), fakePNG, 0644); err != nil {
		t.Fatal(err)
	}

	// Update entity note on disk
	entityPath := filepath.Join(gameDir, "entities", "hero-vance.md")
	noteContent := "---\nid: hero-vance\nname: Hero Vance\ntype: character\nportrait: " + portraitRelPath + "\n---\nA brave hero."
	if err := os.WriteFile(entityPath, []byte(noteContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveEntity(context.Background(), game.ID, "hero-vance", noteContent); err != nil {
		t.Fatal(err)
	}

	server := NewServer(svc, nil)
	req := httptest.NewRequest("GET", "/api/game/"+game.ID+"/character/hero-vance/portrait", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "image/png") {
		t.Errorf("expected image/png content type, got %s", w.Header().Get("Content-Type"))
	}
	if len(w.Body.Bytes()) != len(fakePNG) {
		t.Errorf("expected %d bytes, got %d", len(fakePNG), len(w.Body.Bytes()))
	}
}

func TestSegmentDTO_IncludesPortraitURLForSpeech(t *testing.T) {
	segments := []entity.TurnSegment{
		{
			Kind: "narration",
			Text: "The wind howls.",
		},
		{
			Kind:      "speech",
			Speaker:   "Elena",
			SpeakerID: "elena",
			Text:      "Look over there!",
		},
	}

	dtos := segmentDTOs(segments, "test-game", clipPlan{}, nil)
	if len(dtos) != 2 {
		t.Fatalf("expected 2 dtos, got %d", len(dtos))
	}
	if dtos[0].PortraitURL != "" {
		t.Errorf("expected narration segment to have empty PortraitURL, got %s", dtos[0].PortraitURL)
	}
	expectedURL := "/api/game/test-game/character/elena/portrait"
	if dtos[1].PortraitURL != expectedURL {
		t.Errorf("expected speech segment PortraitURL %s, got %s", expectedURL, dtos[1].PortraitURL)
	}
}

func TestSegmentDTO_HasCustomPortraitFlag(t *testing.T) {
	seg := entity.TurnSegment{
		Kind:      "speech",
		Speaker:   "Kaelen",
		SpeakerID: "kaelen",
		Text:      "Hello traveler.",
	}
	// When portrait file does not exist, HasCustomPortrait should be false
	dtos := segmentDTOs([]entity.TurnSegment{seg}, "test-game", clipPlan{}, nil)
	if len(dtos) == 0 || dtos[0].HasCustomPortrait {
		t.Fatalf("expected HasCustomPortrait=false for character without custom portrait file, got %v", dtos[0].HasCustomPortrait)
	}

	// When custom portrait exists
	dtosWithCustom := segmentDTOs([]entity.TurnSegment{seg}, "test-game", clipPlan{}, nil, func(string) bool { return true })
	if len(dtosWithCustom) == 0 || !dtosWithCustom[0].HasCustomPortrait {
		t.Fatalf("expected HasCustomPortrait=true with custom checker, got %v", dtosWithCustom[0].HasCustomPortrait)
	}
}

func TestListEntities_ReturnsHasPortraitAndPortraitURL(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)
	setupFreeformSystem(t, svc)

	game, err := svc.CreateGame(context.Background(), CreateGameRequestDTO{
		Name:       "Test List Entities Portrait",
		SystemID:   "freeform",
		WorldID:    "harbour-realm",
		PlayerName: "Hero Vance",
		Player: PlayerCharacterDTO{
			Appearance: "A tall adventurer.",
			Age:        "30",
		},
	})
	if err != nil {
		t.Fatalf("CreateGame failed: %v", err)
	}

	// Write custom portrait for hero-vance
	gameDir := svc.GetResolver().GameDir(game.ID)
	portraitsDir := filepath.Join(gameDir, "assets", "portraits")
	if err := os.MkdirAll(portraitsDir, 0755); err != nil {
		t.Fatal(err)
	}
	fakePNG := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}
	portraitRelPath := filepath.Join("assets", "portraits", "hero-vance.png")
	if err := os.WriteFile(filepath.Join(gameDir, portraitRelPath), fakePNG, 0644); err != nil {
		t.Fatal(err)
	}

	// Update entity note on disk
	entityPath := filepath.Join(gameDir, "entities", "hero-vance.md")
	noteContent := "---\nid: hero-vance\nname: Hero Vance\ntype: character\nportrait: " + portraitRelPath + "\n---\nA brave hero."
	if err := os.WriteFile(entityPath, []byte(noteContent), 0644); err != nil {
		t.Fatal(err)
	}

	summaries, err := svc.ListEntities(context.Background(), game.ID)
	if err != nil {
		t.Fatalf("ListEntities failed: %v", err)
	}

	var heroSummary *EntitySummaryDTO
	for i := range summaries {
		if summaries[i].ID == "hero-vance" {
			heroSummary = &summaries[i]
			break
		}
	}
	if heroSummary == nil {
		t.Fatal("expected hero-vance in summaries")
	}
	if !heroSummary.HasPortrait {
		t.Error("expected HasPortrait to be true for hero-vance")
	}
	expectedURL := "/api/game/" + game.ID + "/character/hero-vance/portrait"
	if heroSummary.PortraitURL != expectedURL {
		t.Errorf("expected PortraitURL %s, got %s", expectedURL, heroSummary.PortraitURL)
	}
}

// TestProceduralPortraitReflectsTheEntity guards that the GUI hands the generator
// the entity's tags: two characters that differ only in tags get different
// fallback portraits, rather than the same silhouette in two colour pairs.
func TestProceduralPortraitReflectsTheEntity(t *testing.T) {
	gameID, svc := setupTestGame(t)
	entitiesDir := filepath.Join(svc.resolver.GameDir(gameID), "entities")

	write := func(id string, tags []string) {
		t.Helper()
		note := "---\nid: " + id + "\nname: " + id + "\ntype: character\ntags: [" + strings.Join(tags, ", ") + "]\n---\nA figure.\n"
		if err := os.WriteFile(filepath.Join(entitiesDir, id+".md"), []byte(note), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("orc-fighter", []string{"orc", "warrior"})
	write("elf-mage", []string{"elf", "mage"})

	store, err := svc.store(gameID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	orc, contentType, err := svc.GetCharacterPortrait(context.Background(), gameID, "orc-fighter")
	if err != nil {
		t.Fatalf("GetCharacterPortrait(orc-fighter): %v", err)
	}
	if !strings.Contains(contentType, "svg") {
		t.Fatalf("expected an SVG fallback, got %s", contentType)
	}
	elf, _, err := svc.GetCharacterPortrait(context.Background(), gameID, "elf-mage")
	if err != nil {
		t.Fatalf("GetCharacterPortrait(elf-mage): %v", err)
	}
	if bytes.Equal(orc, elf) {
		t.Fatal("two characters with different tags rendered the same portrait")
	}
}

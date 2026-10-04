// pkg/gui/service_test.go
package gui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/scene"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func setupTestGame(t *testing.T) (string, *Service) {
	tempDir := t.TempDir()
	gamesDir := filepath.Join(tempDir, "games", "test-campaign")
	entitiesDir := filepath.Join(gamesDir, "entities")
	_ = os.MkdirAll(entitiesDir, 0755)

	// Create test game manifest
	manifestContent := `id: test-campaign
name: Test Campaign
system: core-d20
world: shadow-realm
player: player-elena
`
	_ = os.WriteFile(filepath.Join(gamesDir, "game.yaml"), []byte(manifestContent), 0644)

	// Create player entity
	playerMD := `---
id: player-elena
name: Elena Nightshade
type: character
location: "[[aldon-harbour]]"
state:
  hp: 24
  max_hp: 30
  level: 3
---
A cunning rogue in dark leather.`
	_ = os.WriteFile(filepath.Join(entitiesDir, "player-elena.md"), []byte(playerMD), 0644)

	// Create a location, so art and turn-location tests have something real.
	locationMD := `---
id: aldon-harbour
name: Aldon Harbour
type: location
tags: [harbour, docks]
---
Salt air and gull cries.`
	_ = os.WriteFile(filepath.Join(entitiesDir, "aldon-harbour.md"), []byte(locationMD), 0644)

	// Create NPC entity
	npcMD := `---
id: captain-kaelen
name: Captain Kaelen
type: npc
voice:
  provider: kokoro
  voice_id: bm_george
state:
  attitude: neutral
---
The town watch captain. Speaks with [[player-elena]].`
	_ = os.WriteFile(filepath.Join(entitiesDir, "captain-kaelen.md"), []byte(npcMD), 0644)

	dbPath := filepath.Join(gamesDir, "cache", "index.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatal(err)
	}
	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}

	syncer := storage.NewSyncer(store)
	_, _ = syncer.Sync(entitiesDir)
	_ = store.Close()

	// Media is enabled so playback and art routes have something to serve. The
	// built-in TTS client returns synthetic bytes and needs no external process.
	configYAML := "media:\n  tts:\n    type: builtin\n    default_voice: narrator\n"
	if err := os.WriteFile(filepath.Join(tempDir, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	service := NewService(tempDir)
	return "test-campaign", service
}

func TestGUIService_GetGameState(t *testing.T) {
	gameID, svc := setupTestGame(t)

	state, err := svc.GetGameState(context.Background(), gameID)
	if err != nil {
		t.Fatalf("GetGameState failed: %v", err)
	}

	if state.Player.Name != "Elena Nightshade" {
		t.Errorf("expected player Elena Nightshade, got %s", state.Player.Name)
	}
	if state.Player.State["hp"] != 24 {
		t.Errorf("expected hp 24, got %v", state.Player.State["hp"])
	}
}

func TestGUIService_GetGraph(t *testing.T) {
	gameID, svc := setupTestGame(t)

	graph, err := svc.GetGraph(context.Background(), gameID)
	if err != nil {
		t.Fatalf("GetGraph failed: %v", err)
	}

	if len(graph.Nodes) < 2 {
		t.Fatalf("expected at least 2 nodes, got %d", len(graph.Nodes))
	}

	foundLink := false
	for _, link := range graph.Links {
		if link.Source == "captain-kaelen" && link.Target == "player-elena" {
			foundLink = true
			break
		}
	}
	if !foundLink {
		t.Errorf("expected wikilink edge between captain-kaelen and player-elena")
	}
}

func TestGUIService_EntityCRUD(t *testing.T) {
	gameID, svc := setupTestGame(t)

	ent, err := svc.GetEntity(context.Background(), gameID, "captain-kaelen")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}
	if ent.Name != "Captain Kaelen" {
		t.Errorf("expected Captain Kaelen, got %s", ent.Name)
	}

	// Update entity
	updatedMD := `---
name: Captain Kaelen
type: npc
state:
  attitude: friendly
---
The town watch captain, now an ally.`
	err = svc.SaveEntity(context.Background(), gameID, "captain-kaelen", updatedMD)
	if err != nil {
		t.Fatalf("SaveEntity failed: %v", err)
	}

	updated, err := svc.GetEntity(context.Background(), gameID, "captain-kaelen")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}
	if updated.State["attitude"] != "friendly" {
		t.Errorf("expected attitude friendly, got %v", updated.State["attitude"])
	}
}

func TestSaveEntityRejectsMalformedMarkdown(t *testing.T) {
	gameID, svc := setupTestGame(t)
	path := filepath.Join(svc.resolver.GameDir(gameID), "entities", "captain-kaelen.md")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.SaveEntity(context.Background(), gameID, "captain-kaelen", "no frontmatter here at all"); err == nil {
		t.Fatal("expected SaveEntity to reject malformed markdown")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("a rejected save changed the file on disk")
	}
}

func TestSaveEntityForcesFrontmatterIDToFileName(t *testing.T) {
	gameID, svc := setupTestGame(t)

	md := `---
id: someone-else
name: Captain Kaelen
type: npc
---
Still the watch captain.`
	if err := svc.SaveEntity(context.Background(), gameID, "captain-kaelen", md); err != nil {
		t.Fatalf("SaveEntity failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(svc.resolver.GameDir(gameID), "entities", "captain-kaelen.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "id: captain-kaelen") {
		t.Errorf("expected frontmatter id forced to the file name, got:\n%s", string(data))
	}
}

func TestListEntitiesIncludesMalformedNote(t *testing.T) {
	gameID, svc := setupTestGame(t)
	path := filepath.Join(svc.resolver.GameDir(gameID), "entities", "broken-note.md")
	if err := os.WriteFile(path, []byte("---\nname: [unterminated\n---\nbody"), 0644); err != nil {
		t.Fatal(err)
	}

	summaries, err := svc.ListEntities(context.Background(), gameID)
	if err != nil {
		t.Fatalf("ListEntities failed: %v", err)
	}
	found := false
	for _, s := range summaries {
		if s.ID == "broken-note" {
			found = true
			if !s.ParseError {
				t.Errorf("expected broken-note to be flagged as a parse error")
			}
		}
	}
	if !found {
		t.Errorf("expected the malformed note to remain listed")
	}
}

func TestVoiceForResolvesDisplayName(t *testing.T) {
	gameID, svc := setupTestGame(t)
	if err := svc.SaveEntity(context.Background(), gameID, "captain-kaelen",
		"---\nid: captain-kaelen\nname: Captain Kaelen\ntype: npc\nvoice:\n  voice_id: af_bella\n---\nWatch.\n"); err != nil {
		t.Fatal(err)
	}

	got := svc.voiceFor(gameID)("Captain Kaelen")
	if got == nil || got.VoiceID != "af_bella" {
		t.Fatalf("expected Captain Kaelen's voice, got %+v", got)
	}
}

func TestSegmentClipURLsChangeWithVoice(t *testing.T) {
	segments := []entity.TurnSegment{{
		Kind: entity.SegmentSpeech, Speaker: "Captain Kaelen", SpeakerID: "captain-kaelen", Text: "Halt!",
	}}

	first := segmentDTOs(segments, "game", clipPlanFromVoice(t, func(string) *entity.VoiceConfig {
		return &entity.VoiceConfig{VoiceID: "af_bella"}
	}, segments), func(string) string { return "" })
	second := segmentDTOs(segments, "game", clipPlanFromVoice(t, func(string) *entity.VoiceConfig {
		return &entity.VoiceConfig{VoiceID: "am_adam"}
	}, segments), func(string) string { return "" })

	if len(first[0].AudioURLs) != 1 || len(second[0].AudioURLs) != 1 {
		t.Fatalf("audio_urls = %#v / %#v, want one clip each", first[0].AudioURLs, second[0].AudioURLs)
	}
	if first[0].AudioURLs[0] == second[0].AudioURLs[0] {
		t.Fatalf("audio url did not change with the voice: %q", first[0].AudioURLs[0])
	}
	if !strings.HasPrefix(first[0].AudioURLs[0], "/api/audio/clip/") {
		t.Errorf("expected a content-addressed clip URL, got %q", first[0].AudioURLs[0])
	}
}

// clipKeysFromVoice names clips through a real pipeline, so a DTO test exercises
// the same key computation the app uses.
func clipKeysFromVoice(t *testing.T, voiceFor func(string) *entity.VoiceConfig) func(entity.TurnSegment) []string {
	t.Helper()
	pipeline := media.NewTTSPipeline(&bareClient{}, media.NewContentCache(t.TempDir()))
	return func(segment entity.TurnSegment) []string {
		keys, err := pipeline.SegmentClipKeys(segment, nil, voiceFor)
		if err != nil {
			return nil
		}
		return keys
	}
}

// clipPlanFromVoice builds an ungrouped clip plan for a turn, using the same key
// computation the app uses.
func clipPlanFromVoice(t *testing.T, voiceFor func(string) *entity.VoiceConfig, segments []entity.TurnSegment) clipPlan {
	t.Helper()
	keys := clipKeysFromVoice(t, voiceFor)
	plan := clipPlan{segmentKeys: make([][]string, len(segments)), groupKey: make([]string, len(segments))}
	for i, segment := range segments {
		plan.segmentKeys[i] = keys(segment)
	}
	return plan
}

func TestGetEntityReadsCanonicalDatabase(t *testing.T) {
	gameID, svc := setupTestGame(t)

	ent, err := svc.GetEntity(context.Background(), gameID, "player-elena")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}
	if len(ent.Backlinks) == 0 {
		t.Errorf("expected backlinks from the canonical index, got none")
	}

	gameDir := svc.GetResolver().GameDir(gameID)
	if _, err := os.Stat(filepath.Join(gameDir, "game.db")); !os.IsNotExist(err) {
		t.Errorf("expected no legacy game.db, stat err = %v", err)
	}
	if _, err := os.Stat(svc.GetResolver().GameDBPath(gameID)); err != nil {
		t.Errorf("expected the canonical database: %v", err)
	}
}

func TestGetChronicleAndEntityTurnsReportInvolvement(t *testing.T) {
	gameID, svc := setupTestGame(t)
	gameDir := svc.GetResolver().GameDir(gameID)

	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Do","input":"I ask the captain","narration":"The captain nods.","segments":[{"kind":"narration","text":"The captain nods."}],"entities":[{"id":"player-elena","mention":"player"},{"id":"captain-kaelen","mention":"wikilink"}]}` + "\n"
	if err := os.WriteFile(filepath.Join(gameDir, "history.jsonl"), []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	turns, err := svc.GetChronicle(context.Background(), gameID)
	if err != nil {
		t.Fatalf("GetChronicle failed: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	if len(turns[0].EntitiesHit) != 2 {
		t.Errorf("expected 2 entities hit, got %+v", turns[0].EntitiesHit)
	}
	if len(turns[0].Segments) != 1 || turns[0].Segments[0].Kind != "narration" {
		t.Errorf("expected segments to reach the DTO, got %+v", turns[0].Segments)
	}

	involved, err := svc.GetEntityTurns(context.Background(), gameID, "captain-kaelen")
	if err != nil {
		t.Fatalf("GetEntityTurns failed: %v", err)
	}
	if len(involved) != 1 || involved[0].TurnNumber != 1 {
		t.Fatalf("expected turn 1 for captain-kaelen, got %+v", involved)
	}

	uninvolved, err := svc.GetEntityTurns(context.Background(), gameID, "nobody-at-all")
	if err != nil {
		t.Fatalf("GetEntityTurns failed: %v", err)
	}
	if len(uninvolved) != 0 {
		t.Errorf("expected no turns for an uninvolved entity, got %+v", uninvolved)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestGetLocationArtUsesTheBuiltinGeneratorAndCaches(t *testing.T) {
	gameID, svc := setupTestGame(t)

	path, contentType, err := svc.GetLocationArt(context.Background(), gameID, "aldon-harbour", false)
	if err != nil {
		t.Fatalf("GetLocationArt failed: %v", err)
	}
	if contentType != "image/svg+xml" {
		t.Errorf("contentType = %q, want image/svg+xml from the built-in generator", contentType)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected the art on disk: %v", err)
	}
	if !strings.Contains(strings.ToLower(string(data)), "<svg") {
		t.Errorf("expected SVG art")
	}

	// A second request for the same appearance is a cache hit at the same path.
	again, _, err := svc.GetLocationArt(context.Background(), gameID, "aldon-harbour", false)
	if err != nil {
		t.Fatalf("second GetLocationArt failed: %v", err)
	}
	if again != path {
		t.Errorf("expected a cache hit at %q, got %q", path, again)
	}

	// Appearance changes move the key, so the image changes with it.
	entityPath := filepath.Join(svc.GetResolver().GameDir(gameID), "entities", "aldon-harbour.md")
	updated := strings.Replace(string(mustRead(t, entityPath)), "type: location", "type: location\nappearance: burned and abandoned", 1)
	if err := os.WriteFile(entityPath, []byte(updated), 0644); err != nil {
		t.Fatal(err)
	}

	moved, _, err := svc.GetLocationArt(context.Background(), gameID, "aldon-harbour", false)
	if err != nil {
		t.Fatalf("GetLocationArt after a change failed: %v", err)
	}
	if moved == path {
		t.Errorf("expected a new image after the appearance changed")
	}
}

func writeSegmentTurn(t *testing.T, svc *Service, gameID string) {
	t.Helper()

	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Say","input":"Where is the ledger?","narration":"He does not look up.","segments":[{"kind":"narration","text":"He does not look up."},{"kind":"speech","speaker":"Captain Kaelen","speaker_id":"captain-kaelen","text":"Keep walking."}]}` + "\n"
	path := filepath.Join(svc.GetResolver().GameDir(gameID), "history.jsonl")
	if err := os.WriteFile(path, []byte(record), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestGetSegmentClipsSynthesizesAndCaches(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)

	first, err := svc.GetSegmentClips(context.Background(), gameID, 1, 1)
	if err != nil {
		t.Fatalf("GetSegmentClips failed: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("clips = %d, want one for the single-sentence line", len(first))
	}
	if _, err := os.Stat(first[0]); err != nil {
		t.Fatalf("expected a clip on disk: %v", err)
	}

	again, err := svc.GetSegmentClips(context.Background(), gameID, 1, 1)
	if err != nil {
		t.Fatalf("second GetSegmentClips failed: %v", err)
	}
	if len(again) != 1 || again[0] != first[0] {
		t.Errorf("expected the cached clip %q, got %#v", first[0], again)
	}

	if _, err := svc.GetSegmentClips(context.Background(), gameID, 1, 9); err == nil {
		t.Errorf("expected an error for an out-of-range segment")
	}
	if _, err := svc.GetSegmentClips(context.Background(), gameID, 42, 0); err == nil {
		t.Errorf("expected an error for an unknown turn")
	}
}

func TestGetSegmentClipsWithoutTTSIsUnavailable(t *testing.T) {
	// A service whose config never enabled TTS.
	quiet := NewService(t.TempDir())
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)

	if err := os.MkdirAll(quiet.GetResolver().GameDir(gameID), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(quiet.GetResolver().GameDir(gameID), "history.jsonl"), mustRead(t, filepath.Join(svc.GetResolver().GameDir(gameID), "history.jsonl")), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := quiet.GetSegmentClips(context.Background(), gameID, 1, 1); !errors.Is(err, ErrAudioUnavailable) {
		t.Errorf("expected ErrAudioUnavailable, got %v", err)
	}
}

func TestChronicleOffersAudioURLsWhenTTSIsConfigured(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)

	turns, err := svc.GetChronicle(context.Background(), gameID)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || len(turns[0].Segments) != 2 {
		t.Fatalf("unexpected chronicle: %+v", turns)
	}
	if len(turns[0].Segments[0].AudioURLs) == 0 || len(turns[0].Segments[1].AudioURLs) == 0 {
		t.Errorf("expected clip URLs on both segments, got %+v", turns[0].Segments)
	}
}

func TestChronicleReportsSegmentDurations(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)

	turns, err := svc.GetChronicle(context.Background(), gameID)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || len(turns[0].Segments) != 2 {
		t.Fatalf("unexpected chronicle: %+v", turns)
	}

	// The reported estimate is the one the exports pace with, so the app holds a
	// line for the same length of time a rendered bundle does.
	for _, segment := range turns[0].Segments {
		if segment.Duration < scene.MinimumBeatDuration.Seconds() {
			t.Errorf("segment %q duration = %v, want at least the floor", segment.Text, segment.Duration)
		}
	}
}

func TestChronicleTurnsCarryLocationAndPacing(t *testing.T) {
	gameID, svc := setupTestGame(t)

	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Do","input":"I look around","narration":"The harbour is quiet.","location":"aldon-harbour","outcome":"clean_look","segments":[{"kind":"narration","text":"The harbour is quiet."}],"entities":[{"id":"player-elena","mention":"player"},{"id":"aldon-harbour","mention":"location"}]}` + "\n"
	if err := os.WriteFile(filepath.Join(svc.GetResolver().GameDir(gameID), "history.jsonl"), []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	turns, err := svc.GetChronicle(context.Background(), gameID)
	if err != nil {
		t.Fatalf("GetChronicle failed: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}

	turn := turns[0]
	if turn.LocationID != "aldon-harbour" || turn.LocationName != "Aldon Harbour" {
		t.Errorf("expected the location resolved, got %q / %q", turn.LocationID, turn.LocationName)
	}
	if turn.Outcome != "clean_look" {
		t.Errorf("Outcome = %q", turn.Outcome)
	}
	if turn.LocationArtURL == "" {
		t.Errorf("expected an art URL when the built-in generator is available")
	}
	if len(turn.Segments) != 1 || turn.Segments[0].Duration < scene.MinimumBeatDuration.Seconds() {
		t.Errorf("expected a paced segment, got %+v", turn.Segments)
	}
}

func TestTestProviderTTSReturnsPlayableAudio(t *testing.T) {
	_, svc := setupTestGame(t)

	res, err := svc.TestProvider(context.Background(), TestProviderRequestDTO{
		Category: "tts",
		Provider: config.TTSConfig{Type: "builtin", BuiltinName: "echo", DefaultVoice: "narrator"},
	})
	if err != nil {
		t.Fatalf("TestProvider failed: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected a successful TTS probe, got %q", res.Message)
	}
	if !strings.HasPrefix(res.AudioDataURI, "data:audio/wav;base64,") {
		t.Fatalf("expected an inline wav data URI for playback, got %q", res.AudioDataURI)
	}
}

func TestTestProviderSherpaTTSMissingModelReturnsStructuredMissing(t *testing.T) {
	svc := NewService(t.TempDir())
	res, err := svc.TestProvider(context.Background(), TestProviderRequestDTO{
		Category: "tts",
		Provider: config.TTSConfig{
			Type:        "builtin",
			BuiltinName: "sherpa-onnx",
		},
		TestPrompt: "Testing speech",
	})
	if err != nil {
		t.Fatalf("TestProvider failed: %v", err)
	}
	if res.Success {
		t.Errorf("expected failure for uninstalled kokoro model")
	}
	if !res.ModelMissing {
		t.Errorf("expected ModelMissing to be true")
	}
	if res.ModelID != "kokoro-tts" {
		t.Errorf("expected ModelID 'kokoro-tts', got %q", res.ModelID)
	}
}

func TestGetGameStateFindsALegacyDisplayNamePlayer(t *testing.T) {
	gameID, svc := setupTestGame(t)

	// Rewrite the manifest the way a pre-player_name build wrote it: the display
	// name in player:, the note still named by its slug.
	manifestPath := filepath.Join(svc.GetResolver().GameDir(gameID), "game.yaml")
	legacy := "id: test-campaign\nname: Test Campaign\nsystem: core-d20\nworld: shadow-realm\nplayer: Elena Nightshade\n"
	if err := os.WriteFile(manifestPath, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	state, err := svc.GetGameState(context.Background(), gameID)
	if err != nil {
		t.Fatalf("GetGameState failed for a legacy manifest: %v", err)
	}
	if state.Player.Name != "Elena Nightshade" {
		t.Errorf("Player.Name = %q, want Elena Nightshade", state.Player.Name)
	}
}

func TestCampaignTitleIsPersistedAndLatestIsFirst(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)

	sysDir := svc.GetResolver().SystemDir("freeform")
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte("id: freeform\nname: Freeform\nversion: 1.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	worldDir := svc.GetResolver().WorldDir("harbour-realm")
	if err := os.MkdirAll(worldDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte("id: harbour-realm\nname: Harbour Realm\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.CreateGame(context.Background(), CreateGameRequestDTO{
		Name:          "The Salt Road",
		SystemID:      "freeform",
		WorldID:       "harbour-realm",
		PlayerName:    "Elena Nightshade",
		OpeningPrompt: "Begin at dusk on the salt road.",
	}); err != nil {
		t.Fatalf("CreateGame failed: %v", err)
	}

	games, err := svc.ListGames(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 1 {
		t.Fatalf("expected 1 campaign, got %d", len(games))
	}
	if games[0].Name != "The Salt Road" {
		t.Errorf("Name = %q, want the entered title", games[0].Name)
	}
	if games[0].PlayerName != "Elena Nightshade" {
		t.Errorf("PlayerName = %q, want the display name", games[0].PlayerName)
	}

	state, err := svc.GetGameState(context.Background(), games[0].ID)
	if err != nil {
		t.Fatalf("GetGameState failed: %v", err)
	}
	if state.OpeningPrompt != "Begin at dusk on the salt road." {
		t.Errorf("OpeningPrompt = %q, want the prompt captured at creation", state.OpeningPrompt)
	}
}

func TestListGamesOrdersByLatestHistoryModTime(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)

	sysDir := svc.GetResolver().SystemDir("freeform")
	_ = os.MkdirAll(sysDir, 0755)
	_ = os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte("id: freeform\nname: Freeform\nversion: 1.0\n"), 0644)
	worldDir := svc.GetResolver().WorldDir("harbour-realm")
	_ = os.MkdirAll(worldDir, 0755)
	_ = os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte("id: harbour-realm\nname: Harbour Realm\n"), 0644)

	g1, err := svc.CreateGame(context.Background(), CreateGameRequestDTO{
		Name:       "Older Campaign",
		SystemID:   "freeform",
		WorldID:    "harbour-realm",
		PlayerName: "Hero 1",
	})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(10 * time.Millisecond)

	g2, err := svc.CreateGame(context.Background(), CreateGameRequestDTO{
		Name:       "Newer Campaign",
		SystemID:   "freeform",
		WorldID:    "harbour-realm",
		PlayerName: "Hero 2",
	})
	if err != nil {
		t.Fatal(err)
	}

	list, err := svc.ListGames(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != g2.ID {
		t.Fatalf("expected g2 to be first initially, got %v", list)
	}

	historyPath := filepath.Join(svc.GetResolver().GameDir(g1.ID), "history.jsonl")
	_ = os.WriteFile(historyPath, []byte(`{"turn_number":1,"input_text":"look","mode":"Action","prose":"You look."}`+"\n"), 0644)
	futureTime := time.Now().Add(1 * time.Hour)
	_ = os.Chtimes(historyPath, futureTime, futureTime)

	listAfterHistory, err := svc.ListGames(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if listAfterHistory[0].ID != g1.ID {
		t.Errorf("expected g1 with updated history.jsonl to be first, got %s", listAfterHistory[0].ID)
	}
}

func setupFreeformSystem(t *testing.T, svc *Service) {
	t.Helper()

	sysDir := svc.GetResolver().SystemDir("freeform")
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte("id: freeform\nname: Freeform\nversion: 1.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	worldDir := svc.GetResolver().WorldDir("harbour-realm")
	if err := os.MkdirAll(worldDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte("id: harbour-realm\nname: Harbour Realm\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestCreateGamePersistsPlayerCharacter(t *testing.T) {
	svc := NewService(t.TempDir())
	setupFreeformSystem(t, svc)

	game, err := svc.CreateGame(context.Background(), CreateGameRequestDTO{
		Name:       "The Salt Road",
		SystemID:   "freeform",
		WorldID:    "harbour-realm",
		PlayerName: "Elena Nightshade",
		Player: PlayerCharacterDTO{
			Appearance: "Tall, salt-bitten, grey eyes.",
			Age:        "34",
			Background: "A smuggler turned cartographer.",
			Voice:      &config.VoiceProfile{ID: "af_bella", VoiceID: "af_bella", Pitch: 1, SpeechRate: 1},
		},
	})
	if err != nil {
		t.Fatalf("CreateGame failed: %v", err)
	}

	ent, err := svc.GetEntity(context.Background(), game.ID, "elena-nightshade")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}
	for _, want := range []string{"Tall, salt-bitten, grey eyes.", "af_bella", "A smuggler turned cartographer."} {
		if !strings.Contains(ent.Markdown, want) {
			t.Errorf("player note is missing %q:\n%s", want, ent.Markdown)
		}
	}
}

func TestCreateGameRequiresCharacterAppearance(t *testing.T) {
	svc := NewService(t.TempDir())
	setupFreeformSystem(t, svc)

	_, err := svc.CreateGame(context.Background(), CreateGameRequestDTO{
		Name:       "The Salt Road",
		SystemID:   "freeform",
		WorldID:    "harbour-realm",
		PlayerName: "Elena Nightshade",
		Player:     PlayerCharacterDTO{Background: "A smuggler."},
	})
	if err == nil || !strings.Contains(err.Error(), "appearance") {
		t.Fatalf("expected appearance to be required, got %v", err)
	}
}

func TestCreateGamePersistsNarratorVoice(t *testing.T) {
	svc := NewService(t.TempDir())
	setupFreeformSystem(t, svc)

	game, err := svc.CreateGame(context.Background(), CreateGameRequestDTO{
		Name:          "Narrator Campaign",
		SystemID:      "freeform",
		WorldID:       "harbour-realm",
		PlayerName:    "Hero",
		NarratorVoice: "custom_narrator_voice",
		Player: PlayerCharacterDTO{
			Appearance: "Tall and dark",
		},
	})
	if err != nil {
		t.Fatalf("CreateGame: %v", err)
	}

	manifest, err := core.LoadGameManifest(filepath.Join(svc.resolver.GameDir(game.ID), "game.yaml"))
	if err != nil {
		t.Fatalf("LoadGameManifest: %v", err)
	}
	if manifest.Settings["narrator_voice"] != "custom_narrator_voice" {
		t.Errorf("expected narrator_voice 'custom_narrator_voice', got %v", manifest.Settings["narrator_voice"])
	}
}

func TestResolveWikilinksPointsAtEntityIDs(t *testing.T) {
	resolve := func(name string) string {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "guard kael", "guard-kael":
			return "guard-kael"
		default:
			return ""
		}
	}

	got := resolveWikilinks(`[[Guard Kael]] says: [[guard-kael|the warden]] sees [[Nobody]].`, resolve)
	want := `[[guard-kael|Guard Kael]] says: [[guard-kael|the warden]] sees Nobody.`
	if got != want {
		t.Errorf("resolveWikilinks() = %q, want %q", got, want)
	}
}

func TestGameStateSurfacesTheLivingWorld(t *testing.T) {
	gameID, svc := setupTestGame(t)

	arc := "---\nid: the-creeping-miasma\nname: The Creeping Miasma\ntype: arc\nstate:\n  clock_ticks: 2\n  clock_max: 6\n---\nA creeping fog."
	if err := svc.SaveEntity(context.Background(), gameID, "the-creeping-miasma", arc); err != nil {
		t.Fatal(err)
	}

	state, err := svc.GetGameState(context.Background(), gameID)
	if err != nil {
		t.Fatalf("GetGameState failed: %v", err)
	}

	foundLocation := false
	for _, location := range state.Locations {
		if location == "Aldon Harbour" {
			foundLocation = true
		}
	}
	if !foundLocation {
		t.Errorf("expected the campaign's location in state, got %v", state.Locations)
	}

	if len(state.Arcs) != 1 {
		t.Fatalf("expected one arc, got %+v", state.Arcs)
	}
	if state.Arcs[0].Progress != 2 || state.Arcs[0].MaxProgress != 6 {
		t.Errorf("arc clock = %d/%d, want 2/6", state.Arcs[0].Progress, state.Arcs[0].MaxProgress)
	}
}

func TestArcProgressReadsBothConventions(t *testing.T) {
	ticks, maxTicks := arcProgress(map[string]interface{}{"progress": "3/6"})
	if ticks != 3 || maxTicks != 6 {
		t.Errorf(`progress "3/6" = %d/%d, want 3/6`, ticks, maxTicks)
	}

	// YAML and JSON disagree on numeric types, so both must coerce.
	ticks, maxTicks = arcProgress(map[string]interface{}{"clock_ticks": float64(4), "clock_max": 8})
	if ticks != 4 || maxTicks != 8 {
		t.Errorf("clock_ticks/clock_max = %d/%d, want 4/8", ticks, maxTicks)
	}

	// A missing clock must not produce a zero maximum, which would divide by zero.
	if _, maxTicks := arcProgress(map[string]interface{}{"clock_ticks": 1}); maxTicks < 1 {
		t.Errorf("maxTicks = %d, want at least 1", maxTicks)
	}
}

func TestPrepareTurnAppliesTheRecallLimits(t *testing.T) {
	root := t.TempDir()
	configYAML := "agents:\n  roles:\n    gm:\n      type: builtin\n      builtin_name: echo\n  scene_recall_turns: 9\n  retrieval_halflife_turns: 30\n"
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewService(root)
	paths := svc.GetResolver()
	sysDir := paths.SystemDir("freeform")
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte("id: freeform\nname: Freeform\nversion: 1.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	worldDir := paths.WorldDir("harbour-realm")
	if err := os.MkdirAll(worldDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte("id: harbour-realm\nname: Harbour Realm\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.InitGame(paths, engine.InitOptions{
		GameID: "campaign-01", SystemID: "freeform", WorldID: "harbour-realm", PlayerName: "Sean",
	}); err != nil {
		t.Fatal(err)
	}

	session, err := svc.BeginTurn("campaign-01")
	if err != nil {
		t.Fatalf("BeginTurn failed: %v", err)
	}
	defer session.Close()

	limits := session.ContextLimits()
	if limits.SceneRecallTurns != 9 {
		t.Errorf("SceneRecallTurns = %d, want the configured 9", limits.SceneRecallTurns)
	}
	if limits.RetrievalHalflife != 30 {
		t.Errorf("RetrievalHalflife = %d, want the configured 30", limits.RetrievalHalflife)
	}
	// Unset keys must still carry their defaults rather than zero.
	if limits.RetrievalTurns != 3 {
		t.Errorf("RetrievalTurns = %d, want the default 3", limits.RetrievalTurns)
	}
}

func TestAServiceRegeneratesTheSummaryBehindTheTurn(t *testing.T) {
	root := t.TempDir()
	// A cadence of one, so the very first turn makes a regeneration due.
	configYAML := "agents:\n  roles:\n    gm:\n      type: builtin\n      builtin_name: echo\n  summary_every: 1\n"
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewService(root)
	paths := svc.GetResolver()
	sysDir := paths.SystemDir("freeform")
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte("id: freeform\nname: Freeform\nversion: 1.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	worldDir := paths.WorldDir("harbour-realm")
	if err := os.MkdirAll(worldDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte("id: harbour-realm\nname: Harbour Realm\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.InitGame(paths, engine.InitOptions{
		GameID: "campaign-01", SystemID: "freeform", WorldID: "harbour-realm", PlayerName: "Sean",
	}); err != nil {
		t.Fatal(err)
	}

	session, err := svc.BeginTurn("campaign-01")
	if err != nil {
		t.Fatalf("BeginTurn failed: %v", err)
	}
	if err := session.Run(context.Background(), TurnRequest{Mode: "Do", Input: "I look around"}, func(TurnEvent) error {
		return nil
	}); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	session.Close()

	// The regeneration is detached, so the test waits for it rather than assuming
	// it finished before Run returned.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !svc.SummaryPending("campaign-01") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	recap, err := svc.GetRecap(context.Background(), "campaign-01")
	if err != nil {
		t.Fatalf("GetRecap failed: %v", err)
	}
	if !recap.Enabled {
		t.Errorf("expected summaries to be reported as enabled")
	}
	if recap.ThroughTurn != 1 {
		t.Errorf("ThroughTurn = %d, want 1: the summary must cover the turn that triggered it", recap.ThroughTurn)
	}
	if strings.TrimSpace(recap.Summary) == "" {
		t.Errorf("expected a summary")
	}
}

func TestRecapIsDisabledWhenSummariesAreOff(t *testing.T) {
	root := t.TempDir()
	// An omitted key inherits the shipped default of ten turns, so switching
	// summaries off is explicit.
	configYAML := "agents:\n  roles:\n    gm:\n      type: builtin\n      builtin_name: echo\n  summary_every: 0\n"
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewService(root)
	paths := svc.GetResolver()
	sysDir := paths.SystemDir("freeform")
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte("id: freeform\nname: Freeform\nversion: 1.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	worldDir := paths.WorldDir("harbour-realm")
	if err := os.MkdirAll(worldDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte("id: harbour-realm\nname: Harbour Realm\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.InitGame(paths, engine.InitOptions{
		GameID: "campaign-01", SystemID: "freeform", WorldID: "harbour-realm", PlayerName: "Sean",
	}); err != nil {
		t.Fatal(err)
	}

	recap, err := svc.GetRecap(context.Background(), "campaign-01")
	if err != nil {
		t.Fatalf("GetRecap failed: %v", err)
	}
	if recap.Enabled {
		t.Errorf("expected summaries to be off when the cadence is zero")
	}
	if recap.Summary != "" {
		t.Errorf("expected no summary, got %q", recap.Summary)
	}
}

func TestMergeFoldsANoteIntoAnother(t *testing.T) {
	gameID, svc := setupTestGame(t)

	// A duplicate: the same being under the name the model invented for him.
	if err := svc.SaveEntity(context.Background(), gameID, "the-ember-warden",
		"---\nid: the-ember-warden\nname: The Ember Warden\ntype: character\ntags: [warden]\naliases: [Kael]\n---\nStands vigil by the brazier.\n"); err != nil {
		t.Fatal(err)
	}

	// Another note links to the duplicate, so the merge has a link to rewrite.
	captain := "---\nid: captain-kaelen\nname: Captain Kaelen\ntype: npc\n---\nReports to [[the-ember-warden]] each dawn.\n"
	if err := svc.SaveEntity(context.Background(), gameID, "captain-kaelen", captain); err != nil {
		t.Fatal(err)
	}

	merged, err := svc.MergeEntities(context.Background(), gameID, "the-ember-warden", "captain-kaelen")
	if err != nil {
		t.Fatalf("MergeEntities failed: %v", err)
	}
	if merged.ID != "captain-kaelen" {
		t.Errorf("merged into %q, want captain-kaelen", merged.ID)
	}
	if !strings.Contains(merged.Markdown, "Stands vigil by the brazier.") {
		t.Errorf("expected the source body folded in:\n%s", merged.Markdown)
	}
	if !strings.Contains(merged.Markdown, "Kael") {
		t.Errorf("expected the source aliases folded in:\n%s", merged.Markdown)
	}

	// The source note is gone, from disk and from the index.
	if _, err := svc.GetEntity(context.Background(), gameID, "the-ember-warden"); err == nil {
		t.Errorf("expected the source note to be removed")
	}

	// Inbound links now point at the survivor rather than at a note that is gone.
	note, err := svc.GetEntity(context.Background(), gameID, "captain-kaelen")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(note.Markdown, "[[the-ember-warden]]") {
		t.Errorf("expected the inbound link rewritten:\n%s", note.Markdown)
	}
}

func TestAddressingAFindingSurvivesAReload(t *testing.T) {
	gameID, svc := setupTestGame(t)

	before, err := svc.Findings(gameID)
	if err != nil {
		t.Fatalf("Findings failed: %v", err)
	}
	if len(before.Addressed) != 0 {
		t.Fatalf("expected nothing addressed, got %+v", before)
	}

	if err := svc.AddressFinding(gameID, 3, "unknown-entity"); err != nil {
		t.Fatalf("AddressFinding failed: %v", err)
	}
	// Addressing the same finding twice is not an error and not a duplicate.
	if err := svc.AddressFinding(gameID, 3, "unknown-entity"); err != nil {
		t.Fatal(err)
	}

	after, err := svc.Findings(gameID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Addressed) != 1 {
		t.Fatalf("expected one addressed finding, got %+v", after.Addressed)
	}
	if after.Addressed[0].Turn != 3 || after.Addressed[0].Rule != "unknown-entity" {
		t.Errorf("unexpected record: %+v", after.Addressed[0])
	}

	// The sidecar is how this survives, because the timeline is append-only.
	path := filepath.Join(svc.GetResolver().GameDir(gameID), "findings.json")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected a sidecar on disk: %v", err)
	}
}

func TestRecapCarriesTheOpenThreads(t *testing.T) {
	gameID, svc := setupTestGame(t)

	if err := svc.SaveEntity(context.Background(), gameID, "the-miasma",
		"---\nid: the-miasma\nname: The Creeping Miasma\ntype: arc\n---\nA creeping fog.\n"); err != nil {
		t.Fatal(err)
	}

	recap, err := svc.GetRecap(context.Background(), gameID)
	if err != nil {
		t.Fatalf("GetRecap failed: %v", err)
	}
	if len(recap.Threads) != 1 {
		t.Fatalf("expected one open thread, got %+v", recap.Threads)
	}
	if recap.Threads[0].ID != "the-miasma" || recap.Threads[0].Status != "open" {
		t.Errorf("unexpected thread: %+v", recap.Threads[0])
	}
}

func TestResolvedThreadsAreNotOpen(t *testing.T) {
	gameID, svc := setupTestGame(t)

	if err := svc.SaveEntity(context.Background(), gameID, "the-siege",
		"---\nid: the-siege\nname: The Iron Siege\ntype: arc\nstate:\n  status: resolved\n---\nThe siege broke.\n"); err != nil {
		t.Fatal(err)
	}

	recap, err := svc.GetRecap(context.Background(), gameID)
	if err != nil {
		t.Fatal(err)
	}
	if len(recap.Threads) != 0 {
		t.Errorf("expected no open threads, got %+v", recap.Threads)
	}
}

func TestSegmentClipURLsFollowVoiceOptions(t *testing.T) {
	stability := 0.35
	voiceFor := func(string) *entity.VoiceConfig {
		return &entity.VoiceConfig{VoiceID: "af_bella", Options: map[string]interface{}{"stability": stability}}
	}

	segments := []entity.TurnSegment{{Kind: entity.SegmentSpeech, SpeakerID: "aldric", Text: "Hello there."}}
	before := segmentDTOs(segments, "campaign", clipPlanFromVoice(t, voiceFor, segments), func(name string) string { return name })

	stability = 0.8
	after := segmentDTOs(segments, "campaign", clipPlanFromVoice(t, voiceFor, segments), func(name string) string { return name })

	if len(before[0].AudioURLs) != 1 || len(after[0].AudioURLs) != 1 {
		t.Fatalf("audio_urls = %#v / %#v, want one clip each", before[0].AudioURLs, after[0].AudioURLs)
	}
	if before[0].AudioURLs[0] == after[0].AudioURLs[0] {
		t.Errorf("expected a changed option to change the clip URL")
	}
}

// mustStore opens a campaign's store for a test that needs the same wiring the
// service uses.
func mustStore(t *testing.T, svc *Service, gameID string) *storage.Store {
	t.Helper()
	store, err := svc.store(gameID)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return store
}

func TestTurnDTOOffersOrderedClipURLs(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)

	turn, err := svc.findTurn(gameID, 1)
	if err != nil {
		t.Fatal(err)
	}
	dto := svc.turnDTO(*turn, mustStore(t, svc, gameID), svc.configMgr.Get(), gameID)
	if len(dto.Segments) != 2 {
		t.Fatalf("segments = %d", len(dto.Segments))
	}
	if len(dto.Segments[1].AudioURLs) != 1 {
		t.Fatalf("audio_urls = %#v, want one URL for the line", dto.Segments[1].AudioURLs)
	}
	url := dto.Segments[1].AudioURLs[0]
	if !strings.HasPrefix(url, "/api/audio/clip/") {
		t.Errorf("url = %q, want a content-addressed clip URL", url)
	}
	if key := strings.TrimPrefix(url, "/api/audio/clip/"); media.ClipKeyForPath(key) != key {
		t.Errorf("url key %q is not a clip name", key)
	}
}

func TestGetTurnSceneImage(t *testing.T) {
	gameID, svc := setupTestGame(t)
	scenesDir := filepath.Join(svc.GetResolver().GameDir(gameID), "assets", "scenes")
	_ = os.MkdirAll(scenesDir, 0755)
	sceneFile := filepath.Join(scenesDir, "turn-5.png")
	pngBytes := []byte("\x89PNG\r\n\x1a\nfake png data")
	_ = os.WriteFile(sceneFile, pngBytes, 0644)

	data, contentType, err := svc.GetTurnSceneImage(context.Background(), gameID, 5)
	if err != nil {
		t.Fatalf("GetTurnSceneImage failed: %v", err)
	}
	if contentType != "image/png" {
		t.Errorf("expected contentType image/png, got %s", contentType)
	}
	if !bytes.Equal(data, pngBytes) {
		t.Errorf("data mismatch")
	}

	// Turn 99 (does not exist) returns error
	_, _, err = svc.GetTurnSceneImage(context.Background(), gameID, 99)
	if err == nil {
		t.Errorf("expected error for non-existent scene image, got nil")
	}
}

func TestGetCharacterPortraitVersionQuery(t *testing.T) {
	gameID, svc := setupTestGame(t)
	portraitsDir := filepath.Join(svc.GetResolver().GameDir(gameID), "assets", "portraits")
	_ = os.MkdirAll(portraitsDir, 0755)

	v1Bytes := []byte("\x89PNG\r\n\x1a\nportrait v1")
	v2Bytes := []byte("\x89PNG\r\n\x1a\nportrait v2")
	_ = os.WriteFile(filepath.Join(portraitsDir, "elena-v1.png"), v1Bytes, 0644)
	_ = os.WriteFile(filepath.Join(portraitsDir, "elena-v2.png"), v2Bytes, 0644)

	// Save entity note with v2 active
	ent := &entity.Entity{
		ID:              "elena",
		Name:            "Elena",
		Type:            "character",
		Portrait:        "assets/portraits/elena-v2.png",
		PortraitVersion: 2,
		PortraitHistory: []string{"assets/portraits/elena-v1.png"},
	}
	noteBytes, _ := ent.SerializeMarkdown()
	_ = os.WriteFile(filepath.Join(svc.GetResolver().GameDir(gameID), "entities", "elena.md"), noteBytes, 0644)

	// Requesting v=1 returns v1 bytes
	data1, _, err := svc.GetCharacterPortrait(context.Background(), gameID, "elena", 1)
	if err != nil {
		t.Fatalf("GetCharacterPortrait v=1 failed: %v", err)
	}
	if !bytes.Equal(data1, v1Bytes) {
		t.Errorf("expected v1 bytes, got %s", string(data1))
	}

	// Requesting without version returns active v2 bytes
	data2, _, err := svc.GetCharacterPortrait(context.Background(), gameID, "elena")
	if err != nil {
		t.Fatalf("GetCharacterPortrait default failed: %v", err)
	}
	if !bytes.Equal(data2, v2Bytes) {
		t.Errorf("expected v2 bytes, got %s", string(data2))
	}
}

func TestTurnDTO_SceneBreakAndAnchoredSpeakerPortraits(t *testing.T) {
	gameID, svc := setupTestGame(t)

	// Create a scene illustration for turn 3 on disk
	scenesDir := filepath.Join(svc.GetResolver().GameDir(gameID), "assets", "scenes")
	_ = os.MkdirAll(scenesDir, 0755)
	_ = os.WriteFile(filepath.Join(scenesDir, "turn-3.png"), []byte("pngdata"), 0644)

	turn := engine.Turn{
		Number:     3,
		Input:      "I rest.",
		Narration:  "Ten years pass.\n\n---\n\nThe world has changed.",
		SceneBreak: true,
		Segments: []entity.TurnSegment{
			{
				Kind:            "speech",
				Speaker:         "Vera",
				SpeakerID:       "vera",
				Text:            "We survived.",
				SpeakerPortrait: "/api/game/" + gameID + "/character/vera/portrait?v=2",
			},
		},
	}

	dto := svc.turnDTO(turn, nil, svc.Config(), gameID)

	if !dto.SceneBreak {
		t.Errorf("expected dto.SceneBreak to be true")
	}
	expectedSceneURL := fmt.Sprintf("/api/game/%s/turn/3/scene-image", gameID)
	if dto.ImageURL != expectedSceneURL {
		t.Errorf("dto.ImageURL = %q, want %q", dto.ImageURL, expectedSceneURL)
	}

	if len(dto.Segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(dto.Segments))
	}
	expectedPortrait := fmt.Sprintf("/api/game/%s/character/vera/portrait?v=2", gameID)
	if dto.Segments[0].SpeakerPortrait != expectedPortrait {
		t.Errorf("dto.Segments[0].SpeakerPortrait = %q, want %q", dto.Segments[0].SpeakerPortrait, expectedPortrait)
	}
	if dto.Segments[0].PortraitURL != expectedPortrait {
		t.Errorf("dto.Segments[0].PortraitURL = %q, want %q", dto.Segments[0].PortraitURL, expectedPortrait)
	}
}


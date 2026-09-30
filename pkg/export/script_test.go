package export

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/scene"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestCompileReplayScript(t *testing.T) {
	isolateConfig(t)

	tempDir := t.TempDir()
	gameDir := filepath.Join(tempDir, "games", "shadow-campaign")
	_ = os.MkdirAll(gameDir, 0755)

	manifestContent := `id: shadow-campaign
name: Shadow Realm
system: core-d20
world: dark-fantasy
player: elena
`
	_ = os.WriteFile(filepath.Join(gameDir, "game.yaml"), []byte(manifestContent), 0644)

	historyFile := filepath.Join(gameDir, "history.jsonl")
	logger := engine.NewHistoryLogger(historyFile)

	_ = logger.AppendTurn(engine.Turn{
		Number:    1,
		Timestamp: time.Now(),
		Mode:      "Do",
		Input:     "I step into the tavern.",
		Narration: "The tavern is warm and loud. Evelyn looks up from her book.",
	})

	_ = logger.AppendTurn(engine.Turn{
		Number:    2,
		Timestamp: time.Now(),
		Mode:      "Say",
		Input:     "Good evening, Evelyn.",
		Narration: "Evelyn: \"You made it back in one piece.\"",
	})

	compiler := NewScriptCompiler(tempDir)
	script, err := compiler.Compile(context.Background(), "shadow-campaign")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if script.GameID != "shadow-campaign" {
		t.Errorf("expected game ID shadow-campaign, got %s", script.GameID)
	}
	if script.GameName != "Shadow Realm" {
		t.Errorf("GameName = %q, want the manifest name", script.GameName)
	}

	// Neither turn recorded a location, so each opens its own uncarded scene.
	if len(script.Scenes) != 2 {
		t.Fatalf("expected 2 scenes, got %d: %+v", len(script.Scenes), script.Scenes)
	}

	beats := script.Beats()
	if len(beats) != 2 {
		t.Fatalf("expected 2 beats, got %d", len(beats))
	}
	if beats[0].TurnNumber != 1 || beats[0].Kind != scene.BeatNarration {
		t.Errorf("unexpected beat 0: %+v", beats[0])
	}

	// The second turn predates segments, so its prose is parsed instead: the
	// attributed line still arrives as a speech beat.
	speech := beats[1]
	if speech.Kind != scene.BeatSpeech || speech.Speaker != "Evelyn" {
		t.Errorf("expected one attributed speech beat for Evelyn, got %+v", speech)
	}
}

func TestCompileKeepsAttributedSegments(t *testing.T) {
	isolateConfig(t)

	root := t.TempDir()
	gameDir := filepath.Join(root, "games", "segmented")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"), []byte("id: segmented\nname: Segmented\n"), 0644); err != nil {
		t.Fatal(err)
	}

	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Say","input":"Where is the ledger?","narration":"He does not look up. Garrick: \"Keep walking.\"","segments":[{"kind":"speech","speaker":"Sean","speaker_id":"player","text":"Where is the ledger?"},{"kind":"narration","text":"He does not look up."},{"kind":"speech","speaker":"Garrick","speaker_id":"garrick","text":"Keep walking."}]}` + "\n"
	if err := os.WriteFile(filepath.Join(gameDir, "history.jsonl"), []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	script, err := NewScriptCompiler(root).Compile(context.Background(), "segmented")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	beats := script.Beats()
	if len(beats) != 3 {
		t.Fatalf("expected 3 beats, got %d: %+v", len(beats), beats)
	}
	if beats[0].SpeakerID != "player" || beats[2].SpeakerID != "garrick" {
		t.Errorf("expected speaker IDs to survive compilation, got %#v", beats)
	}
	if beats[1].Kind != scene.BeatNarration {
		t.Errorf("expected the middle segment to be narration, got %+v", beats[1])
	}
}

func TestCompileResolvesTheLocationNameFromTheIndex(t *testing.T) {
	isolateConfig(t)

	root := t.TempDir()
	gameDir := filepath.Join(root, "games", "located")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"), []byte("id: located\nname: Located\n"), 0644); err != nil {
		t.Fatal(err)
	}

	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Do","input":"look","narration":"Warm.","location":"alden-tavern","segments":[{"kind":"narration","text":"Warm."}]}` + "\n"
	if err := os.WriteFile(filepath.Join(gameDir, "history.jsonl"), []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	// Compilation reads the location from the index, which is where a synced note
	// ends up.
	store, err := storage.OpenGameStore(core.NewPathResolver(root), "located")
	if err != nil {
		t.Fatalf("open game store: %v", err)
	}
	if err := store.SaveEntity(&entity.Entity{ID: "alden-tavern", Name: "Alden Tavern", Type: "location", Hash: "h1"}); err != nil {
		t.Fatal(err)
	}

	script, err := NewScriptCompiler(root).Compile(context.Background(), "located")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if len(script.Scenes) != 1 {
		t.Fatalf("expected 1 scene, got %d", len(script.Scenes))
	}
	if script.Scenes[0].LocationName != "Alden Tavern" {
		t.Errorf("LocationName = %q, want the entity name", script.Scenes[0].LocationName)
	}

	beats := script.Beats()
	if len(beats) != 2 || beats[0].Kind != scene.BeatSceneCard {
		t.Fatalf("expected a scene card then the narration, got %+v", beats)
	}
	if beats[1].Text != "Warm." {
		t.Errorf("unexpected narration beat: %+v", beats[1])
	}
}

// A bundle shows faces, so compilation has to resolve them: the note's own
// portrait file when the campaign has one, and the procedural bust the app serves
// when it does not.
func TestCompileResolvesPortraits(t *testing.T) {
	isolateConfig(t)

	root := t.TempDir()
	gameDir := filepath.Join(root, "games", "portraits")
	if err := os.MkdirAll(filepath.Join(gameDir, "assets", "portraits"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"),
		[]byte("id: portraits\nname: Portraits\nsystem: freeform\nworld: harbour\nplayer: sean\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "assets", "portraits", "sean.png"), []byte("png-bytes"), 0644); err != nil {
		t.Fatal(err)
	}

	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Say","input":"hello","narration":"Sean speaks.","segments":[` +
		`{"kind":"speech","speaker":"Sean","speaker_id":"sean","player":true,"text":"Hello."},` +
		`{"kind":"speech","speaker":"Garrick","speaker_id":"garrick","text":"Welcome."}]}` + "\n"
	if err := os.WriteFile(filepath.Join(gameDir, "history.jsonl"), []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	store, err := storage.OpenGameStore(core.NewPathResolver(root), "portraits")
	if err != nil {
		t.Fatalf("open game store: %v", err)
	}
	for _, ent := range []*entity.Entity{
		{ID: "sean", Name: "Sean", Type: "character", Portrait: "assets/portraits/sean.png", Hash: "h1"},
		{ID: "garrick", Name: "Garrick", Type: "character", Hash: "h2"},
	} {
		if err := store.SaveEntity(ent); err != nil {
			t.Fatal(err)
		}
	}

	script, err := NewScriptCompiler(root).Compile(context.Background(), "portraits")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if filepath.Base(script.PlayerPortrait) != "sean.png" {
		t.Errorf("PlayerPortrait = %q, want the protagonist's own file", script.PlayerPortrait)
	}

	var portraits = map[string]string{}
	for _, beat := range script.Beats() {
		if beat.Kind == scene.BeatSpeech && beat.PortraitPath != "" {
			portraits[beat.SpeakerID] = beat.PortraitPath
		}
	}
	if filepath.Base(portraits["sean"]) != "sean.png" {
		t.Errorf("sean's portrait = %q, want the note's file", portraits["sean"])
	}
	if _, err := os.Stat(portraits["garrick"]); err != nil {
		t.Errorf("garrick's portrait %q is not a file: %v", portraits["garrick"], err)
	}
	if !strings.HasSuffix(portraits["garrick"], ".svg") {
		t.Errorf("garrick's portrait = %q, want the procedural bust", portraits["garrick"])
	}
}

// A campaign names its own narrator voice, and the clips on disk are keyed by it. An
// export that narrates with the configuration's default instead is a cache miss for
// every line, which is what made an export silent while the app played instantly.
func TestCompileUsesTheCampaignsNarratorVoice(t *testing.T) {
	isolateConfig(t)

	root := t.TempDir()
	gameDir := filepath.Join(root, "games", "voiced")
	if err := os.MkdirAll(filepath.Join(gameDir, "entities"), 0755); err != nil {
		t.Fatal(err)
	}
	manifest := "id: voiced\nname: Voiced\nsystem: freeform\nworld: harbour\nplayer: sean\nsettings:\n  narrator_voice: am_adam\n"
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Do","input":"look","narration":"The quay is quiet.","segments":[{"kind":"narration","text":"The quay is quiet."}]}` + "\n"
	if err := os.WriteFile(filepath.Join(gameDir, "history.jsonl"), []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	// The clip the app would have cached, under the campaign's voice: a real Opus clip,
	// because the cache holds one format and checks it.
	voice := &entity.VoiceConfig{VoiceID: "am_adam"}
	cache := media.NewContentCache(core.NewPathResolver(root).CacheDir())
	if _, err := media.NewTTSPipeline(&toneTTS{}, cache).
		SynthesizeUtterance(context.Background(), "narrator", voice, "The quay is quiet."); err != nil {
		t.Fatal(err)
	}

	// A pipeline whose provider cannot synthesize: only a cache hit can resolve a clip.
	compiler := NewScriptCompiler(root)
	compiler.SetSpeechResolver(NewSpeechResolver(
		media.NewTTSPipeline(deadTTS{}, cache), nil, voice))

	script, err := compiler.Compile(context.Background(), "voiced")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	var clips int
	for _, beat := range script.Beats() {
		clips += len(beat.AudioPaths)
	}
	if clips != 1 {
		t.Fatalf("clips = %d, want the cached narration clip reused", clips)
	}
}

// toneTTS makes a real clip, so a test can seed a cache the way the app does.
type toneTTS struct{}

func (toneTTS) Synthesize(context.Context, string, *entity.VoiceConfig) ([]byte, error) {
	return media.GenerateToneWAV(440, 0.02), nil
}

// deadTTS cannot synthesize anything, so only a cache hit resolves a clip.
type deadTTS struct{}

func (deadTTS) Synthesize(context.Context, string, *entity.VoiceConfig) ([]byte, error) {
	return nil, errors.New("no provider available")
}

// A bundle carries the campaign's own image, which is what the theatre shows behind a
// scene that has no art of its own.
func TestCompileResolvesTheCampaignBanner(t *testing.T) {
	isolateConfig(t)

	root := t.TempDir()
	gameDir := filepath.Join(root, "games", "bannered")
	if err := os.MkdirAll(filepath.Join(gameDir, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"), []byte("id: bannered\nname: Bannered\nworld: harbour\n"), 0644); err != nil {
		t.Fatal(err)
	}
	banner := filepath.Join(gameDir, "assets", "banner.png")
	if err := os.WriteFile(banner, []byte("png-bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Do","input":"look","narration":"Quiet.","segments":[{"kind":"narration","text":"Quiet."}]}` + "\n"
	if err := os.WriteFile(filepath.Join(gameDir, "history.jsonl"), []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	script, err := NewScriptCompiler(root).Compile(context.Background(), "bannered")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if filepath.Base(script.Banner) != "banner.png" {
		t.Errorf("banner = %q, want the campaign's own image", script.Banner)
	}
}

// A bundle that is silent for one character looks the same as a bundle whose cache key
// does not match the app's, so the export reports the voice and keys it looked for.
func TestCompileReportsASpeechBeatWithNoClip(t *testing.T) {
	isolateConfig(t)

	root := t.TempDir()
	gameDir := filepath.Join(root, "games", "silent")
	if err := os.MkdirAll(filepath.Join(gameDir, "entities"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"),
		[]byte("id: silent\nname: Silent\nworld: harbour\nplayer: sean\n"), 0644); err != nil {
		t.Fatal(err)
	}
	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Say","input":"hello","narration":"A guard speaks.","segments":[{"kind":"speech","speaker":"Garrick","speaker_id":"garrick","text":"Keep moving."}]}` + "\n"
	if err := os.WriteFile(filepath.Join(gameDir, "history.jsonl"), []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	compiler := NewScriptCompiler(root)
	compiler.SetSpeechResolver(NewSpeechResolver(
		media.NewTTSPipeline(deadTTS{}, media.NewContentCache(core.NewPathResolver(root).CacheDir())),
		nil, &entity.VoiceConfig{VoiceID: "am_adam"}))

	if _, err := compiler.Compile(context.Background(), "silent"); err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	misses := compiler.SpeechMisses()
	if len(misses) != 1 {
		t.Fatalf("misses = %v, want the speech beat reported", misses)
	}
	for _, want := range []string{"Garrick", "garrick", "am_adam", "no provider available"} {
		if !strings.Contains(misses[0], want) {
			t.Errorf("miss %q does not mention %q", misses[0], want)
		}
	}
}

// A clip that reached the cache in another format is repaired on the way into an export,
// so a bundle never carries audio a browser will refuse, and the repair is reported.
func TestCompileRepairsAClipThatIsNotOpus(t *testing.T) {
	isolateConfig(t)

	root := t.TempDir()
	gameDir := filepath.Join(root, "games", "legacy")
	if err := os.MkdirAll(filepath.Join(gameDir, "entities"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"),
		[]byte("id: legacy\nname: Legacy\nworld: harbour\nplayer: sean\n"), 0644); err != nil {
		t.Fatal(err)
	}
	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Do","input":"look","narration":"The quay is quiet.","segments":[{"kind":"narration","text":"The quay is quiet."}]}` + "\n"
	if err := os.WriteFile(filepath.Join(gameDir, "history.jsonl"), []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	// A WAV clip cached under the key the narration resolves to, as a campaign cached
	// before the Opus migration left it.
	cache := media.NewContentCache(core.NewPathResolver(root).CacheDir())
	narrator := &entity.VoiceConfig{VoiceID: "am_adam"}
	key := media.ComputeAudioCacheKeyForVoice("narrator", narrator, "The quay is quiet.")
	if _, err := cache.Put("audio", key+".opus", media.GenerateToneWAV(440, 0.02)); err != nil {
		t.Fatal(err)
	}

	compiler := NewScriptCompiler(root)
	compiler.SetNarratorVoice(narrator)
	compiler.SetSpeechResolver(NewSpeechResolver(
		media.NewTTSPipeline(deadTTS{}, cache), nil, narrator))

	script, err := compiler.Compile(context.Background(), "legacy")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	var clips []string
	for _, beat := range script.Beats() {
		clips = append(clips, beat.AudioPaths...)
	}
	if len(clips) != 1 {
		t.Fatalf("clips = %v, want the repaired clip", clips)
	}

	data, err := os.ReadFile(clips[0])
	if err != nil {
		t.Fatal(err)
	}
	if !media.IsOpusClip(data) {
		t.Error("the exported clip is not Ogg/Opus")
	}

	repairs := compiler.SpeechRepairs()
	if len(repairs) != 1 || !strings.Contains(repairs[0], "re-encoded") {
		t.Errorf("repairs = %v, want the re-encoded clip reported", repairs)
	}
}

// A bundle carries no codex, so its prose reads with names rather than links: an authored
// label is kept, a bare target becomes the entity's name, and an unknown target stays as
// the author wrote it.
func TestCompileReadsProseWithNamesNotLinks(t *testing.T) {
	isolateConfig(t)

	root := t.TempDir()
	gameDir := filepath.Join(root, "games", "linked")
	if err := os.MkdirAll(filepath.Join(gameDir, "entities"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"),
		[]byte("id: linked\nname: Linked\nworld: harbour\nplayer: sean\n"), 0644); err != nil {
		t.Fatal(err)
	}
	note := "---\nid: the-quay\nname: The Quay\ntype: location\n---\nSalt air.\n"
	if err := os.WriteFile(filepath.Join(gameDir, "entities", "the-quay.md"), []byte(note), 0644); err != nil {
		t.Fatal(err)
	}
	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Do","input":"look","narration":"x","segments":[{"kind":"narration","text":"You reach [[the-quay]] and see [[the-quay|the harbour]] and [[nobody|a stranger]]."}]}` + "\n"
	if err := os.WriteFile(filepath.Join(gameDir, "history.jsonl"), []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	script, err := NewScriptCompiler(root).Compile(context.Background(), "linked")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	var text string
	for _, beat := range script.Beats() {
		if strings.Contains(beat.Text, "You reach") {
			text = beat.Text
		}
	}
	if strings.Contains(text, "[[") {
		t.Fatalf("the bundle still carries a link: %q", text)
	}
	for _, want := range []string{"The Quay", "the harbour", "a stranger"} {
		if !strings.Contains(text, want) {
			t.Errorf("text %q does not read %q", text, want)
		}
	}
}

// The protagonist's name travels with their portrait, so a player labels it as the theatre
// does.
func TestCompileCarriesTheProtagonistsName(t *testing.T) {
	isolateConfig(t)

	root := t.TempDir()
	gameDir := filepath.Join(root, "games", "named")
	if err := os.MkdirAll(filepath.Join(gameDir, "entities"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"),
		[]byte("id: named\nname: Named\nworld: harbour\nplayer: sean\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "entities", "sean.md"),
		[]byte("---\nid: sean\nname: Sean O'Malley\ntype: character\n---\nA traveller.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Do","input":"look","narration":"Quiet.","segments":[{"kind":"narration","text":"Quiet."}]}` + "\n"
	if err := os.WriteFile(filepath.Join(gameDir, "history.jsonl"), []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	script, err := NewScriptCompiler(root).Compile(context.Background(), "named")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if script.PlayerName != "Sean O'Malley" {
		t.Errorf("PlayerName = %q, want the protagonist's name", script.PlayerName)
	}
}

// A clip whose write was interrupted is refused by a browser, so an export must never carry
// one: it is replaced when a provider can, and left out (with a report) when it cannot.
func TestCompileNeverCarriesATruncatedClip(t *testing.T) {
	isolateConfig(t)

	root := t.TempDir()
	gameDir := filepath.Join(root, "games", "cut")
	if err := os.MkdirAll(filepath.Join(gameDir, "entities"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"),
		[]byte("id: cut\nname: Cut\nworld: harbour\nplayer: sean\n"), 0644); err != nil {
		t.Fatal(err)
	}
	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Do","input":"look","narration":"Quiet.","segments":[{"kind":"narration","text":"Quiet."}]}` + "\n"
	if err := os.WriteFile(filepath.Join(gameDir, "history.jsonl"), []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	// A whole clip, cut short, under the key the narration resolves to.
	cache := media.NewContentCache(core.NewPathResolver(root).CacheDir())
	narrator := &entity.VoiceConfig{VoiceID: "am_adam"}
	whole, err := media.NewTTSPipeline(&toneTTS{}, cache).
		SynthesizeUtterance(context.Background(), "narrator", narrator, "Quiet.")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(whole)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(whole, data[:len(data)*2/3], 0644); err != nil {
		t.Fatal(err)
	}

	// With no provider, the broken clip cannot be replaced, so it is left out rather than
	// shipped as audio a browser will refuse.
	compiler := NewScriptCompiler(root)
	compiler.SetNarratorVoice(narrator)
	compiler.SetSpeechResolver(NewSpeechResolver(media.NewTTSPipeline(deadTTS{}, cache), nil, narrator))

	script, err := compiler.Compile(context.Background(), "cut")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	for _, beat := range script.Beats() {
		for _, clip := range beat.AudioPaths {
			raw, err := os.ReadFile(clip)
			if err != nil {
				t.Fatal(err)
			}
			if !media.IsCompleteOpusStream(raw) {
				t.Errorf("the export carries a clip that is not whole: %s", clip)
			}
		}
	}

	if repairs := compiler.SpeechRepairs(); len(repairs) == 0 {
		t.Error("the broken clip was not reported")
	}
}

// A campaign cached by an older build holds clips that do not mark the end of their stream.
// An export re-encodes them, without a provider, so a bundle is playable in a browser that
// is strict about it.
func TestCompileRepairsAClipThatDoesNotEndProperly(t *testing.T) {
	isolateConfig(t)

	root := t.TempDir()
	gameDir := filepath.Join(root, "games", "older")
	if err := os.MkdirAll(filepath.Join(gameDir, "entities"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"),
		[]byte("id: older\nname: Older\nworld: harbour\nplayer: sean\n"), 0644); err != nil {
		t.Fatal(err)
	}
	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Do","input":"look","narration":"Quiet.","segments":[{"kind":"narration","text":"Quiet."}]}` + "\n"
	if err := os.WriteFile(filepath.Join(gameDir, "history.jsonl"), []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	cache := media.NewContentCache(core.NewPathResolver(root).CacheDir())
	narrator := &entity.VoiceConfig{VoiceID: "am_adam"}
	whole, err := media.NewTTSPipeline(&toneTTS{}, cache).
		SynthesizeUtterance(context.Background(), "narrator", narrator, "Quiet.")
	if err != nil {
		t.Fatal(err)
	}
	older, err := media.WithoutEndOfStreamMarker(whole)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(whole, older, 0644); err != nil {
		t.Fatal(err)
	}

	// No provider: only re-muxing the audio that is already cached can make it playable.
	compiler := NewScriptCompiler(root)
	compiler.SetNarratorVoice(narrator)
	compiler.SetSpeechResolver(NewSpeechResolver(media.NewTTSPipeline(deadTTS{}, cache), nil, narrator))

	script, err := compiler.Compile(context.Background(), "older")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	var clips []string
	for _, beat := range script.Beats() {
		clips = append(clips, beat.AudioPaths...)
	}
	if len(clips) != 1 {
		t.Fatalf("clips = %v, want the repaired clip", clips)
	}
	data, err := os.ReadFile(clips[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := media.ClipProblem(data); err != nil {
		t.Errorf("the exported clip is still not playable: %v", err)
	}
	if repairs := compiler.SpeechRepairs(); len(repairs) == 0 {
		t.Error("the re-encoded clip was not reported")
	}
}

// A beat that reduces to nothing is never spoken, so an export neither counts it nor reports
// it as a beat missing audio.
func TestCompileDoesNotReportUnspokenBeats(t *testing.T) {
	isolateConfig(t)

	root := t.TempDir()
	gameDir := filepath.Join(root, "games", "unspoken")
	if err := os.MkdirAll(filepath.Join(gameDir, "entities"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"),
		[]byte("id: unspoken\nname: Unspoken\nworld: harbour\nplayer: sean\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// A speech segment that is only a performance tag, with a provider that cannot act on
	// tags: it reduces to nothing, so it is never spoken.
	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Say","input":"hello","narration":"x","segments":[{"kind":"speech","speaker":"Garrick","speaker_id":"garrick","text":"[whispers]"}]}` + "\n"
	if err := os.WriteFile(filepath.Join(gameDir, "history.jsonl"), []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	cache := media.NewContentCache(core.NewPathResolver(root).CacheDir())
	narrator := &entity.VoiceConfig{VoiceID: "am_adam"}
	compiler := NewScriptCompiler(root)
	compiler.SetNarratorVoice(narrator)
	compiler.SetSpeechResolver(NewSpeechResolver(media.NewTTSPipeline(deadTTS{}, cache), nil, narrator))

	var messages []string
	compiler.SetProgress(func(p scene.Progress) {
		if p.Message != "" {
			messages = append(messages, p.Message)
		}
	})

	if _, err := compiler.Compile(context.Background(), "unspoken"); err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	for _, message := range messages {
		if strings.Contains(message, "no audio clip") {
			t.Errorf("an unspoken beat was reported as missing audio: %q", message)
		}
	}
	if misses := compiler.SpeechMisses(); len(misses) != 0 {
		t.Errorf("misses = %v, want an unspoken beat reported as nothing at all", misses)
	}
}

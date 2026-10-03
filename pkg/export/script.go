package export

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/media/opus"
	"github.com/darkliquid/localrpg/pkg/paths"
	"github.com/darkliquid/localrpg/pkg/scene"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// campaignSource supplies turns and locations from a campaign's files and index.
type campaignSource struct {
	resolver *core.PathResolver
	store    *storage.Store
	gameID   string
}

// Turns reads the campaign's history, filling in segments for turns recorded
// before segments existed by parsing their prose, so a legacy campaign exports
// with the same beat structure as a current one.
func (s *campaignSource) Turns() ([]engine.Turn, error) {
	path := filepath.Join(s.resolver.GameDir(s.gameID), "history.jsonl")
	turns, err := engine.NewHistoryLogger(path).LoadHistory()
	if err != nil {
		return nil, err
	}

	for i := range turns {
		if len(turns[i].Segments) == 0 {
			turns[i].Segments = media.LegacySegments(turns[i].Prose())
		}
	}
	return turns, nil
}

func (s *campaignSource) Location(id string) (*entity.Entity, error) {
	return s.store.GetEntity(id)
}

// speechResolver synthesizes one segment at a time and probes the clip's length,
// falling back to the reading estimate when probing fails. It also remembers every beat
// it could not resolve, because a bundle that is silent for one character is otherwise
// indistinguishable from one whose cache key does not match the app's.
type speechResolver struct {
	pipeline *media.TTSPipeline
	store    *storage.Store
	narrator *entity.VoiceConfig

	mu      sync.Mutex
	misses  []string
	repairs []string
}

// noteRepair records a clip that was not in the cache's format: one that was re-encoded, or
// one that could not be and was therefore left out.
func (r *speechResolver) noteRepair(note string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.repairs) < maxReportedMisses {
		r.repairs = append(r.repairs, note)
	}
}

// Repairs reports the clips that were not in the cache's format, so an export says what it
// had to fix rather than quietly carrying something a browser might refuse. It includes
// what the pipeline repaired while it was resolving the clips, and what this resolver
// found itself.
func (r *speechResolver) Repairs() []string {
	r.mu.Lock()
	own := append([]string(nil), r.repairs...)
	r.mu.Unlock()

	return append(r.pipeline.Repairs(), own...)
}

func (r *speechResolver) SegmentAudio(ctx context.Context, segment entity.TurnSegment) ([]string, time.Duration, error) {
	clips, err := r.pipeline.SynthesizeSegmentClips(ctx, segment, r.narrator, r.voiceFor, false)
	if errors.Is(err, media.ErrNoSpeakableText) {
		// The beat reduces to nothing - a stage direction, say - so it is never spoken. It
		// is not a beat that should have audio, and nothing is reported about it.
		return nil, 0, scene.ErrNoSpeakableText
	}
	if err != nil || len(clips) == 0 {
		r.noteMiss(segment, err)
	}
	if err != nil {
		return nil, 0, err
	}

	// Every clip is checked before it leaves: the cache's one format is Ogg/Opus, whole and
	// properly ended, and a file that reached it another way - a campaign cached before the
	// Opus migration, or one written before a stream marked its own end - is decoded and
	// re-encoded rather than shipped as audio a browser will refuse. The repaired file
	// replaces the original under its key.
	checked := make([]string, 0, len(clips))
	for _, clip := range clips {
		data, err := os.ReadFile(clip)
		if err != nil {
			r.noteRepair(fmt.Sprintf("%s could not be read and was left out: %v", filepath.Base(clip), err))
			continue
		}

		problem := media.ClipProblem(data)
		if problem == nil {
			checked = append(checked, clip)
			continue
		}

		fixed, err := r.pipeline.NormalizeClip(clip)
		if err != nil {
			r.noteRepair(fmt.Sprintf("%s was left out: %v (%v)", filepath.Base(clip), err, problem))
			continue
		}
		r.noteRepair(fmt.Sprintf("%s was re-encoded: %v", filepath.Base(clip), problem))
		checked = append(checked, fixed)
	}
	clips = checked

	// A clip whose length cannot be read contributes nothing rather than throwing
	// the beat's pacing away; the beat falls back to the reading estimate.
	var total time.Duration
	for _, clip := range clips {
		data, err := os.ReadFile(clip)
		if err != nil {
			continue
		}
		duration, err := opus.Duration(data)
		if err != nil {
			continue
		}
		total += duration
	}
	return clips, total, nil
}

func (r *speechResolver) voiceFor(speakerID string) *entity.VoiceConfig {
	return harness.ResolveSpeakerVoice(r.store, speakerID)
}

// noteMiss records what a beat that produced no clip was read as: the speaker, the voice
// it resolved to, the keys the pipeline looked for, and why nothing came back. Keys that
// are absent from the cache mean the app cached the clip under something else.
func (r *speechResolver) noteMiss(segment entity.TurnSegment, err error) {
	speaker := strings.TrimSpace(segment.Speaker)
	if speaker == "" {
		speaker = "narrator"
	}

	voice := r.narrator
	ref := ""
	if segment.Kind == entity.SegmentSpeech {
		ref = segment.SpeakerID
		if ref == "" {
			ref = segment.Speaker
		}
		if resolved := r.voiceFor(ref); resolved != nil {
			voice = resolved
		}
	}

	keys, _ := r.pipeline.SegmentClipKeys(segment, r.narrator, r.voiceFor)
	reason := "no clip was produced"
	if err != nil {
		reason = err.Error()
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.misses) < maxReportedMisses {
		r.misses = append(r.misses, fmt.Sprintf(
			"%s %q (ref %q) voice %q: %d keys %v: %s",
			segment.Kind, speaker, ref, voice.VoiceID, len(keys), keys, reason))
	}
}

// Misses reports the beats that resolved no clip, most recent first, so an export can say
// which lines it could not speak and what it looked for.
func (r *speechResolver) Misses() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.misses...)
}

// maxReportedMisses bounds the report: a campaign with no provider would otherwise list
// every line it has.
const maxReportedMisses = 5

// portraitResolver serves a character's portrait: the note's own file when it has
// one, otherwise the procedural bust the app serves for a character without art.
// A character with neither resolves to nothing, which is decoration lost rather
// than an export lost.
type portraitResolver struct {
	resolver *core.PathResolver
	store    *storage.Store
	cache    *media.ContentCache
	gameID   string
}

func (r *portraitResolver) Portrait(_ context.Context, characterID string) (string, error) {
	if strings.TrimSpace(characterID) == "" || r.store == nil {
		return "", scene.ErrAudioUnavailable
	}

	ent, err := r.store.GetEntity(characterID)
	if err != nil || ent == nil {
		return "", scene.ErrAudioUnavailable
	}

	if strings.TrimSpace(ent.Portrait) != "" {
		path := filepath.Join(r.resolver.GameDir(r.gameID), ent.Portrait)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	name := strings.TrimSpace(ent.Name)
	if name == "" {
		name = characterID
	}
	return r.cache.Put("export-portraits", characterID+".svg", media.GenerateProceduralBustSVG(ent.ID, name, ent.Gender))
}

// NewSpeechResolver builds a speech resolver over an existing pipeline, so an export
// reuses the clips the app plays: the same pipeline means the same text policy, the same
// cache, and therefore the same keys.
func NewSpeechResolver(pipeline *media.TTSPipeline, store *storage.Store, narrator *entity.VoiceConfig) scene.SpeechResolver {
	return &speechResolver{pipeline: pipeline, store: store, narrator: narrator}
}

// narratorVoice resolves the voice narration is read in: the campaign's own setting when
// it has one, otherwise the configuration's default. It is the voice the app narrates
// with, so the clips an export needs are the clips the app already cached.
func (c *ScriptCompiler) narratorVoice(manifest *core.GameManifest) *entity.VoiceConfig {
	if c.narrator != nil {
		return c.narrator
	}

	voiceID := ""
	if c.config != nil {
		voiceID = c.config.Media.TTS.DefaultVoice
	}
	if manifest != nil && manifest.Settings != nil {
		if nv, ok := manifest.Settings["narrator_voice"].(string); ok && strings.TrimSpace(nv) != "" {
			voiceID = strings.TrimSpace(nv)
		}
	}

	voice := &entity.VoiceConfig{VoiceID: voiceID}
	if c.config != nil {
		voice.Pitch = c.config.Media.TTS.Pitch
		voice.SpeechRate = c.config.Media.TTS.SpeechRate
		voice.Options = c.config.Media.TTS.Options
	}
	return voice
}

// bannerPath finds a campaign's own image: its banner when it has one, otherwise its
// world's, which is the fallback the app's launcher and theatre use.
func bannerPath(resolver *core.PathResolver, manifest *core.GameManifest) string {
	if path, _ := findBanner(resolver.GameDir(manifest.ID)); path != "" {
		return path
	}
	if manifest.WorldID != "" {
		if path, _ := findBanner(resolver.WorldDir(manifest.WorldID)); path != "" {
			return path
		}
	}
	return ""
}

// findBanner looks for a banner image in a directory's assets, in the order the app
// serves them.
func findBanner(dir string) (string, string) {
	for _, ext := range []string{".png", ".webp", ".jpg", ".jpeg", ".svg"} {
		path := filepath.Join(dir, "assets", "banner"+ext)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, ext
		}
	}
	return "", ""
}

// ScriptCompiler builds a scene script for a campaign.
type ScriptCompiler struct {
	rootDir  string
	resolver *core.PathResolver
	config   *config.Config
	art      bool
	audio    bool
	progress scene.ProgressFunc
	// art and speech are the app's own resolvers when a caller supplies them, so an
	// export reuses the images and clips the app already has rather than building its
	// own clients and missing the cache. narrator is the campaign's narrator voice,
	// which decides the key every narration clip is cached under. banner is the
	// campaign's own image, which the theatre shows when a scene has none.
	artResolver  scene.ArtResolver
	speech       scene.SpeechResolver
	speechReport interface {
		Misses() []string
		Repairs() []string
	}
	narrator *entity.VoiceConfig
	banner   string
}

// NewScriptCompiler builds a compiler for one campaign root. Imagery and speech
// are both on by default: an export is expected to look and sound like the
// campaign unless it is deliberately told otherwise.
func NewScriptCompiler(rootDir string) *ScriptCompiler {
	cfg, _ := config.NewConfigManager().Load()
	projectRoot := ""
	// "." is the flag's default and means "no project directory", exactly as the
	// app treats it. Only an explicit directory puts the process in project mode,
	// where relative paths resolve under it. Without this a plain `export` looks
	// for campaigns and clips in the working directory instead of the XDG
	// locations the app actually writes to, and every clip is a miss.
	if rootDir != "" && rootDir != "." {
		projectRoot = rootDir
	}
	dirs := paths.Resolve(paths.System(), cfg.Paths, projectRoot)
	return NewScriptCompilerWithResolver(
		core.NewCustomPathResolver(dirs.Systems, dirs.Worlds, dirs.Games, dirs.Cache), cfg)
}

// CacheDir is the clip cache an export reads from, so a silent export can say
// where it looked rather than only that it found nothing.
func (c *ScriptCompiler) CacheDir() string { return c.resolver.CacheDir() }

// NewScriptCompilerWithResolver builds a compiler against an already-resolved
// resolver and configuration, so a server exports from the same locations it
// serves rather than re-deriving them from the working directory.
func NewScriptCompilerWithResolver(resolver *core.PathResolver, cfg *config.Config) *ScriptCompiler {
	if cfg == nil {
		cfg, _ = config.NewConfigManager().Load()
	}
	if resolver == nil {
		resolver = core.NewPathResolver(".")
	}
	return &ScriptCompiler{resolver: resolver, config: cfg, art: true, audio: true}
}

// SetArtResolver supplies the app's own scene art, so an export serves the images the
// app already has. Without one the compiler builds its own client.
func (c *ScriptCompiler) SetArtResolver(art scene.ArtResolver) { c.artResolver = art }

// SetSpeechResolver supplies the app's own speech pipeline, so an export reuses the
// clips the app plays instead of synthesizing its own.
func (c *ScriptCompiler) SetSpeechResolver(speech scene.SpeechResolver) {
	c.speech = speech
	if reporter, ok := speech.(interface {
		Misses() []string
		Repairs() []string
	}); ok {
		c.speechReport = reporter
	}
}

// SpeechMisses reports the beats that resolved no clip, with the voice and keys each was
// looked for under. It is empty until Compile has run.
func (c *ScriptCompiler) SpeechMisses() []string {
	if c.speechReport == nil {
		return nil
	}
	return c.speechReport.Misses()
}

// SpeechRepairs reports the clips that were not Ogg/Opus and were re-encoded, or left out
// when they could not be. It is empty until Compile has run.
func (c *ScriptCompiler) SpeechRepairs() []string {
	if c.speechReport == nil {
		return nil
	}
	return c.speechReport.Repairs()
}

// SetNarratorVoice supplies the campaign's narrator voice. It must be the voice the app
// narrates with, or every narration clip is a cache miss.
func (c *ScriptCompiler) SetNarratorVoice(voice *entity.VoiceConfig) { c.narrator = voice }

// SetBanner supplies the campaign's own image for a scene that has none.
func (c *ScriptCompiler) SetBanner(path string) { c.banner = path }

// SetMedia disables art or audio resolution for an export.
func (c *ScriptCompiler) SetMedia(art, audio bool) {
	c.art = art
	c.audio = audio
}

// SetProgress routes structured progress to fn. When set, the compiler stops
// writing its human-readable progress to stderr, so a server-side export stays
// quiet in the logs.
func (c *ScriptCompiler) SetProgress(fn scene.ProgressFunc) {
	c.progress = fn
}

// Compile resolves a campaign's turns, art and audio into a playable script.
func (c *ScriptCompiler) Compile(ctx context.Context, gameID string) (*scene.Script, error) {
	gameDir := c.resolver.GameDir(gameID)

	manifest, err := core.LoadGameManifest(filepath.Join(gameDir, "game.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load manifest: %w", err)
	}

	store, err := storage.OpenGameStore(c.resolver, gameID)
	if err != nil {
		return nil, fmt.Errorf("open game store: %w", err)
	}
	defer store.Close()

	// Reindex from Markdown first, as playing a campaign does: an export should show
	// the notes as they are on disk rather than as an earlier process happened to
	// index them, and a portrait or a location it cannot see is a face or a place
	// missing from the bundle.
	_, _ = storage.NewSyncer(store).Sync(filepath.Join(gameDir, "entities"))

	worldStyle := ""
	if world, err := core.LoadWorldManifest(filepath.Join(c.resolver.WorldDir(manifest.WorldID), "world.yaml")); err == nil {
		worldStyle = strings.TrimSpace(strings.Join([]string{world.ArtStyle, world.Genre}, ", "))
	}

	compiler := scene.NewCompiler(&campaignSource{resolver: c.resolver, store: store, gameID: gameID})

	narrator := c.narratorVoice(manifest)
	banner := c.banner
	if banner == "" {
		banner = bannerPath(c.resolver, manifest)
	}

	if c.artResolver != nil {
		compiler.SetArtResolver(c.artResolver)
	} else if c.art && (c.config.Media.Image.BuiltinFallback || c.config.Media.Image.Type != "disabled") {
		if client, err := media.NewSceneImageClient(c.config.Media.Image); err == nil {
			cache := media.NewContentCache(c.resolver.CacheDir())
			params := c.config.Media.Image.Type + ":" + c.config.Media.Image.Model
			compiler.SetArtResolver(media.NewArtStore(client, cache, worldStyle, params))
		}
	}

	if c.speech != nil {
		compiler.SetSpeechResolver(c.speech)
	} else if c.audio {
		// Cached clips are the campaign's own audio and are worth playing even when
		// no provider can synthesize: without this an export run where the API key
		// lives in another environment skips every line instead of using the clips
		// it already has.
		// The shared provider key is what the app synthesizes with; without it a
		// CLI run cannot build the client and every miss would be silent even
		// though the app can speak.
		ttsKey, hasTTSKey := media.TTSKeyFor(c.config.Media.TTS)
		client, err := media.NewTTSClientWithSharedKey(c.config.Media.TTS, media.SharedProviderKey(c.config, ttsKey, hasTTSKey))
		if err != nil {
			client = media.NewCacheOnlyTTSClient()
		}
		cache := media.NewContentCache(c.resolver.CacheDir())
		pipeline := media.NewTTSPipeline(client, cache)
		pipeline.SetTextPolicy(media.TextPolicyFromConfig(c.config.Media.TTS))
		pipeline.SetOpusBitrate(c.config.OpusBitrate())
		compiler.SetSpeechResolver(NewSpeechResolver(pipeline, store, narrator))
	}

	// Portraits are always worth resolving: they are what makes an exported bundle
	// look like the theatre, and a missing one costs a face, not the export.
	compiler.SetPortraitResolver(&portraitResolver{
		resolver: c.resolver,
		store:    store,
		cache:    media.NewContentCache(c.resolver.CacheDir()),
		gameID:   gameID,
	})

	opts := scene.Options{
		Art:            c.art,
		Audio:          c.audio,
		WorldStyle:     worldStyle,
		ProviderParams: c.config.Media.Image.Type + ":" + c.config.Media.Image.Model,
		PlayerID:       manifest.Player,
		BannerPath:     banner,
		Progress:       c.progress,
	}
	if c.progress == nil {
		opts.OnProgress = func(format string, args ...interface{}) {
			fmt.Fprintf(os.Stderr, "export: "+format+"\n", args...)
		}
	}

	script, err := compiler.Compile(ctx, gameID, opts)
	if err != nil {
		return nil, err
	}

	script.GameName = manifest.Name
	script.PlayerName = c.playerName(store, manifest)

	// A bundle carries no codex and no navigation, so a link in its prose is noise:
	// [[the-quay]] reads as "The Quay", and an authored label wins over the name.
	names := c.entityName(store)
	for i := range script.Scenes {
		for j := range script.Scenes[i].Beats {
			beat := &script.Scenes[i].Beats[j]
			beat.Text = displayText(beat.Text, names)
		}
	}

	return script, nil
}

// wikilinkPattern matches [[Target]] and [[Target|Label]].
var wikilinkPattern = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)

// displayText rewrites an entity link to the name a reader sees. An authored label is kept
// as written; a bare target becomes the entity's name, or the target itself when nothing
// resolves, because a bundle has nothing to open.
func displayText(text string, name func(string) string) string {
	if name == nil || !strings.Contains(text, "[[") {
		return text
	}

	return wikilinkPattern.ReplaceAllStringFunc(text, func(match string) string {
		groups := wikilinkPattern.FindStringSubmatch(match)
		target := strings.TrimSpace(groups[1])
		if label := strings.TrimSpace(groups[2]); label != "" {
			return label
		}
		if resolved := name(target); resolved != "" {
			return resolved
		}
		return target
	})
}

// entityName resolves a link target to the name it is shown by, by id, name, or alias.
func (c *ScriptCompiler) entityName(store *storage.Store) func(string) string {
	return func(ref string) string {
		if strings.TrimSpace(ref) == "" || store == nil {
			return ""
		}
		if ent, err := store.GetEntity(ref); err == nil && ent != nil && strings.TrimSpace(ent.Name) != "" {
			return ent.Name
		}
		if id := harness.ResolveSpeakerID(store, ref); id != "" {
			if ent, err := store.GetEntity(id); err == nil && ent != nil {
				return ent.Name
			}
		}
		return ""
	}
}

// playerName resolves the protagonist's name, so a player shows it under their portrait as
// the theatre does. The note is canonical and the manifest's name is a copy taken at
// creation, so the entity wins: a hand-edited character is named as they are now.
func (c *ScriptCompiler) playerName(store *storage.Store, manifest *core.GameManifest) string {
	if manifest == nil {
		return ""
	}
	if manifest.Player != "" {
		if ent, err := store.GetEntity(manifest.Player); err == nil && ent != nil && strings.TrimSpace(ent.Name) != "" {
			return ent.Name
		}
	}
	return strings.TrimSpace(manifest.PlayerName)
}

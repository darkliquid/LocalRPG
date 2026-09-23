package export

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
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
// falling back to the reading estimate when probing fails.
type speechResolver struct {
	pipeline *media.TTSPipeline
	store    *storage.Store
	narrator *entity.VoiceConfig
}

func (r *speechResolver) SegmentAudio(ctx context.Context, segment entity.TurnSegment) (string, time.Duration, error) {
	path, err := r.pipeline.SynthesizeSegment(ctx, segment, r.narrator, r.voiceFor)
	if err != nil {
		return "", 0, err
	}

	duration, err := media.ProbeAudioDuration(ctx, path)
	if err != nil {
		return path, 0, nil
	}
	return path, duration, nil
}

func (r *speechResolver) voiceFor(speakerID string) *entity.VoiceConfig {
	return harness.ResolveSpeakerVoice(r.store, speakerID)
}

// ScriptCompiler builds a scene script for a campaign.
type ScriptCompiler struct {
	rootDir  string
	resolver *core.PathResolver
	config   *config.Config
	art      bool
	audio    bool
}

// NewScriptCompiler builds a compiler for one campaign root. Imagery and speech
// are both on by default: an export is expected to look and sound like the
// campaign unless it is deliberately told otherwise.
func NewScriptCompiler(rootDir string) *ScriptCompiler {
	cfg, _ := config.NewConfigManager().Load()
	return &ScriptCompiler{
		rootDir:  rootDir,
		resolver: core.NewPathResolver(rootDir),
		config:   cfg,
		art:      true,
		audio:    true,
	}
}

// SetMedia disables art or audio resolution for an export.
func (c *ScriptCompiler) SetMedia(art, audio bool) {
	c.art = art
	c.audio = audio
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

	worldStyle := ""
	if world, err := core.LoadWorldManifest(filepath.Join(c.resolver.WorldDir(manifest.WorldID), "world.yaml")); err == nil {
		worldStyle = strings.TrimSpace(strings.Join([]string{world.ArtStyle, world.Genre}, ", "))
	}

	compiler := scene.NewCompiler(&campaignSource{resolver: c.resolver, store: store, gameID: gameID})

	if c.art && (c.config.Media.Image.BuiltinFallback || c.config.Media.Image.Type != "disabled") {
		if client, err := media.NewSceneImageClient(c.config.Media.Image); err == nil {
			cache := media.NewContentCache(c.resolver.CacheDir())
			params := c.config.Media.Image.Type + ":" + c.config.Media.Image.Model
			compiler.SetArtResolver(media.NewArtStore(client, cache, worldStyle, params))
		}
	}

	if c.audio && c.config.Media.TTS.Type != "" && c.config.Media.TTS.Type != "disabled" {
		if client, err := media.NewTTSClient(c.config.Media.TTS); err == nil {
			cache := media.NewContentCache(c.resolver.CacheDir())
			pipeline := media.NewTTSPipeline(client, cache)
			pipeline.SetTextPolicy(media.TextPolicyFromConfig(c.config.Media.TTS))
			compiler.SetSpeechResolver(&speechResolver{
				pipeline: pipeline,
				store:    store,
				narrator: &entity.VoiceConfig{
					VoiceID:    c.config.Media.TTS.DefaultVoice,
					Pitch:      c.config.Media.TTS.Pitch,
					SpeechRate: c.config.Media.TTS.SpeechRate,
					Options:    c.config.Media.TTS.Options,
				},
			})
		}
	}

	script, err := compiler.Compile(ctx, gameID, scene.Options{
		Art:            c.art,
		Audio:          c.audio,
		WorldStyle:     worldStyle,
		ProviderParams: c.config.Media.Image.Type + ":" + c.config.Media.Image.Model,
		OnProgress: func(format string, args ...interface{}) {
			fmt.Fprintf(os.Stderr, "export: "+format+"\n", args...)
		},
	})
	if err != nil {
		return nil, err
	}

	script.GameName = manifest.Name
	return script, nil
}

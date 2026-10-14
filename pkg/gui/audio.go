// Speech: synthesis, grouping and clip planning, playback, voice resolution, and the TTS batch queue.

package gui

import (
	"context"
	"errors"
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
	"github.com/darkliquid/localrpg/pkg/media/playback"
	"github.com/darkliquid/localrpg/pkg/scene"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/trace"
	"github.com/darkliquid/localrpg/pkg/ttsbatch"
)

// clipPlanFor names a turn's clips from the shared pipeline. It returns an empty
// plan when no TTS provider is configured. Building the pipeline is cheap: a
// provider loads its model at first synthesis, not at construction.
func (s *Service) clipPlanFor(cfg *config.Config, gameID string, segments []entity.TurnSegment) clipPlan {
	plan := clipPlan{}
	if cfg.Media.TTS.Type == "" || cfg.Media.TTS.Type == "disabled" {
		return plan
	}
	pipeline, err := s.audioPipeline()
	if err != nil {
		return plan
	}
	narrator := s.narratorVoiceFor(gameID, cfg)
	voiceFor := s.voiceFor(gameID)
	plan.segmentKeys = make([][]string, len(segments))
	plan.groupKey = make([]string, len(segments))

	if s.groupingEnabled(cfg) {
		// The pipeline's group caps already fold under the live, single-speaker
		// caps when the streamer ran, so this names the clips it wrote and makes
		// the finalise pass a cache hit.
		groups := pipeline.GroupClipKeys(segments, narrator, voiceFor)
		plan.groups = make([]ClipGroupDTO, 0, len(groups))
		for _, group := range groups {
			plan.groups = append(plan.groups, ClipGroupDTO{
				Key:            group.Key,
				AudioURLs:      []string{clipURL(group.Key)},
				SegmentIndexes: group.SegmentIndexes,
			})
			for _, index := range group.SegmentIndexes {
				if index < 0 || index >= len(segments) {
					continue
				}
				plan.segmentKeys[index] = []string{group.Key}
				plan.groupKey[index] = group.Key
			}
		}
		return plan
	}

	for i, segment := range segments {
		keys, err := pipeline.SegmentClipKeys(segment, narrator, voiceFor)
		if err != nil {
			continue
		}
		plan.segmentKeys[i] = keys
	}
	return plan
}

// groupingEnabled reports whether a turn's audio is rendered as groups. "off"
// never groups; "auto" and "always" group. The streaming fold reproduces the
// batch plan, so a streamed turn groups live without paying twice.
func (s *Service) groupingEnabled(cfg *config.Config) bool {
	return cfg.TTSGrouping() != "off"
}

// streamerRuns reports whether the live pre-synthesiser runs for this turn.
func (s *Service) streamerRuns(cfg *config.Config) bool {
	return media.StreamerRuns(cfg)
}

// liveGrouping reports whether the streamer folds this turn's audio into groups,
// so the turn's clip plan must use the same single-speaker fold.
func (s *Service) liveGrouping(cfg *config.Config) bool {
	return media.LiveGrouping(cfg)
}

// ErrAudioUnavailable is the scene package's sentinel, kept as an alias here so// the route and its tests read unchanged and there is only one value to compare.
var ErrAudioUnavailable = scene.ErrAudioUnavailable

// ErrNoBatchAPI means the configured TTS provider has no batch endpoint, so a
// backfill cannot be submitted to it. It is ours to answer, and the message says
// what to change.
var ErrNoBatchAPI = errors.New("the configured TTS provider has no batch API")

func (s *Service) addPortraitListener(gameID string, listener func(TurnEvent)) func() {
	s.portraitMu.Lock()
	defer s.portraitMu.Unlock()
	if s.portraitListeners == nil {
		s.portraitListeners = make(map[string]map[uint64]func(TurnEvent))
	}
	if s.portraitListeners[gameID] == nil {
		s.portraitListeners[gameID] = make(map[uint64]func(TurnEvent))
	}
	s.portraitSeq++
	id := s.portraitSeq
	s.portraitListeners[gameID][id] = listener

	return func() {
		s.portraitMu.Lock()
		defer s.portraitMu.Unlock()
		if m := s.portraitListeners[gameID]; m != nil {
			delete(m, id)
			if len(m) == 0 {
				delete(s.portraitListeners, gameID)
			}
		}
	}
}

func (s *Service) broadcastPortraitReady(gameID, characterID, relPath string, version ...int) {
	s.portraitMu.Lock()
	var listeners []func(TurnEvent)
	if m := s.portraitListeners[gameID]; m != nil {
		for _, fn := range m {
			listeners = append(listeners, fn)
		}
	}
	s.portraitMu.Unlock()

	ver := 0
	if len(version) > 0 {
		ver = version[0]
	}
	if ver <= 0 {
		ver = parsePortraitVersionFromPath(relPath)
	}
	portraitURL := fmt.Sprintf("/api/game/%s/character/%s/portrait?t=%d", gameID, characterID, time.Now().UnixMilli())
	if ver > 0 {
		portraitURL = fmt.Sprintf("/api/game/%s/character/%s/portrait?v=%d&t=%d", gameID, characterID, ver, time.Now().UnixMilli())
	}

	evt := TurnEvent{
		Type:              "portrait",
		CharacterID:       characterID,
		PortraitURL:       portraitURL,
		Version:           ver,
		HasCustomPortrait: true,
	}
	for _, fn := range listeners {
		fn(evt)
	}
}

// portraitExtensions are the image formats a portrait file may be stored in, in the
// order they are tried.
var portraitExtensions = []string{".png", ".webp", ".jpg", ".jpeg", ".svg"}

// GetCharacterPortrait returns the portrait image for a character, or a procedural SVG fallback if not found.
// When an explicit version (> 0) is specified, it serves that exact historical portrait version.
func (s *Service) GetCharacterPortrait(ctx context.Context, gameID, characterID string, version ...int) ([]byte, string, error) {
	s.ensureIndexed(gameID)
	cleanID := entity.Slugify(characterID)
	if cleanID == "" {
		return nil, "", fmt.Errorf("invalid character id %q", characterID)
	}

	gameDir := filepath.Clean(s.resolver.GameDir(gameID))
	ent := s.portraitEntity(gameDir, gameID, cleanID)
	lookup := portraitLookup{
		ent:          ent,
		gameDir:      gameDir,
		portraitsDir: filepath.Clean(filepath.Join(gameDir, "assets", "portraits")),
		safeID:       ent.ID,
	}

	if len(version) > 0 && version[0] > 0 {
		// An explicit version serves that exact historical portrait.
		if data, contentType, ok := lookup.versioned(version[0]); ok {
			return data, contentType, nil
		}
	} else if data, contentType, ok := lookup.current(); ok {
		// No version asked for, so the portrait in force is the one served.
		return data, contentType, nil
	}

	// Procedural SVG fallback
	svg := media.GenerateProceduralPortrait(media.PortraitRequestFor(ent))
	return svg, "image/svg+xml", nil
}

// portraitLookup is where a portrait is read from: the note the character resolved
// to, the campaign directory its portrait paths are relative to, and the portraits
// directory and id its files are named by.
type portraitLookup struct {
	ent          *entity.Entity
	gameDir      string
	portraitsDir string
	safeID       string
}

// portraitEntity is the note a portrait is asked about, falling back to the note on
// disk when the index has not caught up. An unknown id still gets a placeholder
// derived from the id, so a client never renders a broken portrait for a speaker it
// cannot resolve.
func (s *Service) portraitEntity(gameDir, gameID, cleanID string) *entity.Entity {
	if store, storeErr := s.store(gameID); storeErr == nil {
		if loaded, loadErr := store.GetEntity(cleanID); loadErr == nil && loaded != nil {
			return loaded
		}
	}

	notePath := filepath.Clean(filepath.Join(gameDir, "entities", cleanID+".md"))
	if strings.HasPrefix(notePath, gameDir+string(filepath.Separator)) {
		if data, readErr := os.ReadFile(notePath); readErr == nil {
			if parsed, parseErr := entity.ParseMarkdownEntity(data); parseErr == nil {
				return parsed
			}
		}
	}
	return &entity.Entity{ID: cleanID, Name: cleanID, Type: "character"}
}

// current finds the portrait in force: the note's own portrait when it has one, and
// otherwise the unversioned file on disk.
func (l portraitLookup) current() ([]byte, string, bool) {
	if l.ent.Portrait != "" {
		if data, ok := readGameFileIn(l.gameDir, l.ent.Portrait); ok {
			return data, imageContentType(data), true
		}
	}
	for _, ext := range portraitExtensions {
		if data, ok := readPortraitImage(l.portraitsDir, l.safeID+ext); ok {
			return data, imageContentType(data), true
		}
	}
	return nil, "", false
}

// versioned finds the image for one historical version, in the order the files it may
// be stored as: the versioned file, the note's current portrait when that is the
// version asked for, the note's portrait history, and finally the unversioned file,
// which is the first version by definition.
func (l portraitLookup) versioned(targetVer int) ([]byte, string, bool) {
	for _, ext := range portraitExtensions {
		if data, ok := readPortraitImage(l.portraitsDir, fmt.Sprintf("%s-v%d%s", l.safeID, targetVer, ext)); ok {
			return data, imageContentType(data), true
		}
	}
	if l.ent.PortraitVersion == targetVer && l.ent.Portrait != "" {
		if data, ok := readGameFileIn(l.gameDir, l.ent.Portrait); ok {
			return data, imageContentType(data), true
		}
	}
	for _, histPath := range l.ent.PortraitHistory {
		if strings.Contains(histPath, fmt.Sprintf("-v%d.", targetVer)) {
			if data, ok := readGameFileIn(l.gameDir, histPath); ok {
				return data, imageContentType(data), true
			}
		}
	}
	if targetVer == 1 {
		for _, ext := range portraitExtensions {
			if data, ok := readPortraitImage(l.portraitsDir, l.safeID+ext); ok {
				return data, imageContentType(data), true
			}
		}
	}
	return nil, "", false
}

// readGameFileIn reads a path recorded on a note, which is relative to the campaign
// directory.
func readGameFileIn(gameDir, rel string) ([]byte, bool) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return nil, false
	}
	return readWithin(gameDir, rel)
}

// readPortraitImage reads one file of the portraits directory by name, ignoring any
// directory part the name carries.
func readPortraitImage(portraitsDir, name string) ([]byte, bool) {
	return readWithin(portraitsDir, filepath.Base(name))
}

// readWithin reads a file under base, refusing anything that resolves outside it. The
// containment check is inline because this is where the path is read.
func readWithin(base, rel string) ([]byte, bool) {
	p := filepath.Clean(filepath.Join(base, rel))
	if !strings.HasPrefix(p, base+string(filepath.Separator)) {
		return nil, false
	}
	data, readErr := os.ReadFile(p)
	if readErr != nil || len(data) == 0 {
		return nil, false
	}
	return data, true
}

// RegenerateCharacterPortrait generates a fresh portrait for a character and
// overwrites any existing one, returning a cache-busted URL.
func (s *Service) RegenerateCharacterPortrait(ctx context.Context, gameID, characterID string) (CharacterPortraitDTO, error) {
	s.ensureIndexed(gameID)
	cleanID := entity.Slugify(characterID)
	if cleanID == "" {
		return CharacterPortraitDTO{}, fmt.Errorf("invalid character id %q", characterID)
	}

	gameDir := filepath.Clean(s.resolver.GameDir(gameID))
	store, err := s.store(gameID)
	if err != nil {
		return CharacterPortraitDTO{}, err
	}
	ent, err := store.GetEntity(cleanID)
	if err != nil || ent == nil {
		notePath := filepath.Clean(filepath.Join(gameDir, "entities", cleanID+".md"))
		if strings.HasPrefix(notePath, gameDir+string(filepath.Separator)) {
			if data, readErr := os.ReadFile(notePath); readErr == nil {
				if parsed, parseErr := entity.ParseMarkdownEntity(data); parseErr == nil {
					ent = parsed
				}
			}
		}
	}
	if ent == nil {
		return CharacterPortraitDTO{}, fmt.Errorf("character %q not found", characterID)
	}
	if ent.Type != "character" {
		return CharacterPortraitDTO{}, &harness.GenerationFailure{
			Code:    harness.FailureInvalidRequest,
			Message: fmt.Sprintf("%q is not a character", characterID),
		}
	}

	cfg := s.configMgr.Get()
	if cfg.Media.Image.Type == "" || cfg.Media.Image.Type == "disabled" {
		return CharacterPortraitDTO{}, &harness.GenerationFailure{
			Code:    harness.FailureProviderUnavailable,
			Message: "image generation is disabled or unconfigured",
		}
	}
	client, err := imageClientFactory(cfg.Media.Image, cfg.Providers.Gemini.APIKey)
	if err != nil {
		return CharacterPortraitDTO{}, &harness.GenerationFailure{
			Code:    harness.FailureProviderUnavailable,
			Message: fmt.Sprintf("image provider: %v", err),
			Cause:   err,
		}
	}

	provider := cfg.Media.Image.BuiltinName
	if provider == "" {
		provider = cfg.Media.Image.Type
	}
	started := time.Now()
	ctx, span := startImageSpan(ctx, s.logger, "portrait")
	defer span.End()

	worker := engine.NewPortraitWorker(s.resolver, store, client)
	worker.SetOnReady(func(gID, charID, relPath string) {
		s.broadcastPortraitReady(gID, charID, relPath)
	})
	if _, err := worker.Regenerate(ctx, gameID, ent, s.worldArtStyle(gameID)); err != nil {
		failure := &harness.GenerationFailure{
			Code:    harness.ClassifyProviderError(err),
			Message: fmt.Sprintf("generate portrait: %v", err),
			Cause:   err,
		}
		s.recordImage(ctx, span, "portrait", provider, 0, started, failure)
		return CharacterPortraitDTO{}, failure
	}
	s.recordImage(ctx, span, "portrait", provider, 0, started, nil)

	return CharacterPortraitDTO{
		PortraitURL: fmt.Sprintf("/api/game/%s/character/%s/portrait?t=%d", gameID, characterID, time.Now().UnixNano()),
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// ClipPath resolves a clip key to its stored file. The key is the clip's whole
// name and nothing else is consulted, so a key that is not one resolves to
// nothing rather than to a path.
func (s *Service) ClipPath(key string) (string, bool) {
	if !clipKeyPattern.MatchString(key) {
		return "", false
	}
	path := filepath.Join(s.resolver.CacheDir(), "audio", key+".opus")
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return "", false
	}
	return path, true
}

// clipPath names the file a clip key is stored under. The player plays files, and
// the key is the file name, so no lookup is needed.
func (s *Service) clipPath(key string) string {
	if key == "" {
		return ""
	}
	return filepath.Join(s.resolver.CacheDir(), "audio", key+".opus")
}

// GetSegmentClips synthesizes one segment on demand and returns its ordered clips,
// reusing every clip the cache already holds. When grouping is enabled a segment
// shares its group's clip, so regenerating one segment regenerates the group it
// belongs to.
func (s *Service) GetSegmentClips(ctx context.Context, gameID string, turnNumber, segmentIndex int, force ...bool) ([]string, error) {
	turn, err := s.findTurn(gameID, turnNumber)
	if err != nil {
		return nil, err
	}
	if segmentIndex < 0 || segmentIndex >= len(turn.Segments) {
		return nil, fmt.Errorf("segment %d out of range for turn %d", segmentIndex, turnNumber)
	}
	isForce := len(force) > 0 && force[0]

	cfg := s.configMgr.Get()
	pipeline, err := s.audioPipeline()
	if err != nil {
		return nil, err
	}
	if s.groupingEnabled(cfg) {
		groups := pipeline.GroupClipKeys(turn.Segments, s.narratorVoiceFor(gameID, cfg), s.voiceFor(gameID))
		if group, ok := media.GroupForSegment(groups, segmentIndex); ok {
			rendered, err := pipeline.SynthesizeGroupsForce(ctx, []media.ClipGroup{group}, isForce)
			if err != nil {
				s.noteFailure("tts", err)
				return nil, err
			}
			s.noteSuccess("tts")
			s.recordTTSUsage(gameID, turnNumber, cfg, pipeline)
			if len(rendered) > 0 && rendered[0].Cached {
				return []string{s.clipPath(rendered[0].Key)}, nil
			}
			return nil, nil
		}
	}
	return s.synthesizeSegment(ctx, gameID, *turn, segmentIndex, isForce)
}

// synthesizeSegment renders one segment through the ungrouped per-segment path,
// the fallback when grouping is off or a group failed.
func (s *Service) synthesizeSegment(ctx context.Context, gameID string, turn engine.Turn, segmentIndex int, force bool) ([]string, error) {
	if segmentIndex < 0 || segmentIndex >= len(turn.Segments) {
		return nil, fmt.Errorf("segment %d out of range for turn %d", segmentIndex, turn.Number)
	}
	cfg := s.configMgr.Get()
	pipeline, err := s.audioPipeline()
	if err != nil {
		return nil, err
	}
	clips, err := pipeline.SynthesizeSegmentClips(ctx, turn.Segments[segmentIndex], s.narratorVoiceFor(gameID, cfg), s.voiceFor(gameID), force)
	if err != nil {
		s.noteFailure("tts", err)
		return nil, err
	}
	s.noteSuccess("tts")
	s.recordTTSUsage(gameID, turn.Number, cfg, pipeline)
	return clips, nil
}

// synthesizeTurnGroups renders a whole turn's groups when grouping is enabled,
// reporting whether it did. It returns the rendered groups so a caller can fall
// back to per-segment synthesis for any group that failed.
func (s *Service) synthesizeTurnGroups(ctx context.Context, gameID string, turn engine.Turn, cfg *config.Config, force bool) ([]media.ClipGroup, bool, error) {
	pipeline, err := s.audioPipeline()
	if err != nil {
		return nil, false, err
	}
	if !s.groupingEnabled(cfg) {
		return nil, false, nil
	}
	// The clip plan and the synthesis fold under the pipeline's group caps, which
	// are already the live, single-speaker caps when the streamer ran, so the DTO
	// names the clips the finalise pass wrote.
	groups := pipeline.GroupClipKeys(turn.Segments, s.narratorVoiceFor(gameID, cfg), s.voiceFor(gameID))
	rendered, err := pipeline.SynthesizeGroupsForce(ctx, groups, force)
	if err != nil {
		s.noteFailure("tts", err)
	} else {
		s.noteSuccess("tts")
	}
	s.recordTTSUsage(gameID, turn.Number, cfg, pipeline)
	return rendered, true, err
}

// emitTurnClips yields a turn's clips in play order, grouped when grouping is
// enabled and per-segment otherwise, so every consumer agrees on what a turn
// sounds like. A group that failed falls back to per-segment synthesis so the
// beat is not silent. plan, when non-nil, is the heard ledger: a group whose
// segments are all heard is skipped, and a partially heard group is suppressed
// and traced, so no clip plays twice. The plan's playback ledger refines that per
// clip: a clip the player heard to its end is skipped, and one cut short is
// suppressed here, because the device cannot seek, and resumed by the client from
// its recorded offset.
func (s *Service) emitTurnClips(ctx context.Context, gameID string, turn engine.Turn, force bool, emit func(string), plan *turnAudioPlan) {
	cfg := s.configMgr.Get()
	// emitClip applies the ledger to one clip before sending it.
	emitClip := func(clip string, indexes []int) {
		if plan != nil && s.ledgerHolds(plan, media.ClipKeyForPath(clip), indexes) {
			return
		}
		emit(clip)
	}
	if groups, grouped, _ := s.synthesizeTurnGroups(ctx, gameID, turn, cfg, force); grouped {
		for _, group := range groups {
			if plan != nil {
				all, some := plan.heardState(group.SegmentIndexes)
				if all {
					continue
				}
				if some {
					trace.OrNil(s.logger).Event("turn.audio_partial", map[string]any{
						"indexes": group.SegmentIndexes,
					})
					continue
				}
				if s.ledgerHolds(plan, group.Key, group.SegmentIndexes) {
					plan.markHeard(group.SegmentIndexes)
					continue
				}
			}
			if group.Cached {
				emit(s.clipPath(group.Key))
				plan.markHeard(group.SegmentIndexes)
				continue
			}
			for _, index := range group.SegmentIndexes {
				clips, err := s.synthesizeSegment(ctx, gameID, turn, index, false)
				if err != nil {
					s.broadcastAudioFailure(turn.Number, index, err, "synthesize")
					continue
				}
				for _, clip := range clips {
					emitClip(clip, []int{index})
				}
			}
			plan.markHeard(group.SegmentIndexes)
		}
		return
	}
	for i := range turn.Segments {
		if plan != nil && plan.heardAll([]int{i}) {
			continue
		}
		clips, err := s.synthesizeSegment(ctx, gameID, turn, i, force)
		if err != nil {
			s.broadcastAudioFailure(turn.Number, i, err, "synthesize")
			continue
		}
		for _, clip := range clips {
			emitClip(clip, []int{i})
		}
		plan.markHeard([]int{i})
	}
}

// ledgerHolds reports whether the plan's playback ledger already accounts for a
// clip, so the finalise pass must not send it: heard to its end, or cut short
// and owed to the client from its offset. A suppressed partial clip is traced
// with that offset, so a silent remainder on the device is diagnosable.
func (s *Service) ledgerHolds(plan *turnAudioPlan, key string, indexes []int) bool {
	verdict, entry := plan.verdict(key)
	switch verdict {
	case clipHeard:
		return true
	case clipPartial:
		trace.OrNil(s.logger).Event("turn.audio_partial", map[string]any{
			"indexes":   indexes,
			"key":       key,
			"played_ms": entry.PlayedMS,
			"total_ms":  entry.TotalMS,
		})
		return true
	default:
		return false
	}
}

// recordTTSUsage records what a synthesis consumed. A cache hit reports nothing,
// so only a real synthesis is recorded.
func (s *Service) recordTTSUsage(gameID string, turnNumber int, cfg *config.Config, pipeline *media.TTSPipeline) {
	key, ok := media.TTSKeyFor(cfg.Media.TTS)
	if !ok {
		return
	}
	if u := pipeline.LastUsage(); u.Characters != 0 || u.InputTokens != 0 || u.OutputTokens != 0 || u.Requests != 0 {
		s.RecordUsage(gameID, turnNumber, "tts", mediaUsage(u, key, cfg.Media.TTS.Model))
	}
}

// audioPipeline returns the shared TTS pipeline, building it when the current
// configuration object differs from the one it was built from. Sharing it means a
// built-in TTS model is loaded once, not once per segment.
func (s *Service) audioPipeline() (*media.TTSPipeline, error) {
	cfg := s.configMgr.Get()
	if cfg.Media.TTS.Type == "" || cfg.Media.TTS.Type == "disabled" {
		return nil, ErrAudioUnavailable
	}

	s.ttsMu.Lock()
	defer s.ttsMu.Unlock()
	if s.ttsPipeline != nil && s.ttsConfig == cfg {
		return s.ttsPipeline, nil
	}

	client, err := s.ttsClientFor(cfg.Media.TTS)
	if err != nil {
		return nil, fmt.Errorf("build tts client: %w", err)
	}
	pipeline := media.NewTTSPipeline(client, media.NewContentCache(s.resolver.CacheDir()))
	pipeline.SetTextPolicy(media.TextPolicyFromConfig(cfg.Media.TTS))
	pipeline.SetOpusBitrate(cfg.OpusBitrate())
	pipeline.SetGroupCaps(media.TurnGroupCaps(cfg, media.ResolveGroupCaps(cfg.Media.TTS, client)))
	pipeline.SetSpeechCues(media.ResolveSpeechCueCapabilities(cfg.Media.TTS, client))
	pipeline.SetAudioTagDelivery(cfg.Media.TTS.SpeechCues.AudioTags != nil && *cfg.Media.TTS.SpeechCues.AudioTags)
	s.ttsConfig, s.ttsPipeline = cfg, pipeline
	return pipeline, nil
}

// narratorVoiceFor resolves the narrator voice for a campaign, preferring any
// campaign-level setting in game.yaml and falling back to media.tts.default_voice.
func (s *Service) narratorVoiceFor(gameID string, cfg *config.Config) *entity.VoiceConfig {
	voiceID := ""
	if cfg != nil {
		voiceID = cfg.Media.TTS.DefaultVoice
	}
	if manifest, err := core.LoadGameManifest(filepath.Join(s.resolver.GameDir(gameID), "game.yaml")); err == nil && manifest != nil && manifest.Settings != nil {
		if nv, ok := manifest.Settings["narrator_voice"].(string); ok && strings.TrimSpace(nv) != "" {
			voiceID = strings.TrimSpace(nv)
		}
	}
	res := &entity.VoiceConfig{
		VoiceID: voiceID,
	}
	if cfg != nil {
		res.Pitch = cfg.Media.TTS.Pitch
		res.SpeechRate = cfg.Media.TTS.SpeechRate
		res.Options = cfg.Media.TTS.Options
	}
	return res
}

// audioPlayer opens the process-wide player on first use. A host with no audio
// device leaves it nil, and callers fall back to client-side playback.
func (s *Service) audioPlayer() *playback.Player {
	s.playerOnce.Do(func() {
		volume := 1.0
		if s.configMgr != nil {
			volume = s.configMgr.Get().Media.TTS.MasterVolume
		}
		player, err := playback.Open(volume)
		if err != nil {
			return
		}

		player.SetOnComplete(func() {
			s.audioSubMu.Lock()
			status := AudioStatusDTO{Available: true, Playing: false, Turn: s.audioTurn, Segment: s.audioSegment, Owner: s.PlaybackOwner()}
			s.audioSubMu.Unlock()
			s.broadcastAudioStatus(status)
		})
		s.player = player
	})
	return s.player
}

// SubscribeAudioStatus registers a channel notified when application playback
// ends, so a client can advance on a real completion rather than polling.
func (s *Service) SubscribeAudioStatus() (<-chan AudioStatusDTO, func()) {
	ch := make(chan AudioStatusDTO, 1)
	s.audioSubMu.Lock()
	if s.audioSubs == nil {
		s.audioSubs = make(map[chan AudioStatusDTO]struct{})
	}
	s.audioSubs[ch] = struct{}{}
	s.audioSubMu.Unlock()
	return ch, func() {
		s.audioSubMu.Lock()
		delete(s.audioSubs, ch)
		s.audioSubMu.Unlock()
	}
}

func (s *Service) broadcastAudioStatus(status AudioStatusDTO) {
	if status.Owner == "" {
		status.Owner = s.PlaybackOwner()
	}
	s.audioSubMu.Lock()
	defer s.audioSubMu.Unlock()
	for ch := range s.audioSubs {
		select {
		case ch <- status:
		default:
		}
	}
}

// broadcastAudioFailure reports a beat that could not be produced, so a client
// shows why a beat went quiet rather than treating the silence as success.
func (s *Service) broadcastAudioFailure(turnNumber, segment int, err error, stage string) {
	if err == nil {
		return
	}
	s.broadcastAudioStatus(AudioStatusDTO{
		Available: true,
		Playing:   false,
		Turn:      turnNumber,
		Segment:   segment,
		Error:     err.Error(),
		Stage:     stage,
	})
}

// setAudioCurrent records the beat a queue is playing, so its completion event
// carries the identity a client needs to advance the right beat. Segment is -1
// for a whole-turn queue.
func (s *Service) setAudioCurrent(turn, segment int) {
	s.audioSubMu.Lock()
	s.audioTurn, s.audioSegment = turn, segment
	s.audioSubMu.Unlock()
}

// AudioAvailable reports whether this process can play audio itself, which is
// what decides between application playback and a browser audio element.
func (s *Service) AudioAvailable() bool {
	return s.audioPlayer().Available()
}

// AudioPlaying reports whether a narration queue is running.
func (s *Service) AudioPlaying() bool {
	player := s.audioPlayer()
	if player == nil {
		return false
	}
	return player.Playing()
}

// StopAudio cancels the current narration queue.
func (s *Service) StopAudio() {
	if player := s.audioPlayer(); player != nil {
		player.Stop()
	}
	s.broadcastAudioStatus(AudioStatusDTO{Available: s.AudioAvailable(), Playing: false, Owner: s.PlaybackOwner()})
}

// CountUncachedBeats reports how much of a campaign's speech is already cached,
// so a bulk synthesis can warn before spending money on a metered provider.
func (s *Service) CountUncachedBeats(gameID string) (cached, uncached int, err error) {
	cfg := s.configMgr.Get()
	client, err := s.ttsClientFor(cfg.Media.TTS)
	if err != nil {
		return 0, 0, fmt.Errorf("build tts client: %w", err)
	}

	turns, err := s.cachedHistory(gameID)
	if err != nil {
		return 0, 0, fmt.Errorf("load history: %w", err)
	}

	narratorVoice := s.narratorVoiceFor(gameID, cfg)
	pipeline := media.NewTTSPipeline(client, media.NewContentCache(s.resolver.CacheDir()))
	pipeline.SetTextPolicy(media.TextPolicyFromConfig(cfg.Media.TTS))
	pipeline.SetOpusBitrate(cfg.OpusBitrate())
	pipeline.SetGroupCaps(media.TurnGroupCaps(cfg, media.ResolveGroupCaps(cfg.Media.TTS, client)))
	pipeline.SetSpeechCues(media.ResolveSpeechCueCapabilities(cfg.Media.TTS, client))
	pipeline.SetAudioTagDelivery(cfg.Media.TTS.SpeechCues.AudioTags != nil && *cfg.Media.TTS.SpeechCues.AudioTags)

	voiceFor := s.voiceFor(gameID)
	grouped := s.groupingEnabled(cfg)
	for _, turn := range turns {
		if len(turn.Segments) == 0 {
			continue
		}
		if grouped {
			groups := pipeline.GroupClipKeys(turn.Segments, narratorVoice, voiceFor)
			turnCached, turnUncached := pipeline.CountUncachedGroups(groups)
			cached += turnCached
			uncached += turnUncached
			continue
		}
		turnCached, turnUncached := pipeline.CountUncached(turn.Segments, narratorVoice, voiceFor)
		cached += turnCached
		uncached += turnUncached
	}
	return cached, uncached, nil
}

// TTSBatchJobs lists a campaign's offline batch jobs, newest first.
func (s *Service) TTSBatchJobs(gameID string) ([]TTSBatchJobDTO, error) {
	store, err := s.store(gameID)
	if err != nil {
		return nil, err
	}
	jobs, err := store.ListTTSJobs(gameID)
	if err != nil {
		return nil, err
	}
	out := make([]TTSBatchJobDTO, 0, len(jobs))
	for _, job := range jobs {
		dto := ttsBatchJobDTO(job)
		dto.GameID = gameID
		out = append(out, dto)
	}
	return out, nil
}

// AllTTSBatchJobs lists every campaign's batch jobs, so the global manager can
// show and filter them by campaign.
func (s *Service) AllTTSBatchJobs(ctx context.Context) ([]TTSBatchJobDTO, error) {
	games, err := s.ListGames(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]TTSBatchJobDTO, 0)
	for _, game := range games {
		store, err := s.store(game.ID)
		if err != nil {
			continue
		}
		jobs, err := store.ListTTSJobs(game.ID)
		if err != nil {
			continue
		}
		for _, job := range jobs {
			dto := ttsBatchJobDTO(job)
			dto.GameID = game.ID
			dto.GameName = game.Name
			out = append(out, dto)
		}
	}
	return out, nil
}

// CancelTTSBatch cancels a submitted batch job and records it as cancelled,
// preserving the progress it had reached.
func (s *Service) CancelTTSBatch(ctx context.Context, gameID, jobID string) error {
	cfg := s.configMgr.Get()
	client, err := s.ttsClientFor(cfg.Media.TTS)
	if err != nil {
		return err
	}
	batchClient, ok := client.(media.BatchTTSClient)
	if !ok {
		return ErrNoBatchAPI
	}
	if err := batchClient.CancelBatch(ctx, media.BatchJobHandle{ID: jobID}); err != nil {
		return err
	}
	store, err := s.store(gameID)
	if err != nil {
		return err
	}
	if job, err := store.GetTTSJob(jobID); err == nil && job != nil {
		return store.UpdateTTSJobStatus(jobID, "cancelled", job.Completed, job.FailedKeys)
	}
	return store.UpdateTTSJobStatus(jobID, "cancelled", 0, nil)
}

// DeleteTTSBatch removes a finished batch job. An in-flight job must be
// cancelled first, so a delete never hides work that is still running.
func (s *Service) DeleteTTSBatch(ctx context.Context, gameID, jobID string) error {
	store, err := s.store(gameID)
	if err != nil {
		return err
	}
	job, err := store.GetTTSJob(jobID)
	if err != nil {
		return err
	}
	if job == nil {
		return nil
	}
	if batchJobActive(*job) {
		return fmt.Errorf("batch job %s is still running; cancel it first", jobID)
	}
	return store.DeleteTTSJob(jobID)
}

// ClearTTSBatch removes every finished batch job, for one campaign or, when
// gameID is empty, for all of them. It returns how many jobs it removed.
func (s *Service) ClearTTSBatch(ctx context.Context, gameID string) (int, error) {
	if gameID != "" {
		store, err := s.store(gameID)
		if err != nil {
			return 0, err
		}
		return store.DeleteFinishedTTSJobs(gameID)
	}

	games, err := s.ListGames(ctx)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, game := range games {
		store, err := s.store(game.ID)
		if err != nil {
			continue
		}
		removed, err := store.DeleteFinishedTTSJobs(game.ID)
		if err != nil {
			continue
		}
		total += removed
	}
	return total, nil
}

// ResumeTTSBatch finishes a job now: it polls the provider and, once the job is
// done, downloads the output, converts it to Opus, and writes it to the cache.
// It is how a job that never stored its clips (an output file that arrived
// empty, an interrupted download) is completed without waiting for a restart.
func (s *Service) ResumeTTSBatch(ctx context.Context, gameID, jobID string) (*TTSBatchJobDTO, error) {
	cfg := s.configMgr.Get()
	client, err := s.ttsClientFor(cfg.Media.TTS)
	if err != nil {
		return nil, err
	}
	batchClient, ok := client.(media.BatchTTSClient)
	if !ok {
		return nil, ErrNoBatchAPI
	}
	store, err := s.store(gameID)
	if err != nil {
		return nil, err
	}
	job, err := store.GetTTSJob(jobID)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, fmt.Errorf("batch job %s not found", jobID)
	}

	engine := ttsbatch.New(batchClient, media.NewContentCache(s.resolver.CacheDir()), store)
	engine.SetOpusBitrate(cfg.OpusBitrate())
	opts := ttsbatch.Options{GameID: gameID, Provider: job.Provider, Model: job.Model}
	s.goBackground(func() {
		if _, err := engine.Resume(s.bgCtx, opts, jobID); err != nil {
			trace.OrNil(s.logger).Event("media.tts.batch_error", map[string]any{
				"game":  gameID,
				"job":   jobID,
				"error": err.Error(),
			})
			s.noteFailure("tts", err)
			return
		}
		s.noteSuccess("tts")
	})

	dto := ttsBatchJobDTO(*job)
	dto.GameID = gameID
	return &dto, nil
}

// StartTTSBatch submits an offline batch backfill for a campaign and finishes it
// in the background, so the request returns at once and the panel watches the
// job row. It returns the submitted job, or nil when every clip is already
// cached.
// StartTTSBatch submits an offline batch backfill for a campaign and finishes it
// in the background, so the request returns at once and the panel watches the
// job row. It returns the submitted job, or nil when every clip is already
// cached.
func (s *Service) StartTTSBatch(ctx context.Context, gameID string, force bool) (*TTSBatchJobDTO, error) {
	cfg := s.configMgr.Get()
	client, err := s.ttsClientFor(cfg.Media.TTS)
	if err != nil {
		return nil, err
	}
	batchClient, ok := client.(media.BatchTTSClient)
	if !ok {
		return nil, ErrNoBatchAPI
	}
	store, err := s.store(gameID)
	if err != nil {
		return nil, err
	}

	// Starting twice must not queue two jobs for the same work: a backfill already
	// in flight is returned as-is, so the button is idempotent.
	s.batchStartMu.Lock()
	defer s.batchStartMu.Unlock()
	if existing, ok := activeBatchJobDTO(store, gameID); ok {
		return &existing, nil
	}

	pipeline, err := s.audioPipeline()
	if err != nil {
		return nil, err
	}
	groups, err := s.batchClipGroups(gameID, cfg, pipeline)
	if err != nil {
		return nil, err
	}

	providerKey := ttsProviderKey(cfg.Media.TTS)
	batchEngine := ttsbatch.New(batchClient, media.NewContentCache(s.resolver.CacheDir()), store)
	batchEngine.SetOpusBitrate(cfg.OpusBitrate())
	opts := ttsbatch.Options{GameID: gameID, Provider: providerKey, Model: cfg.Media.TTS.Model, Force: force}

	job, err := batchEngine.Submit(ctx, opts, groups)
	if err != nil {
		s.logTTSBatchError(gameID, providerKey, "", err)
		return nil, err
	}
	if job == nil {
		return nil, nil
	}

	// A batch job can take hours, so it finishes in the background and the panel
	// watches the job row rather than holding a request open. It runs on the
	// service's background context, so closing the app cancels the poll instead of
	// waiting it out; the job stays resumable and is picked up next launch.
	s.goBackground(func() { s.finishTTSBatch(batchEngine, opts, job.ID, gameID, providerKey) })

	dto := ttsBatchJobDTO(*job)
	return &dto, nil
}

// batchClipGroups is every clip a backfill would synthesize, in turn order: the
// segments of each recorded turn folded into the groups the provider is asked for.
// A turn with no segments contributes nothing.
func (s *Service) batchClipGroups(gameID string, cfg *config.Config, pipeline *media.TTSPipeline) ([]media.ClipGroup, error) {
	turns, err := s.cachedHistory(gameID)
	if err != nil {
		return nil, err
	}

	narrator := s.narratorVoiceFor(gameID, cfg)
	voiceFor := s.voiceFor(gameID)
	groups := make([]media.ClipGroup, 0)
	for _, turn := range turns {
		if len(turn.Segments) == 0 {
			continue
		}
		groups = append(groups, pipeline.GroupClipKeys(turn.Segments, narrator, voiceFor)...)
	}
	return groups, nil
}

// activeBatchJobDTO is the backfill already running for a campaign, if there is one.
func activeBatchJobDTO(store *storage.Store, gameID string) (TTSBatchJobDTO, bool) {
	existing, ok := activeBatchJob(store, gameID)
	if !ok {
		return TTSBatchJobDTO{}, false
	}
	dto := ttsBatchJobDTO(*existing)
	dto.GameID = gameID
	return dto, true
}

// ttsProviderKey is the key a batch job's provider is recorded under, which is empty
// for a provider that has no key.
func ttsProviderKey(ttsCfg config.TTSConfig) string {
	if key, ok := media.TTSKeyFor(ttsCfg); ok {
		return string(key)
	}
	return ""
}

// finishTTSBatch runs a submitted backfill to completion in the background and
// records the outcome either way.
func (s *Service) finishTTSBatch(batchEngine *ttsbatch.Engine, opts ttsbatch.Options, jobID, gameID, providerKey string) {
	if _, err := batchEngine.Resume(s.bgCtx, opts, jobID); err != nil {
		s.logTTSBatchError(gameID, providerKey, jobID, err)
		s.noteFailure("tts", err)
		return
	}
	s.noteSuccess("tts")
}

// logTTSBatchError records a backfill failure against the campaign and the provider
// it was submitted to, naming the job when the failure came after submission.
func (s *Service) logTTSBatchError(gameID, providerKey, jobID string, err error) {
	event := map[string]any{
		"game":     gameID,
		"provider": providerKey,
		"error":    err.Error(),
	}
	if jobID != "" {
		event["job"] = jobID
	}
	trace.OrNil(s.logger).Event("media.tts.batch_error", event)
}

// ResumePendingBatches finishes every unfinished batch job in the background, so
// a job started in an earlier session is collected on the next launch without
// the user waiting. It is safe to call once at startup; a provider with no batch
// API makes it a no-op.
func (s *Service) ResumePendingBatches(ctx context.Context) {
	cfg := s.configMgr.Get()
	client, err := s.ttsClientFor(cfg.Media.TTS)
	if err != nil {
		return
	}
	batchClient, ok := client.(media.BatchTTSClient)
	if !ok {
		return
	}

	providerKey := ""
	if key, ok := media.TTSKeyFor(cfg.Media.TTS); ok {
		providerKey = string(key)
	}

	games, err := s.ListGames(ctx)
	if err != nil {
		return
	}
	for _, game := range games {
		store, err := s.store(game.ID)
		if err != nil {
			continue
		}
		jobs, err := store.ListTTSJobs(game.ID)
		if err != nil {
			continue
		}
		for _, job := range jobs {
			if !batchJobActive(job) {
				continue
			}
			// A job belongs to the provider that created it, and a different
			// provider's client cannot poll it; leave it for when that provider is
			// selected again.
			if providerKey != "" && job.Provider != "" && job.Provider != providerKey {
				continue
			}
			opts := ttsbatch.Options{GameID: game.ID, Provider: job.Provider, Model: job.Model}
			engine := ttsbatch.New(batchClient, media.NewContentCache(s.resolver.CacheDir()), store)
			engine.SetOpusBitrate(cfg.OpusBitrate())
			trace.OrNil(s.logger).Event("media.tts.batch_resumed", map[string]any{
				"game": game.ID,
				"job":  job.ID,
			})
			s.goBackground(func() {
				if _, err := engine.Resume(s.bgCtx, opts, job.ID); err != nil {
					trace.OrNil(s.logger).Event("media.tts.batch_error", map[string]any{
						"game":  game.ID,
						"job":   job.ID,
						"error": err.Error(),
					})
					s.noteFailure("tts", err)
					return
				}
				s.noteSuccess("tts")
			})
		}
	}
}

// batchJobActive reports whether a job is still worth working on, so a start is
// not duplicated and a launch knows to resume it.
func batchJobActive(job storage.TTSJob) bool {
	switch job.Status {
	case "queued", "processing", "processed", "downloading", "storing", "submitted", "pending", "running":
		return true
	case "completed", "succeeded":
		// A job recorded as finished but short of its request count never stored
		// everything — an output file that arrived empty, or a download that was
		// interrupted — so it is still worth resuming.
		return job.RequestCount > 0 && job.Completed+len(job.FailedKeys) < job.RequestCount
	default:
		return false
	}
}

// activeBatchJob returns a campaign's in-flight batch job, if one exists.
func activeBatchJob(store *storage.Store, gameID string) (*storage.TTSJob, bool) {
	jobs, err := store.ListTTSJobs(gameID)
	if err != nil {
		return nil, false
	}
	for i := range jobs {
		if batchJobActive(jobs[i]) {
			return &jobs[i], true
		}
	}
	return nil, false
}

// cancelBatchJobs stops a campaign's in-flight batch synthesis, best-effort, so a
// restart does not leave the provider working on narration that is about to be
// discarded. A job belongs to the provider that created it, so only jobs matching
// the selected provider are cancelled; a provider that cannot be reached, or a
// job from a provider that is no longer selected, is left for the reset to
// remove.
func (s *Service) cancelBatchJobs(ctx context.Context, gameID string) {
	store, err := s.store(gameID)
	if err != nil {
		return
	}
	jobs, err := store.ListTTSJobs(gameID)
	if err != nil {
		return
	}

	anyActive := false
	for i := range jobs {
		if batchJobActive(jobs[i]) {
			anyActive = true
			break
		}
	}
	if !anyActive {
		return
	}

	cfg := s.configMgr.Get()
	client, err := s.ttsClientFor(cfg.Media.TTS)
	if err != nil {
		return
	}
	batchClient, ok := client.(media.BatchTTSClient)
	if !ok {
		return
	}

	providerKey := ""
	if key, ok := media.TTSKeyFor(cfg.Media.TTS); ok {
		providerKey = string(key)
	}

	for i := range jobs {
		job := jobs[i]
		if !batchJobActive(job) {
			continue
		}
		if providerKey != "" && job.Provider != "" && job.Provider != providerKey {
			continue
		}
		if err := batchClient.CancelBatch(ctx, media.BatchJobHandle{ID: job.ID}); err != nil {
			trace.OrNil(s.logger).Event("media.tts.batch_cancel_error", map[string]any{
				"game":  gameID,
				"job":   job.ID,
				"error": err.Error(),
			})
		}
	}
}

// ttsBatchJobDTO maps a job row to the wire shape.
func ttsBatchJobDTO(job storage.TTSJob) TTSBatchJobDTO {
	return TTSBatchJobDTO{
		ID:           job.ID,
		Provider:     job.Provider,
		Model:        job.Model,
		Status:       job.Status,
		RequestCount: job.RequestCount,
		Completed:    job.Completed,
		FailedKeys:   job.FailedKeys,
		LastError:    job.LastError,
	}
}

// PlayTurnAudio synthesizes any beat the turn has not already cached and plays
// the whole turn in order. Clips are content-addressed, so a replay is instant.
// It takes no context on purpose: narration belongs to the application, so the
// request that asked for it finishing must not silence it. StopAudio ends it.
func (s *Service) PlayTurnAudio(gameID string, turnNumber int, force ...bool) error {
	player := s.audioPlayer()
	if player == nil || !player.Available() {
		return playback.ErrUnavailable
	}

	if _, err := s.findTurn(gameID, turnNumber); err != nil {
		return err
	}

	s.setPlaybackOwner(ownerDevice)
	s.broadcastAudioStatus(AudioStatusDTO{Available: true, Playing: true, Turn: turnNumber, Segment: -1, Owner: ownerDevice})
	player.SetVolume(s.configMgr.Get().Media.TTS.MasterVolume)
	s.setAudioCurrent(turnNumber, -1)
	return player.PlayQueue(s.turnClipStream(gameID, turnNumber, len(force) > 0 && force[0]))
}

// turnClipStream yields a turn's clips in order, synthesizing each one as its
// predecessor is consumed, so playback starts on the first completed clip instead
// of waiting for the whole turn. The stream is deliberately not bound to the
// caller's context: a request that triggered narration finishing must not silence
// it, and the queue ends when the channel closes or the player is stopped.
func (s *Service) turnClipStream(gameID string, turnNumber int, force bool) <-chan string {
	clips := make(chan string)

	turn, err := s.findTurn(gameID, turnNumber)
	if err != nil {
		close(clips)
		return clips
	}

	go func() {
		defer close(clips)
		ctx := context.Background()
		s.emitTurnClips(ctx, gameID, *turn, force, func(path string) {
			clips <- path
		}, nil)
	}()

	return clips
}

// PlaySegmentAudio plays one beat, which is what a speaker chip triggers.
func (s *Service) PlaySegmentAudio(ctx context.Context, gameID string, turnNumber, segmentIndex int, force ...bool) error {
	player := s.audioPlayer()
	if player == nil || !player.Available() {
		return playback.ErrUnavailable
	}

	clips, err := s.GetSegmentClips(ctx, gameID, turnNumber, segmentIndex, force...)
	if err != nil {
		return err
	}

	s.setPlaybackOwner(ownerDevice)
	s.broadcastAudioStatus(AudioStatusDTO{Available: true, Playing: true, Turn: turnNumber, Segment: segmentIndex, Owner: ownerDevice})
	player.SetVolume(s.configMgr.Get().Media.TTS.MasterVolume)
	s.setAudioCurrent(turnNumber, segmentIndex)
	return player.PlayFiles(clips)
}

// voiceFor resolves a speaker entity's configured voice, if it has one.
func (s *Service) voiceFor(gameID string) func(speakerID string) *entity.VoiceConfig {
	store, err := s.store(gameID)
	if err != nil {
		return nil
	}

	return func(speakerID string) *entity.VoiceConfig {
		return harness.ResolveSpeakerVoice(store, speakerID)
	}
}

// voiceProfileDTO presents an entity's voice as the config-shaped profile the
// client edits, so the sheet can show how a character sounds.
func voiceProfileDTO(voice *entity.VoiceConfig) *config.VoiceProfile {
	if voice == nil || strings.TrimSpace(voice.VoiceID) == "" {
		return nil
	}
	return &config.VoiceProfile{
		ID:         voice.VoiceID,
		Name:       voice.VoiceID,
		VoiceID:    voice.VoiceID,
		Provider:   voice.Provider,
		Pitch:      voice.Pitch,
		SpeechRate: voice.SpeechRate,
	}
}

// defaultTTSPreviewText is long enough to expose cadence, pitch, and pacing
// differences between voice profiles during a probe.
const defaultTTSPreviewText = "Local RPG can use a wide range of voices to bring life to your characters, NPCs, and story narration."

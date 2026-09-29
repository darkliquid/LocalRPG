# Sentence-Scoped Audio and Streamed Narration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a clip and its cache key the same unit everywhere — one sentence of reduced text, or the whole text when splitting is unsafe — so a segment's audio is an ordered list of clips, streamed playback needs no heuristic about scope, and the concatenation path is deleted rather than kept beside the list approach.

**Architecture:** `pkg/media` gains a single definition of what a clip reads (`clipUnits`) plus `SynthesizeSegmentClips`/`SegmentClipKeys`/`SynthesizeSegments ([][]string)`; `concatenateSentences` and the whole-segment key disappear. `pkg/gui` serves clips content-addressed at `GET /api/audio/clip/{key}`, exposes `SegmentDTO.audio_urls`, and turns segment audio regeneration into `POST /api/game/{id}/turn/{n}/segment/{i}/audio`; the turn session opens one playback queue before generation, the sentence streamer appends what it synthesizes, and the finalise pass appends only clips the played set has not already sent. `pkg/scene`/`pkg/export` consume clip lists (one file per unit in the web bundle, one video input per unit). The frontend plays `audio_urls` and honours `speech` events.

**Tech Stack:** Go 1.27 (stdlib `testing`), React 19 + TypeScript (strict, `tsc --noEmit`), Opus via `pkg/media/opus`, `mise run test` as the gate.

**Spec:** `docs/superpowers/specs/2026-09-29-sentence-scoped-audio-and-streamed-narration-design.md`

## Global Constraints

- Module path `github.com/darkliquid/localrpg`; use `interface{}`, never `any`; `go vet` must stay clean.
- Tests use the standard library only (`testing`, `t.TempDir()`); no testify. Errors wrapped `fmt.Errorf("...: %w", err)`.
- Every clip is stored as one file named `<key>.opus` under the audio cache; a key is 64 lowercase hex characters.
- No audio path is ever recorded in `history.jsonl`; the cache key stays a pure function of speaker, voice, prosody, and text.
- Reuse `media.ComputeAudioCacheKeyForVoice` and `entity.Slugify`/`entity.WikilinkTarget` rather than local variants.
- Route and DTO changes must be reflected in `Service`, `pkg/gui/server.go`, `frontend/src/types.ts`, and `frontend/src/api/client.ts` together.
- `frontend/src` has `strict`, `noUnusedLocals`, `noUnusedParameters`: no unused imports may survive.
- Commit style: Conventional Commits with a scope, subject under 72 chars.
- Do not touch `index.db`, Markdown, or `history.jsonl` shapes.

---

## File Map

| File | Responsibility after this change |
| --- | --- |
| `pkg/media/tts.go` | `clipUnits` is the one definition of a clip's text; `SynthesizeSegmentClips`, `SegmentClipKeys`, `SynthesizeSegments`, `SynthesizeProvisional`, `CountUncached`; no concatenation |
| `pkg/media/cache.go` | unchanged key functions; `ClipKeyForPath` (new, next to the keys) |
| `pkg/gui/service.go` | `GetSegmentClips`, `PlayTurnAudio`/`PlaySegmentAudio` over lists, `ClipPath`, `segmentClipKeys`, `segmentDTOs` with `audio_urls`, turn-session playback wiring |
| `pkg/gui/types.go` | `SegmentDTO.AudioURLs`, `TurnEvent` speech fields |
| `pkg/gui/server.go` | `GET /api/audio/clip/{key}`, `POST .../segment/{i}/audio`; the GET segment-audio case is deleted |
| `pkg/gui/streaming_tts.go` | `provisionalSpeech` + `emit` callback |
| `pkg/gui/turn_audio.go` | (new) `clipSet`, `turnAudioPlan`, `speechEvent`, clip URL/path helpers |
| `pkg/scene/scene.go` | `Beat.AudioPaths []string` |
| `pkg/scene/compile.go` | `SpeechResolver.SegmentAudio` returns a list; `AudioDuration` sums it |
| `pkg/export/script.go` | `speechResolver.SegmentAudio` sums clip durations |
| `pkg/export/web.go` | `webBeat.Audio []string`, one copied file per unit |
| `pkg/export/video.go` | one ffmpeg input per unit |
| `frontend/src/types.ts` | `TurnSegment.audio_urls`, `TurnEvent` speech fields |
| `frontend/src/api/client.ts` | `regenerateSegmentAudio` |
| `frontend/src/lib/audio.ts` | (new) `clipKeyFromURL`, `segmentClipURLs` |
| `frontend/src/hooks/useSegmentPlayback.ts` | flattened clip playback, `skipKeys`, regeneration via POST |
| `frontend/src/hooks/useStreamedSpeech.ts` | (new) plays `speech` events while prose streams |
| `frontend/src/components/{TurnSegments,ChronicleView,StoryTheater}.tsx` | `audio_urls`, `skipAudioKeys` plumbing |
| `frontend/src/App.tsx` | `speech` event handling, played-set handoff on `turn` |

---

### Task 1: Media: one unit, one clip, one key

**Files:**
- Modify: `pkg/media/tts.go:172-341` (synthesis surface), `pkg/media/tts.go:343-359` (`CountUncached`)
- Modify: `pkg/media/cache.go` (add `ClipKeyForPath`)
- Test: `pkg/media/sentence_test.go`, `pkg/media/tts_test.go`, `pkg/media/speakable_test.go`, `pkg/media/uncached_test.go`

**Interfaces:**
- Consumes: `prepareSegment`, `sentencesFor`, `SynthesizeUtteranceForce`, `ComputeAudioCacheKeyForVoice`, `cachedClip` (all existing).
- Produces:
  - `func (p *TTSPipeline) SynthesizeSegmentClips(ctx context.Context, segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(string) *entity.VoiceConfig, force bool) ([]string, error)`
  - `func (p *TTSPipeline) SegmentClipKeys(segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(string) *entity.VoiceConfig) ([]string, error)`
  - `func (p *TTSPipeline) SynthesizeSegments(ctx context.Context, segments []entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(string) *entity.VoiceConfig) ([][]string, error)`
  - `func (p *TTSPipeline) SynthesizeProvisional(ctx context.Context, kind, speakerID, text string, voice *entity.VoiceConfig) (string, error)` (signature unchanged)
  - `func ClipKeyForPath(path string) string`
- Deletes: `SynthesizeSegment`, `SynthesizeSegmentForce`, `concatenateSentences`.

- [ ] **Step 1: Write the failing tests**

Replace the concatenation-era tests in `pkg/media/sentence_test.go` (`TestEvictingTheSegmentClipReconcatenatesFromSentenceClips`, `TestMultiSentenceSegmentReusesSentenceClips`, `TestSingleSentenceSegmentTakesTheDirectPath`) with:

```go
func TestSegmentClipKeysNamesTheClipsSynthesisWrites(t *testing.T) {
	client := &recordingTTSClient{}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))
	voice := &entity.VoiceConfig{VoiceID: "v1", Pitch: 1, SpeechRate: 1}
	segment := entity.TurnSegment{Kind: entity.SegmentNarration, Text: "One. Two."}

	keys, err := pipeline.SegmentClipKeys(segment, voice, nil)
	if err != nil {
		t.Fatalf("SegmentClipKeys: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("keys = %d, want one per sentence", len(keys))
	}

	clips, err := pipeline.SynthesizeSegmentClips(context.Background(), segment, voice, nil, false)
	if err != nil {
		t.Fatalf("SynthesizeSegmentClips: %v", err)
	}
	if len(clips) != len(keys) {
		t.Fatalf("clips = %d, keys = %d", len(clips), len(keys))
	}
	for i, clip := range clips {
		if got := ClipKeyForPath(clip); got != keys[i] {
			t.Errorf("clip %d is named %q, want the key %q of its unit", i, got, keys[i])
		}
	}
	if client.calls != 2 {
		t.Errorf("calls = %d, want one per sentence", client.calls)
	}
}

func TestStreamedSentenceIsReusedByTheFinalSegment(t *testing.T) {
	client := &recordingTTSClient{}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))
	voice := &entity.VoiceConfig{VoiceID: "v1", Pitch: 1, SpeechRate: 1}

	if _, err := pipeline.SynthesizeProvisional(context.Background(), entity.SegmentNarration, "", "The hall is quiet.", voice); err != nil {
		t.Fatalf("SynthesizeProvisional: %v", err)
	}

	segment := entity.TurnSegment{Kind: entity.SegmentNarration, Text: "The hall is quiet. Garrick nods."}
	clips, err := pipeline.SynthesizeSegmentClips(context.Background(), segment, voice, nil, false)
	if err != nil {
		t.Fatalf("SynthesizeSegmentClips: %v", err)
	}
	if len(clips) != 2 {
		t.Fatalf("clips = %d, want one per sentence", len(clips))
	}
	// One call for the streamed sentence, one for the second: the first is reused.
	if client.calls != 2 {
		t.Errorf("calls = %d, want the streamed sentence reused", client.calls)
	}

	again, err := pipeline.SynthesizeSegmentClips(context.Background(), segment, voice, nil, false)
	if err != nil {
		t.Fatalf("second SynthesizeSegmentClips: %v", err)
	}
	if client.calls != 2 {
		t.Errorf("calls = %d, want both clips cached", client.calls)
	}
	if len(again) != 2 || again[0] != clips[0] || again[1] != clips[1] {
		t.Errorf("second read = %#v, want the cached clips %#v", again, clips)
	}
}

func TestNoClipIsWrittenUnderTheWholeSegmentKey(t *testing.T) {
	client := &recordingTTSClient{}
	cache := NewContentCache(t.TempDir())
	pipeline := NewTTSPipeline(client, cache)
	voice := &entity.VoiceConfig{VoiceID: "v1", Pitch: 1, SpeechRate: 1}
	segment := entity.TurnSegment{Kind: entity.SegmentNarration, Text: "One. Two."}

	if _, err := pipeline.SynthesizeSegmentClips(context.Background(), segment, voice, nil, false); err != nil {
		t.Fatalf("SynthesizeSegmentClips: %v", err)
	}

	base := ComputeAudioCacheKeyForVoice(narratorSpeaker, voice, segment.Text)
	if _, err := os.Stat(filepath.Join(cache.Subdir("audio"), base+".opus")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a clip is stored under the whole-segment key %q; nothing concatenates any more", base)
	}
}

func TestSynthesizeSegmentClipsSkipsAFailedUnit(t *testing.T) {
	client := &testTTSClient{onSynthesize: func(_ context.Context, text string, _ *entity.VoiceConfig) ([]byte, error) {
		if text == "Two." {
			return nil, errors.New("provider said no")
		}
		return GenerateToneWAV(440, 0.02), nil
	}}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))

	clips, err := pipeline.SynthesizeSegmentClips(context.Background(), entity.TurnSegment{
		Kind: entity.SegmentNarration, Text: "One. Two.",
	}, nil, nil, false)
	if err != nil {
		t.Fatalf("a single failed unit must not fail the segment: %v", err)
	}
	if len(clips) != 1 {
		t.Fatalf("clips = %d, want the one unit that succeeded", len(clips))
	}
}
```

Keep `TestSynthesizeProvisionalSharesTheFinalSegmentCacheKey` and `TestMarkdownAwareClientIsNotSplit`, changing only the call to `SynthesizeSegmentClips` and asserting one clip for the Markdown-aware client:

```go
func TestMarkdownAwareClientIsNotSplit(t *testing.T) {
	client := &markdownTTSClient{}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))
	voice := &entity.VoiceConfig{VoiceID: "v1"}

	clips, err := pipeline.SynthesizeSegmentClips(context.Background(), entity.TurnSegment{
		Kind: entity.SegmentNarration, Text: "A *long* breath. Then another.",
	}, voice, nil, false)
	if err != nil {
		t.Fatalf("SynthesizeSegmentClips: %v", err)
	}
	if len(clips) != 1 {
		t.Fatalf("clips = %d, want the whole segment as one unit", len(clips))
	}
	if client.calls != 1 {
		t.Fatalf("calls = %d, want the whole segment synthesized once", client.calls)
	}
}
```

Add `errors` to the `sentence_test.go` imports.

- [ ] **Step 2: Run the media tests to watch them fail**

Run: `go test ./pkg/media/ -run 'SegmentClipKeys|SynthesizeSegmentClips|WholeSegmentKey|FailedUnit|MarkdownAware' -v`
Expected: compile failure, `pipeline.SynthesizeSegmentClips undefined`.

- [ ] **Step 3: Implement the media surface**

In `pkg/media/tts.go`, replace `SynthesizeSegments`…`concatenateSentences` (lines 172-341) with:

```go
// SynthesizeSegments renders every segment as a list of clips, skipping a segment
// that reduces to no speakable text rather than treating it as a failure. Cached
// clips are reused; audio references stay out of the turn record because the cache
// key is a pure function of speaker, voice, prosody, and text.
func (p *TTSPipeline) SynthesizeSegments(ctx context.Context, segments []entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig) ([][]string, error) {
	clips := make([][]string, 0, len(segments))

	for _, segment := range segments {
		list, err := p.SynthesizeSegmentClips(ctx, segment, narratorVoice, voiceFor, false)
		if errors.Is(err, ErrNoSpeakableText) {
			continue
		}
		if err != nil {
			return nil, err
		}
		clips = append(clips, list)
	}

	return clips, nil
}

// clipUnits resolves a segment to the units synthesis reads: one sentence of the
// text this client is sent, or the whole text when splitting could corrupt it. It
// is the single definition of what a clip reads, shared by key computation and
// synthesis so the two can never disagree about which clip a segment needs.
func (p *TTSPipeline) clipUnits(segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig) (speakerID string, voice *entity.VoiceConfig, units []string, err error) {
	speakerID, voice, spoken := p.prepareSegment(segment, narratorVoice, voiceFor)
	if strings.TrimSpace(spoken) == "" {
		return speakerID, voice, nil, ErrNoSpeakableText
	}
	if spoken != segment.Text {
		logger := trace.OrNil(p.logger)
		logger.Event("media.tts.reduced", map[string]interface{}{
			"chars_raw":    len([]rune(segment.Text)),
			"chars_spoken": len([]rune(spoken)),
		})
	}
	return speakerID, voice, p.sentencesFor(spoken), nil
}

// SegmentClipKeys reports the keys a segment's clips will have, without
// synthesizing anything, so a caller can name a clip before it exists.
func (p *TTSPipeline) SegmentClipKeys(segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig) ([]string, error) {
	speakerID, voice, units, err := p.clipUnits(segment, narratorVoice, voiceFor)
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(units))
	for _, unit := range units {
		keys = append(keys, ComputeAudioCacheKeyForVoice(speakerID, voice, unit))
	}
	return keys, nil
}

// SynthesizeSegmentClips renders a segment as one clip per unit, in order. A unit
// that fails is skipped so its neighbours still play; only a segment that produced
// no clip at all reports the failure.
func (p *TTSPipeline) SynthesizeSegmentClips(ctx context.Context, segment entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig, force bool) ([]string, error) {
	speakerID, voice, units, err := p.clipUnits(segment, narratorVoice, voiceFor)
	if err != nil {
		return nil, err
	}

	clips := make([]string, 0, len(units))
	var firstErr error
	for _, unit := range units {
		clip, err := p.SynthesizeUtteranceForce(ctx, speakerID, voice, unit, force)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		clips = append(clips, clip)
	}
	if len(clips) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return clips, nil
}

// SynthesizeProvisional renders a sentence of prose before the turn's final
// segments exist. It reads the same unit the finaliser will, so when the finished
// segment's text contains that sentence the final synthesis is a cache hit rather
// than a second provider call. Text that reduces to nothing returns
// ErrNoSpeakableText.
func (p *TTSPipeline) SynthesizeProvisional(ctx context.Context, kind, speakerID, text string, voice *entity.VoiceConfig) (string, error) {
	segment := entity.TurnSegment{Kind: kind, SpeakerID: speakerID, Text: text}
	speaker, resolved, spoken := p.prepareSegment(segment, voice, nil)
	if strings.TrimSpace(spoken) == "" {
		return "", ErrNoSpeakableText
	}
	return p.SynthesizeUtteranceForce(ctx, speaker, resolved, spoken, false)
}
```

Leave `sentencesFor` and `markdownPreserved` in place; delete `concatenateSentences`. Rewrite `CountUncached` so a segment counts as cached only when every unit's clip exists:

```go
// CountUncached reports how many speakable segments already have a clip and how
// many would need synthesis, so a bulk operation can warn before spending money
// on a metered provider. It mutates nothing.
func (p *TTSPipeline) CountUncached(segments []entity.TurnSegment, narratorVoice *entity.VoiceConfig, voiceFor func(speakerID string) *entity.VoiceConfig) (cached, uncached int) {
	for _, segment := range segments {
		keys, err := p.SegmentClipKeys(segment, narratorVoice, voiceFor)
		if err != nil || len(keys) == 0 {
			continue
		}
		complete := true
		for _, key := range keys {
			if _, ok := p.cachedClip(key); !ok {
				complete = false
				break
			}
		}
		if complete {
			cached++
		} else {
			uncached++
		}
	}
	return cached, uncached
}
```

In `pkg/media/cache.go`, beside the key functions:

```go
// ClipKeyForPath names the key a stored clip was written under: the file name is
// the key, so nothing else has to be consulted to name the audio it holds.
func ClipKeyForPath(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}
```

Add `"strings"` to `cache.go` imports.

- [ ] **Step 4: Update the remaining media callers in tests**

Mechanical rewrites, all to `SynthesizeSegmentClips(...)[0]` semantics:
- `pkg/media/tts_test.go:157,165,255` (`SynthesizeSegments` now returns `[][]string`: assert `len(first) == 2` and compare `first[1][0] != second[1][0]`).
- `pkg/media/tts_test.go:274-297` (`SynthesizeSegment` → `SynthesizeSegmentClips`, keeping the voice assertions).
- `pkg/media/speakable_test.go:59,72,86,99,113,129,134`: `SynthesizeSegment` → `SynthesizeSegmentClips`; `TestSynthesizeSegmentsSkipsUnspeakableBeats` keeps asserting one list for two segments.
- `pkg/media/uncached_test.go:27`: `SynthesizeSegment` → `SynthesizeSegmentClips`.

- [ ] **Step 5: Run the media tests**

Run: `go test ./pkg/media/ -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/media
git commit -m "refactor(media): make a clip and its key one sentence of text"
```

---

### Task 2: GUI: content-addressed clips and `audio_urls`

**Files:**
- Modify: `pkg/gui/types.go:123-134` (`SegmentDTO`), `pkg/gui/types.go:491-509` (`TurnEvent`)
- Modify: `pkg/gui/service.go:367-414` (`segmentDTOs`), `pkg/gui/service.go:1013-1055` (`turnDTO`), `pkg/gui/service.go:1467-1477` (warm-up), `pkg/gui/service.go:1820-1863` (`GetSegmentClips`), `:1994-2045` (`PlayTurnAudio`/`PlaySegmentAudio`)
- Modify: `pkg/gui/turn_audio.go` (new: clip URL/path helpers)
- Modify: `pkg/gui/server.go:406-498` (routes), `:1101-1128` (audio routes)
- Test: `pkg/gui/service_test.go`, `pkg/gui/turn_audio_force_test.go`, `pkg/gui/server_clip_cache_test.go` (new clip-route tests)

**Interfaces:**
- Consumes: `media.SynthesizeSegmentClips`, `media.SegmentClipKeys`, `media.ClipKeyForPath`, `media.ContentCache`.
- Produces:
  - `func (s *Service) GetSegmentClips(ctx context.Context, gameID string, turnNumber, segmentIndex int, force ...bool) ([]string, error)`
  - `func (s *Service) ClipPath(key string) (string, bool)`
  - `func clipURL(key string) string`
  - `func clipURLs(clips []string) []string`
  - `SegmentDTO.AudioURLs []string` (`json:"audio_urls,omitempty"`)
  - `TurnEvent.Index int`, `TurnEvent.AudioKey`, `TurnEvent.AudioURL string`
- Deletes: `Service.GetSegmentAudio`, the `AudioURL`/`AudioKey` DTO fields, the DTO-side `ComputeAudioCacheKeyForVoice` call.

- [ ] **Step 1: Write the failing tests**

New file `pkg/gui/clip_route_test.go`:

```go
package gui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/media"
)

func TestClipRouteServesAStoredClip(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)

	clips, err := svc.GetSegmentClips(context.Background(), gameID, 1, 1)
	if err != nil || len(clips) == 0 {
		t.Fatalf("GetSegmentClips = %#v, %v", clips, err)
	}
	key := media.ClipKeyForPath(clips[0])

	server := NewServer(svc, http.NotFoundHandler())
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest("GET", "/api/audio/clip/"+key, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() == 0 {
		t.Error("expected the clip's bytes")
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "ogg") {
		t.Errorf("Content-Type = %q, want an Ogg type", ct)
	}
}

func TestClipRouteRejectsUnknownAndMalformedKeys(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)
	server := NewServer(svc, http.NotFoundHandler())

	for _, path := range []string{
		"/api/audio/clip/" + strings.Repeat("0", 64),
		"/api/audio/clip/nothex",
		"/api/audio/clip/" + strings.Repeat("a", 63),
		"/api/audio/clip/" + strings.Repeat("F", 64),
	} {
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", path, rec.Code)
		}
	}
}

func TestClipPathRefusesAKeyThatIsNotOne(t *testing.T) {
	_, svc := setupTestGame(t)
	for _, key := range []string{"", "..", "../../etc/passwd", "abc/def", strings.Repeat("0", 64) + "/x"} {
		if path, ok := svc.ClipPath(key); ok {
			t.Errorf("ClipPath(%q) = %q, want a refusal", key, path)
		}
	}
}
```

Replace `pkg/gui/turn_audio_force_test.go`'s `TestSegmentAudioRouteWithForce` with regeneration assertions:

```go
func TestRegenerateSegmentAudioReturnsClipURLs(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest("POST", "/api/game/"+gameID+"/turn/1/segment/1/audio", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "/api/audio/clip/") {
		t.Errorf("body = %s, want content-addressed clip URLs", rec.Body.String())
	}
}
```

In `pkg/gui/service_test.go`, rename `TestGetSegmentAudioSynthesizesAndCaches` to `TestGetSegmentClipsSynthesizesAndCaches`:

```go
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
```

`mustStore` is a new one-line helper in `service_test.go`:

```go
func mustStore(t *testing.T, svc *Service, gameID string) *storage.Store {
	t.Helper()
	store, err := svc.store(gameID)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return store
}
```

- [ ] **Step 2: Run them to watch them fail**

Run: `go test ./pkg/gui/ -run 'ClipRoute|ClipPath|Regenerate|GetSegmentClips|ClipURLs' -v`
Expected: compile failure, `svc.GetSegmentClips undefined`.

- [ ] **Step 3: Implement the gui surface**

New file `pkg/gui/turn_audio.go`:

```go
package gui

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/darkliquid/localrpg/pkg/media"
)

// clipKeyPattern is the whole shape of a clip key: a content-addressed name is
// 64 lowercase hex characters, so no client-supplied path can reach the cache.
var clipKeyPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// clipURL names a clip by its key, which is what makes a clip's URL stable
// without a turn or a segment index.
func clipURL(key string) string {
	return "/api/audio/clip/" + key
}

// clipURLs names each clip in the order the segment plays them.
func clipURLs(clips []string) []string {
	urls := make([]string, 0, len(clips))
	for _, clip := range clips {
		urls = append(urls, clipURL(media.ClipKeyForPath(clip)))
	}
	return urls
}
```

In `pkg/gui/service.go`:

```go
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
```

Replace `segmentDTOs` (drop `turnNumber`, take a key resolver):

```go
func segmentDTOs(segments []entity.TurnSegment, gameID string, clipKeys func(entity.TurnSegment) []string, resolve func(string) string, voiceFor func(string) *entity.VoiceConfig) []SegmentDTO {
	dtos := make([]SegmentDTO, 0, len(segments))
	for _, segment := range segments {
		text := resolveWikilinks(segment.Text, resolve)
		dto := SegmentDTO{
			Kind:      segment.Kind,
			Speaker:   segment.Speaker,
			SpeakerID: segment.SpeakerID,
			Text:      text,
			CheckRef:  segment.CheckRef,
			Player:    segment.Player,
			// The reading estimate is the same one the exports pace with, so the
			// app and a rendered bundle hold a line for the same length of time.
			Duration: scene.ReadingDuration(text).Seconds(),
		}
		if segment.Kind == "speech" {
			refID := segment.SpeakerID
			if refID == "" && resolve != nil {
				refID = resolve(segment.Speaker)
			}
			if refID == "" && segment.Speaker != "" {
				refID = entity.Slugify(segment.Speaker)
			}
			if refID != "" {
				dto.PortraitURL = fmt.Sprintf("/api/game/%s/character/%s/portrait", gameID, refID)
			}
		}
		if clipKeys != nil {
			// The keys come from the pipeline, so the URL a client is handed is the
			// URL of the audio synthesis writes: one sound, one name.
			dto.AudioURLs = clipURLs(clipKeysForSegment(clipKeys, segment))
		}
		dtos = append(dtos, dto)
	}
	return dtos
}

// clipKeysForSegment tolerates a segment with no nameable clip (a line that
// reduces to nothing), which is a normal state rather than an error.
func clipKeysForSegment(clipKeys func(entity.TurnSegment) []string, segment entity.TurnSegment) []string {
	keys := clipKeys(segment)
	if len(keys) == 0 {
		return nil
	}
	return keys
}
```

The `voiceFor` parameter is no longer used by `segmentDTOs`; drop it and update `turnDTO`:

```go
// clipKeyResolver names a segment's clips from the shared pipeline, or nil when
// no TTS provider is configured. Building the pipeline is cheap: a provider loads
// its model at first synthesis, not at construction.
func (s *Service) clipKeyResolver(cfg *config.Config, gameID string) func(entity.TurnSegment) []string {
	if cfg.Media.TTS.Type == "" || cfg.Media.TTS.Type == "disabled" {
		return nil
	}
	pipeline, err := s.audioPipeline()
	if err != nil {
		return nil
	}
	narrator := s.narratorVoiceFor(gameID, cfg)
	voiceFor := s.voiceFor(gameID)
	return func(segment entity.TurnSegment) []string {
		keys, err := pipeline.SegmentClipKeys(segment, narrator, voiceFor)
		if err != nil {
			return nil
		}
		return keys
	}
}
```

and inside `turnDTO`:

```go
	audioAvailable := cfg.Media.TTS.Type != "" && cfg.Media.TTS.Type != "disabled"
```
becomes

```go
	var clipKeys func(entity.TurnSegment) []string
	if cfg.Media.TTS.Type != "" && cfg.Media.TTS.Type != "disabled" {
		clipKeys = s.clipKeyResolver(cfg, gameID)
	}
```
with

```go
		Segments: segmentDTOs(turn.Segments, gameID, clipKeys, func(name string) string {
			return harness.ResolveSpeakerID(store, name)
		}),
```

Replace `GetSegmentAudio` with `GetSegmentClips` (same lookup, list result, unchanged usage recording):

```go
// GetSegmentClips synthesizes one segment on demand and returns its ordered clips,
// reusing every clip the cache already holds.
func (s *Service) GetSegmentClips(ctx context.Context, gameID string, turnNumber, segmentIndex int, force ...bool) ([]string, error) {
	turns, err := s.cachedHistory(gameID)
	if err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}

	var turn *engine.Turn
	for i := range turns {
		if turns[i].Number == turnNumber {
			turn = &turns[i]
			break
		}
	}
	if turn == nil {
		return nil, fmt.Errorf("turn %d not found", turnNumber)
	}
	if segmentIndex < 0 || segmentIndex >= len(turn.Segments) {
		return nil, fmt.Errorf("segment %d out of range for turn %d", segmentIndex, turnNumber)
	}

	cfg := s.configMgr.Get()
	narratorVoice := s.narratorVoiceFor(gameID, cfg)

	pipeline, err := s.audioPipeline()
	if err != nil {
		return nil, err
	}
	isForce := len(force) > 0 && force[0]
	clips, err := pipeline.SynthesizeSegmentClips(ctx, turn.Segments[segmentIndex], narratorVoice, s.voiceFor(gameID), isForce)
	if err != nil {
		s.noteFailure("tts", err)
		return nil, err
	}
	s.noteSuccess("tts")
	// A cache hit reports nothing, so only a real synthesis is recorded.
	if key, ok := media.TTSKeyFor(cfg.Media.TTS); ok {
		if u := pipeline.LastUsage(); u.Characters != 0 || u.InputTokens != 0 || u.OutputTokens != 0 || u.Requests != 0 {
			s.RecordUsage(gameID, turnNumber, "tts", mediaUsage(u, key, cfg.Media.TTS.Model))
		}
	}
	return clips, nil
}
```

`PlayTurnAudio` feeds every clip of every segment through the queue; `PlaySegmentAudio` plays the segment's list:

```go
	clips := make(chan string)
	go func() {
		defer close(clips)
		for i := range turn.Segments {
			list, err := s.GetSegmentClips(ctx, gameID, turnNumber, i, isForce)
			if err != nil {
				continue
			}
			for _, path := range list {
				select {
				case clips <- path:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
```
and
```go
	clips, err := s.GetSegmentClips(ctx, gameID, turnNumber, segmentIndex, force...)
	if err != nil {
		return err
	}

	player.SetVolume(s.configMgr.Get().Media.TTS.MasterVolume)
	return player.PlayFiles(clips)
```

In `pkg/gui/types.go`:

```go
type SegmentDTO struct {
	Kind        string   `json:"kind"`
	Speaker     string   `json:"speaker,omitempty"`
	SpeakerID   string   `json:"speaker_id,omitempty"`
	Text        string   `json:"text"`
	AudioURLs   []string `json:"audio_urls,omitempty"`
	PortraitURL string   `json:"portrait_url,omitempty"`
	CheckRef    string   `json:"check_ref,omitempty"`
	Player      bool     `json:"player,omitempty"`
	Duration    float64  `json:"duration"`
}
```

and in `TurnEvent`:

```go
	Type    string   `json:"type"` // "chunk", "speech", "turn", "tool", "error", or "model_missing"
	...
	// Streamed narration, present when Type is "speech": which unit it is, and the
	// clip that was written for it.
	Index    int    `json:"index,omitempty"`
	AudioKey string `json:"audio_key,omitempty"`
	AudioURL string `json:"audio_url,omitempty"`
```

In `pkg/gui/server.go`, `handleAudioRoutes` gains the clip case:

```go
	case "clip":
		http.NotFound(w, r)

	default:
		if r.Method == http.MethodGet && strings.HasPrefix(action, "clip/") {
			s.serveClip(w, r, strings.TrimPrefix(action, "clip/"))
			return
		}
		http.NotFound(w, r)
	}
}

// serveClip serves one stored clip by its content key. The key names the file and
// nothing else does, so a malformed or unknown key is a 404 rather than a lookup.
func (s *Server) serveClip(w http.ResponseWriter, r *http.Request, key string) {
	path, ok := s.service.ClipPath(key)
	if !ok {
		http.NotFound(w, r)
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	setClipHeaders(w, data)
	_, _ = w.Write(data)
}
```
(Drop the `case "clip": http.NotFound` stub; it is written here only to show the shape — the `default` branch carries the prefix test.)

In `handleGameRoutes` case `"turn"`, delete the GET segment-audio branch at `server.go:460-498` and add the regeneration branch:

```go
		// POST /api/game/{id}/turn/{n}/segment/{i}/audio re-synthesizes one beat
		// and answers with its refreshed clip URLs. Playback itself reads clips by
		// content key, so this is only what a regenerate control needs.
		if r.Method == http.MethodPost && len(parts) == 6 && parts[3] == "segment" && parts[5] == "audio" {
			turnNumber, err := strconv.Atoi(parts[2])
			if err != nil {
				http.Error(w, "invalid turn number", http.StatusBadRequest)
				return
			}
			segmentIndex, err := strconv.Atoi(parts[4])
			if err != nil {
				http.Error(w, "invalid segment index", http.StatusBadRequest)
				return
			}
			clips, err := s.service.GetSegmentClips(r.Context(), gameID, turnNumber, segmentIndex, true)
			switch {
			case errors.Is(err, ErrAudioUnavailable):
				w.WriteHeader(http.StatusNoContent)
			case err != nil:
				writeGameError(w, err)
			default:
				writeJSON(w, map[string]interface{}{"audio_urls": clipURLs(clips)})
			}
			return
		}

		http.NotFound(w, r)
```

Also update the warm-up loop in `TurnSession.Run` (`service.go:1471-1477`) to consume the list:

```go
	} else if t.cfg.Media.TTS.Type != "" && t.cfg.Media.TTS.Type != "disabled" {
		t.service.goBackground(func() {
			for i := range turn.Segments {
				_, _ = t.service.GetSegmentClips(context.Background(), t.gameID, turn.Number, i)
			}
		})
	}
```

- [ ] **Step 4: Run the gui tests**

Run: `go test ./pkg/gui/ -count=1 -run 'Clip|Regenerate|GetSegmentClips|TurnDTO|Play' -v && go vet ./pkg/gui/`
Expected: PASS, vet clean.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui
git commit -m "feat(gui): serve a segment's audio as content-addressed clips"
```

---

### Task 3: GUI: the streamer emits what it synthesizes

**Files:**
- Modify: `pkg/gui/streaming_tts.go` (whole file)
- Test: `pkg/gui/streaming_tts_test.go`

**Interfaces:**
- Consumes: `media.SynthesizeProvisional`, `media.ClipKeyForPath`, `clipURL`.
- Produces:
  - `type provisionalSpeech struct { Index int; Text, AudioKey, AudioURL string }`
  - `func newSentenceStreamer(ctx context.Context, pipeline *media.TTSPipeline, voice *entity.VoiceConfig, logger trace.Logger, workers int, emit func(provisionalSpeech)) *sentenceStreamer`

- [ ] **Step 1: Write the failing test**

```go
func TestSentenceStreamerEmitsOrderedSentences(t *testing.T) {
	client := &fakeSentenceTTSClient{}
	pipeline := media.NewTTSPipeline(client, media.NewContentCache(t.TempDir()))

	var mu sync.Mutex
	var got []provisionalSpeech
	streamer := newSentenceStreamer(context.Background(), pipeline, &entity.VoiceConfig{VoiceID: "v1"}, trace.Nop(), 1, func(speech provisionalSpeech) {
		mu.Lock()
		got = append(got, speech)
		mu.Unlock()
	})

	streamer.Feed("The hall is quiet. Garrick")
	streamer.Feed(" steps inside.")
	streamer.Close()

	if len(got) != 2 {
		t.Fatalf("events = %#v, want the two complete sentences", got)
	}
	if got[0].Text != "The hall is quiet." || got[1].Text != "Garrick steps inside." {
		t.Errorf("texts = %q, %q", got[0].Text, got[1].Text)
	}
	for i, speech := range got {
		if speech.Index != i {
			t.Errorf("event %d carries index %d, want its ordinal", i, speech.Index)
		}
		if speech.AudioKey == "" {
			t.Errorf("event %d names no clip key", i)
		}
		if speech.AudioURL != "/api/audio/clip/"+speech.AudioKey {
			t.Errorf("event %d url = %q, want the clip's content-addressed URL", i, speech.AudioURL)
		}
	}
}

func TestSentenceStreamerWithoutAConsumerIsSafe(t *testing.T) {
	client := &fakeSentenceTTSClient{}
	pipeline := media.NewTTSPipeline(client, media.NewContentCache(t.TempDir()))
	streamer := newSentenceStreamer(context.Background(), pipeline, &entity.VoiceConfig{VoiceID: "v1"}, trace.Nop(), 1, nil)

	streamer.Feed("The hall is quiet.")
	streamer.Close()

	if got := client.callCount(); got != 1 {
		t.Fatalf("calls = %d, want the sentence still synthesized", got)
	}
}
```

Update the existing `TestSentenceStreamerSynthesizesCompleteSentencesOnly` to pass `nil` as the emit callback.

- [ ] **Step 2: Run it to watch it fail**

Run: `go test ./pkg/gui/ -run SentenceStreamer -v`
Expected: compile failure (wrong number of arguments).

- [ ] **Step 3: Implement the streamer**

```go
// provisionalSpeech is one sentence the streamer synthesized: its ordinal within
// the turn and the clip that was written for it. A client plays it while the prose
// is still arriving, and the turn's own clip list is the same audio.
type provisionalSpeech struct {
	Index    int
	Text     string
	AudioKey string
	AudioURL string
}

type sentenceStreamer struct {
	ctx      context.Context
	pipeline *media.TTSPipeline
	voice    *entity.VoiceConfig
	logger   trace.Logger
	queue    chan string
	wg       sync.WaitGroup
	closeOne sync.Once
	mu       sync.Mutex
	buf      strings.Builder
	// emit reports a completed sentence, and next is the monotonic ordinal it
	// carries. Both are optional: a streamer with no consumer still caches audio.
	emit func(provisionalSpeech)
	next uint64
}

// newSentenceStreamer builds a streamer with workers consuming the queue, calling
// emit once per completed sentence. workers below one becomes one; emit may be nil.
func newSentenceStreamer(ctx context.Context, pipeline *media.TTSPipeline, voice *entity.VoiceConfig, logger trace.Logger, workers int, emit func(provisionalSpeech)) *sentenceStreamer {
	if workers < 1 {
		workers = 1
	}
	streamer := &sentenceStreamer{
		ctx:      ctx,
		pipeline: pipeline,
		voice:    voice,
		logger:   trace.OrNil(logger),
		queue:    make(chan string, sentenceQueueDepth),
		emit:     emit,
	}
	for i := 0; i < workers; i++ {
		streamer.wg.Add(1)
		go streamer.worker()
	}
	return streamer
}

// worker synthesizes queued sentences until the queue closes. Narration is
// provisionally read in the narrator voice; dialogue cannot be attributed until
// submit_turn parses the segments, so it is left to the finalise path.
func (s *sentenceStreamer) worker() {
	defer s.wg.Done()
	for sentence := range s.queue {
		path, err := s.pipeline.SynthesizeProvisional(s.ctx, entity.SegmentNarration, "", sentence, s.voice)
		if err != nil {
			s.logger.Event("media.tts.provisional_error", map[string]interface{}{"error": err.Error()})
			continue
		}
		if s.emit == nil || path == "" {
			continue
		}
		key := media.ClipKeyForPath(path)
		s.emit(provisionalSpeech{
			Index:    int(atomic.AddUint64(&s.next, 1) - 1),
			Text:     sentence,
			AudioKey: key,
			AudioURL: clipURL(key),
		})
	}
}
```

`sentenceStreamerFor` gains the emit argument:

```go
func (s *Service) sentenceStreamerFor(ctx context.Context, gameID string, cfg *config.Config, emit func(provisionalSpeech)) *sentenceStreamer {
	if !cfg.TTSStreamSentences() {
		return nil
	}
	if cfg.Media.TTS.Type == "" || cfg.Media.TTS.Type == "disabled" {
		return nil
	}
	pipeline, err := s.audioPipeline()
	if err != nil {
		return nil
	}
	return newSentenceStreamer(ctx, pipeline, s.narratorVoiceFor(gameID, cfg), s.logger, 2, emit)
}
```

Add `"sync/atomic"` to the imports.

In `pkg/gui/service.go`, `Run` passes the emitter (the playback wiring lands in Task 4; for now the emitter only publishes the event):

```go
	streamer := t.service.sentenceStreamerFor(runCtx, t.gameID, t.cfg, func(speech provisionalSpeech) {
		_ = emit(speechEvent(speech))
	})
```

`speechEvent` lives in `pkg/gui/turn_audio.go`:

```go
// speechEvent frames a streamed sentence for a client-authority session, which
// plays the clip while the prose is still arriving.
func speechEvent(speech provisionalSpeech) TurnEvent {
	return TurnEvent{
		Type:     "speech",
		Index:    speech.Index,
		Text:     speech.Text,
		AudioKey: speech.AudioKey,
		AudioURL: speech.AudioURL,
	}
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./pkg/gui/ -run 'SentenceStreamer|SpeechEvent' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui
git commit -m "feat(gui): tell the client which clip each streamed sentence wrote"
```

---

### Task 4: GUI: one queue, one played set, no repeats

**Files:**
- Modify: `pkg/gui/turn_audio.go` (add `clipSet`, `turnAudioPlan`)
- Modify: `pkg/gui/service.go:1413-1484` (`TurnSession.Run`), add `finishTurnAudio`
- Test: `pkg/gui/turn_audio_test.go` (new)

**Interfaces:**
- Consumes: `Service.GetSegmentClips`, `Service.audioPlayer`, `media.ClipKeyForPath`, `playback.Player.PlayQueue`.
- Produces:
  - `type clipSet struct{ mu sync.Mutex; keys map[string]bool }`, `newClipSet()`, `(c *clipSet) take(key string) bool`
  - `type turnAudioPlan struct{ queue chan string; played *clipSet }`, `(a *turnAudioPlan) enqueueClip(key, path string) bool`, `(a *turnAudioPlan) close()`
  - `func (t *TurnSession) finishTurnAudio(ctx context.Context, turn engine.Turn, plan *turnAudioPlan)`

- [ ] **Step 1: Write the failing tests**

New file `pkg/gui/turn_audio_test.go`:

```go
package gui

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/media"
)

func TestTurnAudioPlanSendsEachClipOnce(t *testing.T) {
	plan := &turnAudioPlan{queue: make(chan string, 4), played: newClipSet()}

	if !plan.enqueueClip("k1", "/cache/k1.opus") {
		t.Fatal("the first clip was refused")
	}
	if plan.enqueueClip("k1", "/cache/k1.opus") {
		t.Error("a clip the stream already played was sent twice")
	}
	if got := <-plan.queue; got != "/cache/k1.opus" {
		t.Errorf("queue received %q", got)
	}
}

func TestTurnAudioPlanWithoutAQueueSwallowsClips(t *testing.T) {
	plan := &turnAudioPlan{played: newClipSet()}
	if plan.enqueueClip("k1", "/cache/k1.opus") {
		t.Error("a plan with no queue accepted a clip")
	}
	plan.close()
}

func TestFinishTurnAudioSkipsStreamedClips(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)

	session := &TurnSession{service: svc, gameID: gameID, cfg: svc.configMgr.Get()}
	turn, err := svc.findTurn(gameID, 1)
	if err != nil {
		t.Fatal(err)
	}

	plan := &turnAudioPlan{queue: make(chan string, 8), played: newClipSet()}
	// The stream already played the first segment's clip.
	played, err := svc.GetSegmentClips(context.Background(), gameID, 1, 0)
	if err != nil || len(played) == 0 {
		t.Fatalf("GetSegmentClips = %#v, %v", played, err)
	}
	plan.enqueueClip(media.ClipKeyForPath(played[0]), played[0])

	session.finishTurnAudio(context.Background(), *turn, plan)
	plan.close()

	remaining := drainClips(plan.queue)
	if len(remaining) != 1 {
		t.Fatalf("remaining = %#v, want only the clip the stream had not played", remaining)
	}
	if media.ClipKeyForPath(remaining[0]) == media.ClipKeyForPath(played[0]) {
		t.Errorf("the streamed clip %q was queued a second time", remaining[0])
	}
}

func drainClips(queue <-chan string) []string {
	var clips []string
	for clip := range queue {
		clips = append(clips, clip)
	}
	return clips
}

func TestSpeechEventNamesTheClip(t *testing.T) {
	event := speechEvent(provisionalSpeech{Index: 3, Text: "One.", AudioKey: "abc", AudioURL: "/api/audio/clip/abc"})
	if event.Type != "speech" || event.Index != 3 || event.Text != "One." {
		t.Errorf("event = %+v", event)
	}
	if event.AudioKey != "abc" || event.AudioURL != "/api/audio/clip/abc" {
		t.Errorf("event = %+v, want the clip named", event)
	}
}
```

- [ ] **Step 2: Run them to watch them fail**

Run: `go test ./pkg/gui/ -run 'TurnAudioPlan|FinishTurnAudio|SpeechEvent' -v`
Expected: compile failure, `turnAudioPlan undefined`.

- [ ] **Step 3: Implement the plan and the wiring**

Append to `pkg/gui/turn_audio.go`:

```go
// clipSet records which clips have already been sent to the player. With one clip
// per unit it is exact: a unit heard is a unit skipped, and no unit is heard twice.
type clipSet struct {
	mu   sync.Mutex
	keys map[string]bool
}

func newClipSet() *clipSet {
	return &clipSet{keys: map[string]bool{}}
}

// take records a key and reports whether it was new.
func (c *clipSet) take(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.keys[key] {
		return false
	}
	c.keys[key] = true
	return true
}

// turnAudioPlan is a turn's playback state: the queue the streamer appends to when
// application playback is running, and the set of clips it has already sent. A plan
// with no queue is a client-authority or silent session, where clips are warmed but
// not played here.
type turnAudioPlan struct {
	queue  chan string
	played *clipSet
}

// enqueueClip sends one clip to the player once, reporting whether it was sent.
func (a *turnAudioPlan) enqueueClip(key, path string) bool {
	if a == nil || a.queue == nil || path == "" {
		return false
	}
	if !a.played.take(key) {
		return false
	}
	select {
	case a.queue <- path:
		return true
	default:
		// The player is behind by more than a queue's depth; a dropped clip is
		// heard late rather than blocking the turn.
		return false
	}
}

// close ends the turn's queue, so the player stops when it has played everything.
func (a *turnAudioPlan) close() {
	if a != nil && a.queue != nil {
		close(a.queue)
	}
}
```

In `pkg/gui/service.go`, replace the sentence-streamer/playback block in `Run`:

```go
	// Application playback runs on one queue opened before generation: a sentence
	// the streamer synthesizes is heard as soon as it lands, and the finalise pass
	// adds only what the stream has not already played. A session with no player
	// still warms the clips, because the URLs a client is handed are
	// content-addressed and cannot synthesize on demand.
	audioEnabled := t.cfg.Media.TTS.Type != "" && t.cfg.Media.TTS.Type != "disabled"
	plan := &turnAudioPlan{played: newClipSet()}
	if audioEnabled && t.cfg.Media.TTS.AutoPlay {
		if player := t.service.audioPlayer(); player != nil && player.Available() {
			plan.queue = make(chan string, sentenceQueueDepth)
			player.SetVolume(t.cfg.Media.TTS.MasterVolume)
			t.service.goBackground(func() {
				if err := player.PlayQueue(runCtx, plan.queue); err != nil && !errors.Is(err, playback.ErrUnavailable) {
					fmt.Fprintf(os.Stderr, "Warning: narration playback stopped: %v\n", err)
				}
			})
		}
	}

	streamer := t.service.sentenceStreamerFor(runCtx, t.gameID, t.cfg, func(speech provisionalSpeech) {
		plan.enqueueClip(speech.AudioKey, t.service.clipPath(speech.AudioKey))
		_ = emit(speechEvent(speech))
	})
	defer streamer.Close()

	turn, err := t.orchestrator.ProcessActionStream(runCtx, req.Mode, req.Input, func(text string) error {
		if emitErr := emit(TurnEvent{Type: "chunk", Text: text}); emitErr != nil {
			return emitErr
		}
		streamer.Feed(text)
		return nil
	})
	if err != nil {
		t.service.noteFailure("gm", err)
		plan.close()
		return err
	}
	t.service.noteSuccess("gm")
```

and after the turn DTO is emitted (replacing the old AutoPlay/warm-up branch):

```go
	// The rest of the turn's clips are synthesized behind the turn and appended to
	// the same queue, so playback continues without a second start and a clip that
	// failed mid-stream is retried here.
	if audioEnabled {
		t.service.goBackground(func() {
			t.finishTurnAudio(context.Background(), *turn, plan)
			plan.close()
		})
	} else {
		plan.close()
	}
```

`clipPath` resolves a key to the file the cache would hold, so the queue can carry paths:

```go
// clipPath names the file a clip key is stored under. The queue plays files, and
// the key is the file name, so no lookup is needed.
func (s *Service) clipPath(key string) string {
	if key == "" {
		return ""
	}
	return filepath.Join(s.resolver.CacheDir(), "audio", key+".opus")
}
```

and `finishTurnAudio`:

```go
// finishTurnAudio synthesizes every clip the turn still needs once it is recorded,
// appending to the plan only what the streamed sentences did not already play, so
// no line is heard twice and none is missed.
func (t *TurnSession) finishTurnAudio(ctx context.Context, turn engine.Turn, plan *turnAudioPlan) {
	for i := range turn.Segments {
		clips, err := t.service.GetSegmentClips(ctx, t.gameID, turn.Number, i)
		if err != nil {
			continue
		}
		for _, clip := range clips {
			plan.enqueueClip(media.ClipKeyForPath(clip), clip)
		}
	}
}
```

Imports to touch in `service.go`: add `playback`, keep `os`, `errors`, `fmt`.

- [ ] **Step 4: Run the tests**

Run: `go test ./pkg/gui/ -count=1 && go vet ./pkg/gui/`
Expected: PASS, vet clean.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui
git commit -m "feat(gui): play streamed narration from one exact played set"
```

---

### Task 5: Scene: a beat is a list of clips

**Files:**
- Modify: `pkg/scene/scene.go:23-34`, `pkg/scene/compile.go:44-47,162-173`
- Modify: `pkg/export/script.go:51-70`
- Test: `pkg/scene/compile_test.go`

**Interfaces:**
- Produces:
  - `type SpeechResolver interface { SegmentAudio(ctx context.Context, segment entity.TurnSegment) ([]string, time.Duration, error) }`
  - `Beat.AudioPaths []string`
  - `export.speechResolver.SegmentAudio` returning `([]string, time.Duration, error)`

- [ ] **Step 1: Write the failing test**

In `pkg/scene/compile_test.go`, change `fakeSpeech` to a list and add:

```go
type fakeSpeech struct {
	clips map[string]struct {
		paths    []string
		duration time.Duration
	}
	unavailable bool
}

func (f *fakeSpeech) SegmentAudio(ctx context.Context, segment entity.TurnSegment) ([]string, time.Duration, error) {
	if f.unavailable {
		return nil, 0, ErrAudioUnavailable
	}
	if clip, ok := f.clips[segment.Text]; ok {
		return clip.paths, clip.duration, nil
	}
	return nil, 0, ErrAudioUnavailable
}

func TestCompileKeepsEveryClipOfABeatInOrder(t *testing.T) {
	source := twoLocationSource()
	compiler := NewCompiler(source)
	compiler.SetSpeechResolver(&fakeSpeech{clips: map[string]struct {
		paths    []string
		duration time.Duration
	}{
		"Welcome.": {paths: []string{"/cache/welcome-1.wav", "/cache/welcome-2.wav"}, duration: 3 * time.Second},
	}})

	script, err := compiler.Compile(context.Background(), "campaign-01", Options{Audio: true})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	var found bool
	for _, beat := range script.Beats() {
		if beat.Text != "Welcome." {
			continue
		}
		found = true
		if len(beat.AudioPaths) != 2 || beat.AudioPaths[0] != "/cache/welcome-1.wav" || beat.AudioPaths[1] != "/cache/welcome-2.wav" {
			t.Errorf("AudioPaths = %#v, want both clips in order", beat.AudioPaths)
		}
		if beat.AudioDuration != 3*time.Second {
			t.Errorf("AudioDuration = %v, want the summed clip duration", beat.AudioDuration)
		}
		if want := 3*time.Second + scene.BeatGap; beat.Duration != want {
			t.Errorf("Duration = %v, want %v", beat.Duration, want)
		}
	}
	if !found {
		t.Fatal("the fixture no longer contains the audio beat")
	}
}
```

(Use `scene.BeatGap` only if the test lives outside package `scene`; inside package `scene`, write `BeatGap`.)

- [ ] **Step 2: Run it to watch it fail**

Run: `go test ./pkg/scene/ -run CompileKeepsEveryClip -v`
Expected: compile failure (type mismatch on `fakeSpeech`).

- [ ] **Step 3: Implement**

`pkg/scene/scene.go`:

```go
// Beat is one unit of playback: a span of text, its imagery, and its audio.
type Beat struct {
	Kind          BeatKind
	TurnNumber    int
	Speaker       string
	SpeakerID     string
	Text          string
	ArtPath       string
	AudioPaths    []string
	AudioDuration time.Duration
	Duration      time.Duration
}
```

`pkg/scene/compile.go`:

```go
// SpeechResolver returns a beat's clip paths and their total duration, or
// ErrAudioUnavailable. A beat is several clips when it is several sentences.
type SpeechResolver interface {
	SegmentAudio(ctx context.Context, segment entity.TurnSegment) ([]string, time.Duration, error)
}
```

and

```go
// resolveAudio attaches a beat's clips when they can be resolved. A missing clip is
// counted and skipped so one silent line cannot abandon the export.
func (c *Compiler) resolveAudio(ctx context.Context, beat *Beat, segment entity.TurnSegment, silent *int) {
	paths, duration, err := c.speech.SegmentAudio(ctx, segment)
	if err != nil || len(paths) == 0 {
		*silent++
		return
	}

	beat.AudioPaths = paths
	beat.AudioDuration = duration
}
```

`pkg/export/script.go`:

```go
func (r *speechResolver) SegmentAudio(ctx context.Context, segment entity.TurnSegment) ([]string, time.Duration, error) {
	clips, err := r.pipeline.SynthesizeSegmentClips(ctx, segment, r.narrator, r.voiceFor, false)
	if err != nil {
		return nil, 0, err
	}

	var total time.Duration
	for _, clip := range clips {
		duration, err := media.ProbeAudioDuration(ctx, clip)
		if err != nil {
			continue
		}
		total += duration
	}
	return clips, total, nil
}
```

- [ ] **Step 4: Run the scene and export tests**

Run: `go test ./pkg/scene/ ./pkg/export/ -count=1`
Expected: the scene test passes; export tests still fail to compile until Task 6 — run `go test ./pkg/scene/ -count=1` alone to confirm the scene side first.

- [ ] **Step 5: Commit**

```bash
git add pkg/scene pkg/export/script.go
git commit -m "refactor(scene): carry a beat's clips as a list"
```

---

### Task 6: Export: one file per unit

**Files:**
- Modify: `pkg/export/web.go:24-116` (and the player script's audio handling)
- Modify: `pkg/export/video.go:134-189`
- Test: `pkg/export/web_test.go`, `pkg/export/video_test.go`

**Interfaces:**
- Consumes: `scene.Beat.AudioPaths`.
- Produces: `webBeat.Audio []string` (`json:"audio,omitempty"`), one ffmpeg input per clip.

- [ ] **Step 1: Write the failing tests**

`pkg/export/video_test.go`:

```go
func TestBuildCommandAddsOneInputPerClip(t *testing.T) {
	pipeline := NewVideoPipeline(".")

	script := smallScript()
	script.Scenes[0].Beats[1].AudioPaths = []string{"/cache/one.wav", "/cache/two.wav"}
	script.Scenes[0].Beats[1].AudioDuration = 1500 * time.Millisecond
	script.Scenes[0].Beats[1].Duration = 1900 * time.Millisecond

	cmd, err := pipeline.BuildCommand(context.Background(), script, "/frames", "/out.mp4")
	if err != nil {
		t.Fatalf("BuildCommand failed: %v", err)
	}

	args := strings.Join(cmd.Args, " ")
	for _, want := range []string{
		"-i /cache/one.wav",
		"-i /cache/two.wav",
		"concat=n=3:v=0:a=1[a]",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("expected %q in the ffmpeg invocation:\n%s", want, args)
		}
	}
}
```

`pkg/export/web_test.go`: update `fixtureScript`'s clip beat to `AudioPaths: []string{clip}` and assert two files for a two-clip beat:

```go
func TestWebExportCopiesEveryClipOfABeat(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle")
	dir := t.TempDir()

	script := fixtureScript(t, dir)
	second := filepath.Join(dir, "walk.wav")
	if err := os.WriteFile(second, []byte("RIFF....WAVEfmt ....data"), 0644); err != nil {
		t.Fatal(err)
	}
	script.Scenes[0].Beats[2].AudioPaths = append(script.Scenes[0].Beats[2].AudioPaths, second)

	if _, err := NewWebExporter(".").Export(context.Background(), script, out); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	payload := readWebPayload(t, out)
	audio := payload.Scenes[0].Beats[2].Audio
	if len(audio) != 2 {
		t.Fatalf("audio = %#v, want both clips", audio)
	}
	for _, relative := range audio {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(relative))); err != nil {
			t.Errorf("expected %s in the bundle: %v", relative, err)
		}
	}
}
```

`readWebPayload` is a small helper extracted from the existing embedded-script test:

```go
type webPayloadFixture struct {
	GameName string `json:"game_name"`
	Scenes   []struct {
		Location string `json:"location"`
		Art      string `json:"art"`
		Beats    []struct {
			Kind     string   `json:"kind"`
			Speaker  string   `json:"speaker"`
			Text     string   `json:"text"`
			Art      string   `json:"art"`
			Audio    []string `json:"audio"`
			Duration float64  `json:"duration"`
		} `json:"beats"`
	} `json:"scenes"`
	Total float64 `json:"total_duration"`
}

func readWebPayload(t *testing.T, outDir string) webPayloadFixture {
	t.Helper()
	page, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	const marker = "const SCRIPT = "
	idx := strings.Index(string(page), marker)
	if idx == -1 {
		t.Fatal("expected the script to be embedded")
	}
	rest := string(page)[idx+len(marker):]
	rest = rest[:strings.Index(rest, ";\n")]

	var payload webPayloadFixture
	if err := json.Unmarshal([]byte(rest), &payload); err != nil {
		t.Fatalf("decode embedded script: %v", err)
	}
	return payload
}
```

- [ ] **Step 2: Run them to watch them fail**

Run: `go test ./pkg/export/ -run 'OneInputPerClip|EveryClip' -v`
Expected: compile failure, `AudioPaths undefined`.

- [ ] **Step 3: Implement**

`pkg/export/web.go`:

```go
type webBeat struct {
	Kind     string   `json:"kind"`
	Speaker  string   `json:"speaker,omitempty"`
	Text     string   `json:"text"`
	Art      string   `json:"art,omitempty"`
	Audio    []string `json:"audio,omitempty"`
	Duration float64  `json:"duration"`
}
```

and the beat loop:

```go
			for _, clip := range beat.AudioPaths {
				// Clips are numbered in the order they appear, so a bundle's audio
				// directory is a readable running order rather than beat numbers
				// with holes in them.
				beatNumber++
				name := fmt.Sprintf("beat-%04d%s", beatNumber, filepath.Ext(clip))
				if err := copyFile(clip, filepath.Join(outDir, "audio", name)); err == nil {
					jsBeat.Audio = append(jsBeat.Audio, "audio/"+name)
				}
			}
```

and the embedded player's playback, which must walk a beat's clips in order:

```js
function playBeatClips(beat, done) {
  const clips = beat.audio || [];
  let i = 0;
  const next = () => {
    if (i >= clips.length) { done(); return; }
    const audio = new Audio(clips[i++]);
    audio.onended = next;
    audio.onerror = next;
    audio.play().catch(() => done());
  };
  next();
}
```
replacing the single-`beat.audio` element handling in `playerHTML` (search for `new Audio`).

`pkg/export/video.go`:

```go
	streams := make([]string, 0, len(script.Beats()))
	index := 1

	for _, beat := range script.Beats() {
		// One input per unit: a multi-clip beat gets finer pacing than the beat's
		// own reading estimate, and a clip-less beat still gets its silence.
		if len(beat.AudioPaths) == 0 {
			silence := beat.Duration.Seconds()
			if silence <= 0 {
				silence = scene.MinimumBeatDuration.Seconds()
			}
			args = append(args,
				"-f", "lavfi",
				"-t", strconv.FormatFloat(silence, 'f', 3, 64),
				"-i", "anullsrc=r=44100:cl=stereo",
			)
			streams = append(streams, fmt.Sprintf("[%d:a]", index))
			index++
			continue
		}

		for _, clip := range beat.AudioPaths {
			args = append(args, "-i", clip)
			streams = append(streams, fmt.Sprintf("[%d:a]", index))
			index++
		}
	}
```

- [ ] **Step 4: Run the export tests**

Run: `go test ./pkg/export/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/export
git commit -m "feat(export): play a beat's clips, one file and one input per unit"
```

---

### Task 7: Frontend: consume `audio_urls` and regenerate by POST

**Files:**
- Create: `frontend/src/lib/audio.ts`
- Modify: `frontend/src/types.ts:53-70,134-149`
- Modify: `frontend/src/api/client.ts` (add `regenerateSegmentAudio`)
- Modify: `frontend/src/hooks/useSegmentPlayback.ts`
- Modify: `frontend/src/components/TurnSegments.tsx:54,76-88`, `frontend/src/components/StoryTheater.tsx:112`
- Test: `npx tsc --noEmit`

**Interfaces:**
- Consumes: `audio_urls`, `GET /api/audio/clip/{key}`, `POST .../segment/{i}/audio`.
- Produces:
  - `clipKeyFromURL(url: string): string`
  - `segmentClipURLs(segment: TurnSegment): string[]`
  - `APIClient.regenerateSegmentAudio(gameId, turnNumber, segmentIndex): Promise<string[]>`
  - `useSegmentPlayback(segments, autoPlay, volume, skipKeys?)`

- [ ] **Step 1: Rewrite the types and the clip helpers**

`frontend/src/types.ts`:

```ts
export interface TurnSegment {
  kind: 'narration' | 'speech';
  speaker?: string;
  speaker_id?: string;
  text: string;
  // Names the check whose roll this segment narrates, so the dice render inline.
  check_ref?: string;
  // The segment's clips in play order, one per sentence of reduced text. Each URL
  // is content-addressed, so the key in it is what identifies the audio.
  audio_urls?: string[];
  portrait_url?: string;
  // True for the protagonist's own line, which renders as speech but suppresses
  // the duplicate action block for the turn.
  player?: boolean;
  // Seconds the backend estimates this line takes to read, which is the same
  // estimate the exports pace with.
  duration?: number;
}
```
and
```ts
export interface TurnEvent {
  type: 'chunk' | 'speech' | 'turn' | 'tool' | 'error' | 'model_missing';
  text?: string;
  turn?: Turn;
  // Streamed narration, present when type is 'speech': the unit's ordinal and the
  // clip written for it.
  index?: number;
  audio_key?: string;
  audio_url?: string;
  message?: string;
  ...
}
```

`frontend/src/lib/audio.ts` (new):

```ts
import { TurnSegment } from '../types';

// clipKeyFromURL reads the content key out of a clip URL. The key is the audio's
// identity, so a played-set comparison needs nothing but the URL itself.
export const clipKeyFromURL = (url: string): string => {
  const path = url.split('?')[0];
  const parts = path.split('/');
  return parts[parts.length - 1] ?? '';
};

// segmentClipURLs is the ordered list of clip URLs a segment plays.
export const segmentClipURLs = (segment: TurnSegment | undefined): string[] =>
  segment?.audio_urls?.filter((url) => !!url) ?? [];
```

- [ ] **Step 2: Add the regeneration call**

`frontend/src/api/client.ts`, next to `audioStatus`/`stopAudio`:

```ts
  // regenerateSegmentAudio re-synthesizes one beat and answers with its refreshed
  // clip URLs. The keys are unchanged when nothing about the line changed.
  static async regenerateSegmentAudio(
    gameId: string,
    turnNumber: number,
    segmentIndex: number
  ): Promise<string[]> {
    const res = await fetch(
      `/api/game/${gameId}/turn/${turnNumber}/segment/${segmentIndex}/audio`,
      { method: 'POST' }
    );
    if (res.status === 204) return [];
    if (!res.ok) throw new Error(`regenerateSegmentAudio: ${res.statusText}`);
    const body = (await res.json()) as { audio_urls?: string[] };
    return body.audio_urls ?? [];
  }
```

- [ ] **Step 3: Flatten playback and honour a played set**

`frontend/src/hooks/useSegmentPlayback.ts` becomes:

```ts
import { useCallback, useEffect, useRef, useState } from 'react';
import { TurnSegment } from '../types';
import { clipKeyFromURL, segmentClipURLs } from '../lib/audio';
import { APIClient } from '../api/client';

interface Clip {
  segmentIndex: number;
  url: string;
}

export const useSegmentPlayback = (
  segments: TurnSegment[] | undefined,
  autoPlay: boolean,
  volume: number,
  skipKeys?: ReadonlySet<string>
) => {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const prefetchRef = useRef<HTMLAudioElement | null>(null);
  const prefetchedUrlRef = useRef<string | null>(null);
  const [playing, setPlaying] = useState(false);
  const [blocked, setBlocked] = useState(false);
  const [playingIndex, setPlayingIndex] = useState<number | null>(null);

  const clipsFor = useCallback((): Clip[] => {
    const clips: Clip[] = [];
    (segments ?? []).forEach((segment, segmentIndex) => {
      segmentClipURLs(segment).forEach((url) => {
        if (skipKeys?.has(clipKeyFromURL(url))) return;
        clips.push({ segmentIndex, url });
      });
    });
    return clips;
  }, [segments, skipKeys]);

  const stop = useCallback(() => { /* unchanged */ }, []);
  const playUrl = useCallback(/* unchanged, taking segmentIndex */, [volume]);

  const playFrom = useCallback(
    (index: number) => {
      const clips = clipsFor();
      const next = clips.findIndex((clip) => clip.segmentIndex >= index);
      if (next === -1) {
        stop();
        return;
      }

      const following = clips[next + 1];
      if (following && prefetchedUrlRef.current !== following.url) {
        const prefetch = new Audio(following.url);
        prefetch.preload = 'auto';
        prefetchRef.current = prefetch;
        prefetchedUrlRef.current = following.url;
      }

      playUrl(clips[next].url, clips[next].segmentIndex, () => playFrom(clips[next].segmentIndex + 1));
    },
    [clipsFor, stop, playUrl]
  );

  // regenerateFrom re-synthesizes one segment and plays its refreshed clips. The
  // server bypasses the cache, so the keys, and therefore the URLs, can change.
  const regenerateFrom = useCallback(
    async (index: number) => {
      if (!gameId) return;
      const urls = await APIClient.regenerateSegmentAudio(gameId, turnNumber, index);
      if (urls.length === 0) return;
      playUrl(urls[0], index);
    },
    [gameId, turnNumber, playUrl]
  );

  useEffect(() => {
    if (!autoPlay) return;
    playFrom(0);
    return stop;
  }, [autoPlay, playFrom, stop]);

  const play = useCallback(() => {
    setBlocked(false);
    playFrom(0);
  }, [playFrom]);

  return { playing, blocked, playingIndex, play, playFrom, regenerateFrom, stop };
};
```

The hook therefore needs `gameId` and `turnNumber` to regenerate; `TurnSegments` already has `turnNumber` and `ChronicleView` has `gameId`. Add both as parameters of `useSegmentPlayback` (a 6-argument hook is a smell, so pass an options object: `useSegmentPlayback(segments, { autoPlay, volume, skipKeys, gameId, turnNumber })`).

- [ ] **Step 4: Update the components**

`TurnSegments.tsx`:
```tsx
  const hasAudio = (segments ?? []).some((segment) => (segment.audio_urls?.length ?? 0) > 0);
  const { playing, blocked, playingIndex, play, playFrom, regenerateFrom, stop } = useSegmentPlayback(
    segments,
    { autoPlay: autoPlay && hasAudio && !serverPlayback, volume, gameId, turnNumber, skipKeys: skipAudioKeys }
  );
```
and
```tsx
    if ((ordered[index]?.audio_urls?.length ?? 0) === 0) return null;
```
plus a new optional prop `skipAudioKeys?: ReadonlySet<string>`.

`ChronicleView.tsx` and `StoryTheater.tsx`: `hasAudio` becomes `(segment.audio_urls?.length ?? 0) > 0`, `gameId` and `skipAudioKeys` are forwarded.

- [ ] **Step 5: Typecheck and build**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors (a red type error here means a missed `audio_url` reference).

- [ ] **Step 6: Commit**

```bash
git add frontend/src
git commit -m "feat(frontend): play a segment's ordered clips and regenerate by key"
```

---

### Task 8: Frontend: play streamed speech, skip it at finalise

**Files:**
- Create: `frontend/src/hooks/useStreamedSpeech.ts`
- Modify: `frontend/src/App.tsx` (`handleActionSubmit`, `ChronicleView`/`StoryTheater` wiring)
- Test: `npx tsc --noEmit`

**Interfaces:**
- Consumes: `speech` events (`audio_key`, `audio_url`), `TurnSegment.audio_urls`.
- Produces: `useStreamedSpeech(options: { enabled: boolean; volume: number })` returning `{ enqueue(url: string, key: string): void, playedKeys(): Set<string>, reset(): void, stop(): void }`.

- [ ] **Step 1: Write the hook**

`frontend/src/hooks/useStreamedSpeech.ts` (new):

```ts
import { useCallback, useRef } from 'react';

// useStreamedSpeech plays narration clips as the turn streams, in arrival order,
// and remembers which keys were played. The chronicle then skips exactly those
// keys when the authoritative turn arrives, so no line is heard twice. It is idle
// when application playback is running: one device must own the sound.
export const useStreamedSpeech = (enabled: boolean, volume: number) => {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const queueRef = useRef<Array<{ url: string; key: string }>>([]);
  const playedRef = useRef<Set<string>>(new Set());
  const drainingRef = useRef(false);

  const drain = useCallback(() => {
    if (drainingRef.current) return;
    const next = queueRef.current.shift();
    if (!next) return;
    drainingRef.current = true;
    const audio = new Audio(next.url);
    audio.volume = volume;
    audioRef.current = audio;
    const advance = () => {
      drainingRef.current = false;
      drain();
    };
    audio.onended = advance;
    audio.onerror = advance;
    audio.play().catch(advance);
  }, [volume]);

  const enqueue = useCallback(
    (url: string, key: string) => {
      if (!enabled || !url || !key) return;
      playedRef.current.add(key);
      queueRef.current.push({ url, key });
      drain();
    },
    [enabled, drain]
  );

  const playedKeys = useCallback(() => new Set(playedRef.current), []);

  const reset = useCallback(() => {
    audioRef.current?.pause();
    audioRef.current = null;
    queueRef.current = [];
    playedRef.current = new Set();
    drainingRef.current = false;
  }, []);

  const stop = useCallback(() => {
    audioRef.current?.pause();
    audioRef.current = null;
    queueRef.current = [];
    drainingRef.current = false;
  }, []);

  return { enqueue, playedKeys, reset, stop };
};
```

- [ ] **Step 2: Wire it into the turn stream**

In `App.tsx`:

```tsx
  const streamedSpeech = useStreamedSpeech(
    !serverAudio && (config?.media.tts.auto_play ?? false),
    config?.media.tts.master_volume ?? 1
  );
  const [streamedKeys, setStreamedKeys] = useState<ReadonlySet<string>>(new Set());
```

`handleActionSubmit`:

```tsx
    setStreamedProse('');
    setToolActivity(null);
    setTurnError(null);
    streamedSpeech.reset();
```
inside the callback:

```tsx
          } else if (event.type === 'speech') {
            streamedSpeech.enqueue(event.audio_url ?? '', event.audio_key ?? '');
          } else if (event.type === 'turn' && event.turn) {
            // The clips already played are handed to the chronicle, which plays
            // only the rest of the turn: the played set is exact because a clip is
            // one unit of text.
            setStreamedKeys(streamedSpeech.playedKeys());
            const turn = event.turn;
            ...
```
in `finally`:
```tsx
      streamedSpeech.stop();
```
in `handleStopTurn`:
```tsx
    streamedSpeech.stop();
    setStreamedProse('');
    setPendingAction(null);
```

and pass the set down:

```tsx
                    autoPlay={serverAudio ? false : config?.media.tts.auto_play ?? false}
                    skipAudioKeys={streamedKeys}
```
and to `StoryTheater`:
```tsx
              serverPlayback={serverAudio}
              skipAudioKeys={streamedKeys}
```

`ChronicleView`/`StoryTheater`/`TurnSegments` forward `skipAudioKeys` to `useSegmentPlayback` (Task 7 already added the prop).

- [ ] **Step 3: Typecheck**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src
git commit -m "feat(frontend): start narrating as the prose arrives"
```

---

### Task 9: Full verification

**Files:** none new.

- [ ] **Step 1: Run the Go suite and the frontend typecheck**

Run: `mise run test`
Expected: `go test -v -count=1 ./...` PASS and `npx tsc --noEmit` clean.

- [ ] **Step 2: Lint**

Run: `mise run lint`
Expected: markdownlint and `go vet ./...` clean.

- [ ] **Step 3: Prove the concatenation path is gone**

Run: `rg -n "concatenateSentences|GetSegmentAudio|SegmentAudio\(ctx" --type go`
Expected: matches only in `pkg/scene/compile.go` (the renamed resolver interface) and `pkg/export/script.go` (its implementation); no `concatenateSentences`, no `GetSegmentAudio`.

- [ ] **Step 4: Regenerate generated docs if the route or DTO manifest test asks**

Run: `go test ./pkg/gui -run 'RouteManifest|DTO' -count=1`
Expected: PASS. If the manifest is stale: `go test ./pkg/gui -update-routes`.

- [ ] **Step 5: Commit any generated artefacts**

```bash
git add -A && git commit -m "chore(gui): refresh the generated route manifest"
```

---

## Implementation Notes (added during execution)

Two things the plan's task list did not spell out, both required by the spec's error
table ("Emit from a worker | Serialised by a mutex owned by the turn session"):

1. `TurnSession.Run` serialises every emission through one mutex (`announce`), because
   a streamer worker announces a finished sentence while the orchestrator emits tool
   activity and chunks, and the wire is a single newline-delimited stream.
2. `sentenceStreamer.StopEmitting` is called once the turn is authoritative (and on the
   error path, before the queue closes). A sentence still in flight is synthesized all
   the same — the finalise pass plays it — but it is no longer announced, so the played
   set a client holds stays in step with the audio it actually heard and no worker can
   send into a closing queue.

`useStreamedSpeech` marks a clip heard only when it ends, so a line cut short by the
turn landing is replayed from its start by the chronicle instead of being lost, and the
server's playback queue outlives the request context (`context.Background()`), because
it drains after the turn rather than being cancelled with it.

---

## Self-Review

**1. Spec coverage**

| Spec requirement | Task |
| --- | --- |
| §3.1 one unit, one clip, one key; `SynthesizeSegmentClips`/`SegmentClipKeys`/`SynthesizeSegments`; `concatenateSentences` deleted | 1 |
| §3.2 `GET /api/audio/clip/{key}`; `SegmentDTO.audio_urls`; `POST .../segment/{i}/audio` | 2 |
| §3.3 `provisionalSpeech`, streamer emission, `TurnEvent` speech fields, played set, no partial-coverage rule | 3, 4 |
| §3.4 `scene.Beat.AudioPaths`, summed `AudioDuration`, web bundle per clip, video one input per unit | 5, 6 |
| §3.5 removals: concatenated segment clip, `AudioKey`/`audio_url` duplication, narration namespace mismatch | 1, 2, 4 |
| §5 per-unit failure skipped; malformed key 404; player unavailable; queue closed on failure | 1, 2, 4 |
| §6 test matrix (Go and frontend) | 1-9 |
| Frontend `audio_urls` in chronicle and theater; `tsc` | 7, 8 |

**2. Placeholder scan:** no "TBD", no "add error handling", no "similar to Task N"; every code step carries the code.

**3. Type consistency:** `SynthesizeSegmentClips(...) ([]string, error)` is used identically in Tasks 1, 2, 4, 5; `SegmentClipKeys` returns keys whose length equals the clip list (Task 1 test); `clipURL`/`clipURLs`/`ClipKeyForPath`/`clipKeyFromURL` are defined before use across tasks; `turnAudioPlan.enqueueClip(key, path)` is called with a key and a path in both Tasks 3 and 4.

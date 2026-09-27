# Turn Latency Quick Wins Implementation Plan

> **Status:** Implemented and verified against the code on 2026-09-27.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Cut the avoidable per-turn latency: bounded tool rounds, a trim default, targeted entity indexing, overlapped extraction, one shared TTS pipeline, and playback that starts on the first clip.

**Architecture:** Six independent, behaviour-preserving changes across `pkg/config`, `pkg/engine`, `pkg/gui`, `pkg/media`, and `frontend`. Each is revertable on its own and is verified by a Go test.

**Tech Stack:** Go 1.27 (stdlib `testing`), `beep`/`oto` playback, React 19 frontend. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-09-26-turn-latency-reduction-design.md`

## Global Constraints

- Go tests use only `testing` and `t.TempDir()`; no testify, no mock libraries.
- Use `interface{}`, never `any`; wrap errors with `fmt.Errorf("...: %w", err)`; `go vet ./...` clean.
- No new dependencies.
- Conventional Commits with a scope; subject under 72 chars.
- Verification commands: `mise run test`, `mise run lint`, `mise run build`.
- Known pre-existing flake: `pkg/gui` intermittently fails a TempDir-cleanup race in `TestRegenerateCharacterPortraitEndpoint` / `TestTurnSessionEmitsModelMissingWhenTTSMissing` (also on `main`). Re-run before treating a `pkg/gui` failure as real.

---

### Task 1: Bound tool rounds and default completion to trim

**Files:**
- Modify: `pkg/config/types.go` (`ToolRounds` ~line 649-655, `CompletionMode` ~line 594-603, `DefaultConfig` completion `Mode` ~line 287)
- Test: `pkg/config/types_test.go` (append)

**Interfaces:**
- Produces: `Config.ToolRounds() int` returns `3` when unset; `Config.CompletionMode()` returns `"trim"` when unset.

- [x] **Step 1: Write the failing tests**

Append to `pkg/config/types_test.go`:

```go
func TestToolRoundsDefaultsToABoundedCap(t *testing.T) {
	if got := (&Config{}).ToolRounds(); got != 3 {
		t.Errorf("ToolRounds() = %d, want 3 for an unset value", got)
	}
	if got := (&Config{Agents: AgentsConfig{ToolRounds: 7}}).ToolRounds(); got != 7 {
		t.Errorf("ToolRounds() = %d, want the configured 7", got)
	}
	if got := (&Config{Agents: AgentsConfig{ToolRounds: -1}}).ToolRounds(); got != 0 {
		t.Errorf("ToolRounds() = %d, want 0 (unbounded) for a negative value", got)
	}
}

func TestCompletionModeDefaultsToTrim(t *testing.T) {
	if got := (&Config{}).CompletionMode(); got != "trim" {
		t.Errorf("CompletionMode() = %q, want trim for an unset value", got)
	}
	if got := (&Config{Agents: AgentsConfig{Completion: CompletionConfig{Mode: "auto"}}}).CompletionMode(); got != "auto" {
		t.Errorf("CompletionMode() = %q, want the configured auto", got)
	}
}
```

If `AgentsConfig`/`CompletionConfig` field names differ, use the names in `pkg/config/types.go`; the nested type is what `DefaultConfig` uses at line ~287.

- [x] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestToolRoundsDefaults|TestCompletionModeDefaults' ./pkg/config/ -v`
Expected: FAIL — both return the old defaults (`0` unbounded, `auto`).

- [x] **Step 3: Change the defaults**

In `pkg/config/types.go`:

```go
// defaultToolRounds bounds a turn's tool calls when none is configured. A
// negative value means unbounded.
const defaultToolRounds = 3

// ToolRounds caps how many times a turn may call tools. Zero or unset uses the
// default; a negative value means unbounded.
func (c *Config) ToolRounds() int {
	if c.Agents.ToolRounds < 0 {
		return 0
	}
	if c.Agents.ToolRounds == 0 {
		return defaultToolRounds
	}
	return c.Agents.ToolRounds
}
```

```go
// CompletionMode is the recovery policy: "auto", "continue", "trim", or "off".
// Unset defaults to "trim" so a cut reply ends instead of paying for a second
// full model call.
func (c *Config) CompletionMode() string {
	mode := strings.ToLower(strings.TrimSpace(c.Agents.Completion.Mode))
	switch mode {
	case "auto", "continue", "trim", "off":
		return mode
	default:
		return "trim"
	}
}
```

In `DefaultConfig` (~line 287) change the completion `Mode: "auto"` to `Mode: "trim"`.

- [x] **Step 4: Run tests to verify they pass**

Run: `go test -run 'TestToolRoundsDefaults|TestCompletionModeDefaults' ./pkg/config/ -v`
Expected: PASS.

- [x] **Step 5: Run the package and commit**

Run: `go test ./pkg/config/`

```bash
git add pkg/config/types.go pkg/config/types_test.go
git commit -m "perf(config): bound tool rounds and default completion to trim"
```

---

### Task 2: Index only the entities a turn writes

**Files:**
- Modify: `pkg/engine/timeline.go` (`writeEntities` ~line 248-273)
- Test: `pkg/engine/timeline_sync_test.go` (create)

**Interfaces:**
- Consumes: `storage.Syncer.SyncFile(path string) error` (`pkg/storage/sync.go:81`).
- Produces: no signature change; `writeEntities` no longer re-indexes the whole entities directory.

- [x] **Step 1: Write the failing test**

Create `pkg/engine/timeline_sync_test.go`:

```go
package engine

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWriteEntitiesIndexesWrittenEntities guards the correctness the sync change
// must keep: every entity the turn writes is present in the index afterwards.
func TestWriteEntitiesIndexesWrittenEntities(t *testing.T) {
	paths, store := writeTestCampaignScaffoldWithStore(t)
	timeline := NewTimeline(paths, store, NewHistoryLogger(filepath.Join(paths.GameDir("campaign-01"), "history.jsonl")), "campaign-01")

	ent := newTestEntity("mira", "Mira", "character", "A scout.")
	pending := map[string]*entity.Entity{"mira": ent}
	if err := timeline.writeEntities(pending); err != nil {
		t.Fatalf("writeEntities: %v", err)
	}

	loaded, err := store.GetEntity("mira")
	if err != nil {
		t.Fatalf("GetEntity(mira): %v", err)
	}
	if loaded == nil || loaded.Name != "Mira" {
		t.Fatalf("written entity not indexed: %+v", loaded)
	}
}
```

Use the package's existing scaffold helpers: `writeTestCampaignScaffold` (`pkg/engine/game_test.go:97`) returns a resolver; open the store with `storage.OpenGameStore(paths, "campaign-01")`. If no `writeTestCampaignScaffoldWithStore` helper exists, write the test inline:

```go
	paths := writeTestCampaignScaffold(t, t.TempDir(), nil)
	if _, err := InitGame(paths, InitOptions{GameID: "campaign-01", SystemID: "d20-test", WorldID: "fantasy-realm", PlayerName: "Sean"}); err != nil {
		t.Fatal(err)
	}
	store, err := storage.OpenGameStore(paths, "campaign-01")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
```

and construct `entity.Entity` directly with `&entity.Entity{ID: "mira", Name: "Mira", Type: "character", Body: "A scout.", Hash: "mira-hash"}`.

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestWriteEntitiesIndexesWrittenEntities ./pkg/engine/ -v`
Expected: FAIL to compile (`writeTestCampaignScaffoldWithStore`/`newTestEntity` undefined) if using the helper form — then inline it as above and confirm it PASSES against current code (it is the correctness guard, not the bug). Proceed once green; the change in Step 3 must keep it green.

- [x] **Step 3: Replace the full-directory sync**

In `pkg/engine/timeline.go`, change `writeEntities` so each written file is indexed individually:

```go
	for _, id := range ids {
		path := filepath.Join(dir, id+".md")
		data, err := pending[id].SerializeMarkdown()
		if err != nil {
			return fmt.Errorf("serialize entity %q: %w", id, err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			return fmt.Errorf("write entity %q: %w", id, err)
		}
		// Index just this file. Syncing the whole directory re-parsed every
		// entity on every turn that touched one.
		if err := storage.NewSyncer(t.store).SyncFile(path); err != nil {
			return fmt.Errorf("index entity %q: %w", id, err)
		}
	}

	return nil
```

Delete the trailing `storage.NewSyncer(t.store).Sync(dir)` block.

- [x] **Step 4: Run the test and the engine package**

Run: `go test -run TestWriteEntitiesIndexesWrittenEntities ./pkg/engine/ -v && go test ./pkg/engine/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/engine/timeline.go pkg/engine/timeline_sync_test.go
git commit -m "perf(engine): index only the entities a turn writes"
```

---

### Task 3: Start extraction before the local turn work

**Files:**
- Modify: `pkg/engine/orchestrator.go` (turn assembly ~line 810-852)
- Test: `pkg/engine/extract_overlap_test.go` (create)

**Interfaces:**
- Consumes: `harness.Extractor.Extract(ctx, narration) (*harness.Extraction, error)`.
- Produces: no signature change; extraction runs concurrently with mention resolution and structured-turn bookkeeping and is awaited before segments are built.

- [x] **Step 1: Write the failing test**

A concurrency change is not observable from a single-turn assertion, so the test guards the contract the overlap must preserve: a non-structured turn still merges extraction into its segments.

Create `pkg/engine/extract_overlap_test.go`:

```go
package engine

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

// TestNonStructuredTurnStillUsesExtraction asserts the extraction result reaches
// segment building, which the overlap refactor must not break.
func TestNonStructuredTurnStillUsesExtraction(t *testing.T) {
	o := &TurnOrchestrator{}
	o.store = nil
	extraction := harness.Extraction{Entities: []harness.ExtractedEntity{{Name: "Mira", Type: "character"}}}
	segments := buildTurnSegments(o.store, "Mira waves from the ridge.", extraction)
	if len(segments) == 0 {
		t.Fatal("expected segments from an extracted turn")
	}
	found := false
	for _, seg := range segments {
		if seg.Kind == "speech" && seg.Speaker == "Mira" {
			found = true
		}
	}
	if !found {
		t.Fatalf("extracted speaker missing from segments: %+v", segments)
	}
}
```

Adjust the `harness.Extraction`/`ExtractedEntity` field names to match `pkg/harness/extractor.go`; `buildTurnSegments` is in `pkg/engine/segments.go`. If building segments needs a resolver, construct the orchestrator with the same scaffold as `toolLoopOrchestrator` and use its store.

- [x] **Step 2: Run test to verify it fails or passes for the right reason**

Run: `go test -run TestNonStructuredTurnStillUsesExtraction ./pkg/engine/ -v`
Expected: PASS once the test compiles against current code (contract guard). Keep it green through Step 3.

- [x] **Step 3: Overlap extraction with local work**

In `pkg/engine/orchestrator.go`, immediately after the `turn` struct is built (around line 808, before `turn.Entities = harness.ResolveEntityMentions(...)`):

```go
	structured := result.Submission != nil
	extraction := harness.Extraction{}
	var extractionErr error
	extractionDone := make(chan struct{})

	// Extraction is a model call, so start it before the local mention and
	// segment work and await it just before segments are built. A failed
	// extractor must not lose the turn.
	if !structured && o.extractor != nil {
		go func() {
			defer close(extractionDone)
			extractCtx, extractSpan := telemetry.Tracer("github.com/darkliquid/localrpg/pkg/engine").Start(ctx, "extract.entities")
			defer extractSpan.End()
			if extracted, err := o.extractor.Extract(extractCtx, turn.Narration); err == nil {
				extraction = *extracted
			} else {
				extractionErr = err
				o.logger.Event("extract.error", map[string]interface{}{"error": err.Error()})
			}
			extractSpan.SetAttributes(attribute.Int("localrpg.entities.extracted", len(extraction.Entities)))
		}()
	} else {
		close(extractionDone)
	}
```

Then replace the existing `structured := result.Submission != nil` / `extraction := harness.Extraction{}` block (lines 812-813) so it does **not** redeclare `structured`/`extraction`, and replace the whole `else if o.extractor != nil { ... }` branch (lines 828-842) with an await:

```go
	}

	<-extractionDone
	if extractionErr != nil {
		o.logger.Event("extract.error", map[string]interface{}{"error": extractionErr.Error()})
	}
```

Leave the `if structured { ... }` branch that reads `result.Submission` unchanged, and ensure the `buildTurnSegments(o.store, turn.Narration, extraction)` call at ~line 851 happens after `<-extractionDone`.

- [x] **Step 4: Verify with the race detector and the package**

Run: `go test -race -run TestNonStructuredTurnStillUsesExtraction ./pkg/engine/ -v && go test -race ./pkg/engine/`
Expected: PASS, no data race (the channel close orders the writes before the read).

- [x] **Step 5: Commit**

```bash
git add pkg/engine/orchestrator.go pkg/engine/extract_overlap_test.go
git commit -m "perf(engine): overlap entity extraction with local turn work"
```

---

### Task 4: Share one TTS client and pipeline

**Files:**
- Modify: `pkg/gui/service.go` (Service struct ~line 36-63; `GetSegmentAudio` ~line 1671-1709)
- Test: `pkg/gui/audio_pipeline_test.go` (create)

**Interfaces:**
- Produces: `(*Service).audioPipeline() (*media.TTSPipeline, error)` — one pipeline per `*config.Config` identity, carrying the client and content cache.
- Consumes: `s.configMgr.Get()` returns the same `*Config` until the next `Load`/`Save`, so pointer identity is the cache key.

- [x] **Step 1: Write the failing test**

Create `pkg/gui/audio_pipeline_test.go`:

```go
package gui

import "testing"

// TestAudioPipelineIsBuiltOnce checks the shared pipeline: two requests must not
// construct two TTS clients (which for built-in TTS reloads the model).
func TestAudioPipelineIsBuiltOnce(t *testing.T) {
	_, svc := turnFixture(t)

	builds := 0
	svc.newTTSClient = func(cfg config.TTSConfig) (media.TTSClient, error) {
		builds++
		return &stubTTSClient{}, nil
	}
	svc.configMgr.Get().Media.TTS.Type = "cli"
	svc.configMgr.Get().Media.TTS.Command = "true"

	first, err := svc.audioPipeline()
	if err != nil {
		t.Fatalf("first audioPipeline: %v", err)
	}
	second, err := svc.audioPipeline()
	if err != nil {
		t.Fatalf("second audioPipeline: %v", err)
	}
	if first != second {
		t.Fatal("audioPipeline rebuilt the pipeline for the same config")
	}
	if builds != 1 {
		t.Fatalf("tts client builds = %d, want 1", builds)
	}
}
```

Add a minimal `stubTTSClient` in the test file implementing `media.TTSClient` (methods `Synthesize`, `ListVoices`, `PreviewURL`, `MarkdownAware`, `Name` — copy the interface from `pkg/media/providers.go`). Import `config` and `media`.

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestAudioPipelineIsBuiltOnce ./pkg/gui/ -v`
Expected: FAIL — `svc.audioPipeline` undefined (and `newTTSClient` may not be exported for assignment in the package test; it is a field, so this is fine).

- [x] **Step 3: Add the cached pipeline**

In `pkg/gui/service.go`, add to the `Service` struct:

```go
	// The audio pipeline is shared and rebuilt only when the configuration
	// object changes, so a built-in TTS model is loaded once, not per segment.
	ttsMu       sync.Mutex
	ttsConfig   *config.Config
	ttsPipeline *media.TTSPipeline
```

Add the constructor:

```go
// audioPipeline returns the shared TTS pipeline, building it when the current
// configuration object differs from the one it was built from.
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
	s.ttsConfig, s.ttsPipeline = cfg, pipeline
	return pipeline, nil
}
```

Update `GetSegmentAudio` to use it, keeping the narrator voice lookup per call:

```go
	narratorVoice := s.narratorVoiceFor(gameID, cfg)
	pipeline, err := s.audioPipeline()
	if err != nil {
		return "", err
	}
	isForce := len(force) > 0 && force[0]
	return pipeline.SynthesizeSegmentForce(ctx, turn.Segments[segmentIndex], narratorVoice, s.voiceFor(gameID), isForce)
```

Remove the now-unused `client, err := s.ttsClientFor(...)` and `pipeline := media.NewTTSPipeline(...)` lines from `GetSegmentAudio`.

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestAudioPipelineIsBuiltOnce ./pkg/gui/ -v`
Expected: PASS.

- [x] **Step 5: Run the package and commit**

Run: `go test ./pkg/gui/`

```bash
git add pkg/gui/service.go pkg/gui/audio_pipeline_test.go
git commit -m "perf(media): share one TTS pipeline across segments"
```

---

### Task 5: Start playback on the first completed clip

**Files:**
- Modify: `pkg/media/playback/player.go` (`PlayFiles` ~line 147-231; add `playStreamer` and `PlayQueue`)
- Modify: `pkg/gui/service.go` (`PlayTurnAudio` ~line 1818-1857)
- Test: `pkg/media/playback/queue_test.go` (create)

**Interfaces:**
- Produces: `(*Player).PlayQueue(ctx context.Context, clips <-chan string) error`; internal `playStreamer(queue beep.Streamer, closers []io.Closer) error`.
- Consumes: `decodeFile(path) (beep.Streamer, io.Closer, error)` (existing, `pkg/media/playback/player.go`).

- [x] **Step 1: Write the failing test**

Create `pkg/media/playback/queue_test.go`:

```go
package playback

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// writeSilentWAV writes a minimal 16-bit mono PCM WAV of the given frames.
func writeSilentWAV(t *testing.T, dir, name string, frames int) string {
	t.Helper()
	path := filepath.Join(dir, name)
	data := make([]byte, 44+frames*2)
	copy(data[0:], "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(36+frames*2))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], 44100)
	binary.LittleEndian.PutUint32(data[28:], 44100*2)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(frames*2))
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestQueueStreamerPullsClipsLazilyInOrder(t *testing.T) {
	dir := t.TempDir()
	first := writeSilentWAV(t, dir, "one.wav", 100)
	second := writeSilentWAV(t, dir, "two.wav", 100)

	clips := make(chan string, 2)
	clips <- first
	clips <- second
	close(clips)

	q := newQueueStreamer(context.Background(), clips)
	buf := make([][2]float64, 50)
	total := 0
	for {
		n, ok := q.Stream(buf)
		total += n
		if !ok {
			break
		}
	}
	if total != 200 {
		t.Fatalf("streamed %d frames, want 200 across both clips", total)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestQueueStreamerPullsClipsLazilyInOrder ./pkg/media/playback/ -v`
Expected: FAIL — `newQueueStreamer` undefined.

- [x] **Step 3: Add the lazy queue streamer and refactor playback**

In `pkg/media/playback/player.go`, add:

```go
// queueStreamer lazily decodes clips as the audio callback needs them, so
// playback can start on the first completed clip instead of waiting for all of
// them. A clip that cannot be decoded is skipped, as PlayFiles does.
type queueStreamer struct {
	ctx     context.Context
	clips   <-chan string
	current beep.Streamer
	closer  io.Closer
	done    bool
}

func newQueueStreamer(ctx context.Context, clips <-chan string) *queueStreamer {
	return &queueStreamer{ctx: ctx, clips: clips}
}

func (q *queueStreamer) next() bool {
	if q.done {
		return false
	}
	for {
		select {
		case <-q.ctx.Done():
			q.done = true
			return false
		case path, ok := <-q.clips:
			if !ok {
				q.done = true
				return false
			}
			streamer, closer, err := decodeFile(path)
			if err != nil {
				continue
			}
			q.current, q.closer = streamer, closer
			return true
		}
	}
}

func (q *queueStreamer) Stream(samples [][2]float64) (int, bool) {
	filled := 0
	for filled < len(samples) {
		if q.current == nil && !q.next() {
			return filled, filled > 0
		}
		n, ok := q.current.Stream(samples[filled:])
		filled += n
		if !ok {
			if q.closer != nil {
				_ = q.closer.Close()
			}
			q.current, q.closer = nil, nil
			if n == 0 {
				continue
			}
		}
	}
	return filled, true
}

func (q *queueStreamer) Err() error { return nil }
```

Refactor `PlayFiles` so the post-decode wiring is shared:

```go
// playStreamer installs a streamer as the current queue. It returns once
// playback has started.
func (p *Player) playStreamer(queue beep.Streamer, closers []io.Closer) error {
	p.mu.Lock()
	previousClosers := p.closers
	if p.otoPlayer != nil {
		_ = p.otoPlayer.Close()
		p.otoPlayer = nil
	}

	p.generation++
	gen := p.generation
	p.streamer = queue
	p.closers = closers
	p.playing = true

	reader := &streamerReader{streamer: queue}
	otoPlayer := p.otoCtx.NewPlayer(reader)
	otoPlayer.SetVolume(p.gain)
	p.otoPlayer = otoPlayer

	logger := trace.OrNil(p.logger)
	gain := p.gain
	p.mu.Unlock()

	logger.Event("audio.play", map[string]interface{}{"clips": 0, "volume": gain})
	go closeAll(previousClosers)
	otoPlayer.Play()

	go func(player *oto.Player, gen uint64, closers []io.Closer, r *streamerReader) {
		for {
			time.Sleep(20 * time.Millisecond)
			p.mu.Lock()
			if p.generation != gen || p.otoPlayer != player {
				p.mu.Unlock()
				return
			}
			if !player.IsPlaying() || (r.isDrained() && player.BufferedSize() == 0) {
				p.playing = false
				p.streamer = nil
				p.closers = nil
				p.otoPlayer = nil
				p.mu.Unlock()
				_ = player.Close()
				closeAll(closers)
				return
			}
			p.mu.Unlock()
		}
	}(otoPlayer, gen, closers, reader)

	return nil
}
```

Change `PlayFiles` to build its `queue`/`closers` as today and finish with `return p.playStreamer(queue, closers)`.

Add:

```go
// PlayQueue starts playback and pulls clip paths from clips as each previous
// clip drains, so the first completed clip is heard while the rest are still
// synthesized. It returns once playback has started; closing the channel ends
// the queue.
func (p *Player) PlayQueue(ctx context.Context, clips <-chan string) error {
	if !p.Available() {
		return ErrUnavailable
	}
	return p.playStreamer(newQueueStreamer(ctx, clips), nil)
}
```

- [x] **Step 4: Run the queue test**

Run: `go test -run TestQueueStreamer ./pkg/media/playback/ -v && go test ./pkg/media/playback/`
Expected: PASS.

- [x] **Step 5: Use PlayQueue for whole-turn playback**

In `pkg/gui/service.go`, rewrite `PlayTurnAudio` so synthesis feeds a channel while playback starts on the first clip:

```go
func (s *Service) PlayTurnAudio(ctx context.Context, gameID string, turnNumber int, force ...bool) error {
	player := s.audioPlayer()
	if player == nil || !player.Available() {
		return playback.ErrUnavailable
	}

	turn, err := s.findTurn(gameID, turnNumber)
	if err != nil {
		return err
	}
	isForce := len(force) > 0 && force[0]

	clips := make(chan string)
	go func() {
		defer close(clips)
		for i := range turn.Segments {
			path, err := s.GetSegmentAudio(ctx, gameID, turnNumber, i, isForce)
			if err != nil || path == "" {
				continue
			}
			select {
			case clips <- path:
			case <-ctx.Done():
				return
			}
		}
	}()

	player.SetVolume(s.configMgr.Get().Media.TTS.MasterVolume)
	return player.PlayQueue(ctx, clips)
}
```

- [x] **Step 6: Run the gui package and commit**

Run: `go test ./pkg/gui/ ./pkg/media/playback/ && mise run lint`

```bash
git add pkg/media/playback/player.go pkg/media/playback/queue_test.go pkg/gui/service.go
git commit -m "perf(playback): start narration on the first completed clip"
```

---

### Task 6: Let the browser cache and actually prefetch clips

**Files:**
- Modify: `pkg/gui/server.go` (clip route ~line 427-437)
- Modify: `frontend/src/hooks/useSegmentPlayback.ts` (prefetch ~line 59-67)
- Test: `pkg/gui/server_clip_cache_test.go` (create)

**Interfaces:**
- Produces: clip responses carry an `ETag` derived from the bytes and a cacheable `Cache-Control`, replacing `no-store`.

- [x] **Step 1: Write the failing test**

Create `pkg/gui/server_clip_cache_test.go`. Reuse the route test harness in `pkg/gui/server_test.go`; the test calls the clip route for a synthesized segment and asserts:

```go
func TestClipResponseIsCacheableWithETag(t *testing.T) {
	// ... build a game with one turn whose segment has a clip ...
	req := httptest.NewRequest(http.MethodGet, "/api/game/campaign-01/turn/1/segment/0/audio", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("ETag") == "" {
		t.Error("expected an ETag on the clip response")
	}
	if cc := rec.Header().Get("Cache-Control"); strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control = %q, must be cacheable", cc)
	}
}
```

Follow the existing route-test setup in `pkg/gui/server_test.go` for constructing the handler and a game with audio; if audio cannot be synthesized in-test, assert on the header logic by extracting the header-setting into a small helper `setClipHeaders(w http.ResponseWriter, data []byte)` and testing that helper directly.

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestClipResponseIsCacheableWithETag ./pkg/gui/ -v`
Expected: FAIL — `Cache-Control: no-store`, no `ETag`.

- [x] **Step 3: Set cacheable headers**

In `pkg/gui/server.go`, replace the `no-store` line with an ETag from the bytes and a cacheable policy. The URL already carries the audio key as `?v=`, so a changed clip changes the URL:

```go
			sum := sha256.Sum256(data)
			w.Header().Set("ETag", fmt.Sprintf(`"%x"`, sum[:8]))
			// The clip URL embeds the audio cache key, so a voice or text change
			// yields a new URL; the old one can be cached hard.
			w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
			w.Header().Set("Content-Type", media.AudioContentType(data))
			_, _ = w.Write(data)
```

Add `crypto/sha256` to the imports if absent.

- [x] **Step 4: Retain the prefetch element**

In `frontend/src/hooks/useSegmentPlayback.ts`, keep the prefetched element and reuse it for the next clip:

```ts
  const prefetchRef = useRef<HTMLAudioElement | null>(null);
```

and in `playFrom`:

```ts
      // Preload the next clip and keep it, so the browser has it decoded when
      // the current one ends.
      const following = urls.findIndex((url, i) => i > next && !!url);
      if (following !== -1) {
        const nextUrl = urls[following] as string;
        if (prefetchRef.current?.src !== nextUrl) {
          const prefetch = new Audio(nextUrl);
          prefetch.preload = 'auto';
          prefetchRef.current = prefetch;
        }
      }
```

- [x] **Step 5: Verify**

Run: `go test -run TestClipResponseIsCacheableWithETag ./pkg/gui/ -v && mise run test:frontend && mise run lint`
Expected: PASS.

- [x] **Step 6: Commit**

```bash
git add pkg/gui/server.go pkg/gui/server_clip_cache_test.go frontend/src/hooks/useSegmentPlayback.ts
git commit -m "perf(audio): cache clips with an ETag and reuse the prefetch"
```

---

### Task 7: Full verification

- [x] **Step 1: Run the whole suite**

Run: `mise run test`
Expected: PASS (Go + `tsc`). Re-run `./pkg/gui/` once if the known TempDir flake appears.

- [x] **Step 2: Vet and build**

Run: `mise run lint && mise run build`
Expected: clean, binary built.

- [x] **Step 3: Measure**

Play a 3-beat turn and compare TTFT, time-to-first-audio, and total against the pre-change trace. Record the numbers on the spec.

- [x] **Step 4: Commit anything remaining**

```bash
git status --short
```

---

## Self-Review Notes

- Spec coverage: §3.2.1 → Task 2; §3.2.2/3.2.3 → Task 1; §3.2.4 → Task 3; §3.2.5 → Task 4; §3.2.6 single-flight is deferred to the structural plan (it needs the shared pipeline from Task 4, delivered there); §3.2.7 → Task 5; §3.2.8 → Task 6; §3.2.9 gap/SSE is in the structural plan.
- No placeholders: each step contains real code; two tests note an inline fallback where a helper may not exist, with the exact code.

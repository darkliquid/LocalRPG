# TTS Grouping, Multi-Speaker and Batch Rendering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the group the unit of speech synthesis so a turn issues as few provider requests as a provider will accept, add a Gemini multi-speaker path, and add an offline Batch API backfill, without breaking the content-addressed clip cache, the streaming path, or GUI/export parity.

**Architecture:** Add a capability model and a pure `GroupPlan` to `pkg/media`. Group adjacent same-speaker segments (up to `MaxSpeakers`), render each group in one request (single-speaker via the existing `Synthesize`, multi-speaker via a new optional `GroupTTSClient`), and cache one content-addressed Opus clip per group under a `v4:` key. Add `pkg/ttsbatch` driving an optional `BatchTTSClient` over the Gemini Batch API with jobs persisted in the game DB. Thread clip groups through the GUI DTO and the scene/export model so both resolve identical keys.

**Tech Stack:** Go standard library, `modernc.org/sqlite`, `google.golang.org/genai` (GenerateContent, `MultiSpeakerVoiceConfig`, `client.Batches`), React 19 + TypeScript.

---

### File Map

**Stage 1 — single-speaker grouping**
- **`pkg/media/group.go`** (new): `TTSCapabilities`, `TTSCapabilityReporter`, `SpeakerLine`, `GroupTTSClient`, `ClipGroup`, `ComputeGroupCacheKey`, `GroupPlan`, limit helpers.
- **`pkg/media/group_test.go`** (new): grouping rules, limit splits, key stability, same-voice fallback.
- **`pkg/media/tts.go`**: add `GroupClipKeys`, `SynthesizeGroups`, `SynthesizeTurn`; resolve caps from the client/config.
- **`pkg/media/tts_group_test.go`** (new): pipeline grouping with mock clients.
- **`pkg/config/types.go`**: `TTSConfig.Grouping`, `TTSConfig.MultiSpeaker`, `TTSConfig.Limits`, `TTSLimits`.
- **`pkg/config/types_test.go`**: round-trip and default tests.
- **`pkg/gui/types.go`**: `ClipGroupDTO`; `SegmentDTO.ClipGroup`.
- **`pkg/gui/service.go`**: build clip groups, use `GroupClipKeys`, group-scoped regen.
- **`pkg/gui/turn_audio.go`**: group-aware clip set and enqueue.
- **`pkg/gui/clip_group_test.go`** (new): DTO group keys match pipeline keys.
- **`frontend/src/types.ts`**: `ClipGroupDTO`, `SegmentDTO.clip_group`.
- **`frontend/src/components/SegmentAudioControls.tsx`**: render one control per group.
- **`frontend/src/components/TurnSegments.tsx`**: pass group info to the control.
- **`frontend/src/hooks/useSegmentPlayback.ts`**, **`frontend/src/lib/audio.ts`**: group-scoped playback.
- **`pkg/scene/scene.go`**: `scene.ClipGroup`.
- **`pkg/scene/compile.go`**: build groups, resolve audio once per group.
- **`pkg/export/script.go`**: group-aware `SpeechResolver`.
- **`pkg/export/web.go`**, **`pkg/export/video.go`**: iterate groups for audio.

**Stage 2 — Gemini batch backfill**
- **`pkg/media/batch.go`** (new): `BatchRequest`, `BatchJobHandle`, `BatchStatus`, `BatchResult`, `BatchTTSClient`.
- **`pkg/provider/ttsgemini/batch.go`** (new): `SubmitBatch`/`PollBatch`/`FetchBatch`/`CancelBatch` over `client.Batches` + File API.
- **`pkg/storage/migrate.go`**: migration 10, `tts_jobs`.
- **`pkg/storage/ttsjobs.go`** (new): job record CRUD.
- **`pkg/ttsbatch/engine.go`** (new): plan → JSONL → submit → poll → fetch → cache.
- **`pkg/ttsbatch/engine_test.go`** (new): resumability, partial failure.
- **`cmd/localrpg/tts.go`**: `tts batch` subcommand.
- **`pkg/gui/service.go`**, **`pkg/gui/server.go`**, **`pkg/gui/types.go`**: batch job start/status endpoints.
- **`frontend/src/components/SettingsStudio.tsx`**: batch jobs panel.
- **`pkg/pricing/pricing.go`**, **`pkg/harness/usage.go`**: batch discount marker.

**Stage 3 — Gemini multi-speaker**
- **`pkg/provider/ttsgemini/client.go`**: `SynthesizeGroup`, `TTSCapabilities`.
- **`pkg/provider/ttsgemini/client_test.go`**: multi-speaker request shape.
- **`pkg/media/group.go`**: multi-speaker partitioning, same-voice fallback.

---

## Stage 1 — Single-Speaker Grouping

### Task 1: Capability model and clip group types in `pkg/media`

- [ ] **Step 1.1**: Create `pkg/media/group.go` with the capability and group types:
  ```go
  package media

  type TTSCapabilities struct {
      MaxSpeakers         int
      MaxCharsPerRequest  int
      MaxTokensPerRequest int
      SupportsGrouping    bool
      SupportsBatch       bool
      SupportsStreaming   bool
  }

  type TTSCapabilityReporter interface {
      TTSCapabilities() TTSCapabilities
  }

  type SpeakerLine struct {
      SpeakerID string
      Label     string
      Voice     *entity.VoiceConfig
      Text      string
  }

  type GroupTTSClient interface {
      SynthesizeGroup(ctx context.Context, lines []SpeakerLine) ([]byte, error)
      TTSCapabilities() TTSCapabilities
  }

  type ClipGroup struct {
      SegmentIndexes []int
      Lines          []SpeakerLine
      Key            string
      Cached         bool
  }
  ```
- [ ] **Step 1.2**: Add `ClientCapabilities(client TTSClient) TTSCapabilities` returning the reporter's value, else a single-speaker zero value, and overlay configured limits in a helper `ResolveGroupCaps(cfg config.TTSConfig, client TTSClient) TTSCapabilities` (config `limits` wins when non-zero).
- [ ] **Step 1.3**: Write `pkg/media/group_test.go` asserting a non-reporter client yields `MaxSpeakers == 1`, `SupportsGrouping == false`, and that `ResolveGroupCaps` overlays configured values.
- [ ] **Step 1.4**: Run `go test ./pkg/media/...` and verify it passes.
- [ ] **Step 1.5**: Commit: `git commit -am "feat(media): add tts capability and clip group types"`

---

### Task 2: Content-addressed group key

- [ ] **Step 2.1**: In `pkg/media/group.go`, implement:
  ```go
  // ComputeGroupCacheKey hashes the effective lines of a group. It is
  // segmentation-stable: the same text in the same order under the same voices
  // yields the same key regardless of how the turn was segmented.
  func ComputeGroupCacheKey(provider, model string, lines []SpeakerLine) string
  ```
  Build `canonicalJSON` from `[]struct{ Label, SpeakerID, VoiceID string; Pitch, SpeechRate float64; Options map[string]interface{}; Text string }` (JSON sorts map keys) and hash `"v4:" + provider + "\x00" + model + "\x00" + encoded`.
- [ ] **Step 2.2**: Write tests: identical lines reordered produce different keys; the same text produced from a different segmentation produces the same key; changing a voice, pitch, option, label, or model changes the key; a nil voice is handled.
- [ ] **Step 2.3**: Run `go test ./pkg/media/...` and verify it passes.
- [ ] **Step 2.4**: Commit: `git commit -am "feat(media): add content-addressed group cache key"`

---

### Task 3: Pure `GroupPlan`

- [ ] **Step 3.1**: In `pkg/media/group.go`, implement `GroupPlan`:
  ```go
  func GroupPlan(segments []entity.TurnSegment, narratorVoice *entity.VoiceConfig,
      voiceFor func(speakerID string) *entity.VoiceConfig, caps TTSCapabilities) []ClipGroup
  ```
  Behaviour:
  - Resolve each segment with `prepareSegment`-equivalent logic (speaker ID, voice, `SpeakableTextFor` reduction); skip `ErrNoSpeakableText` segments.
  - Walk in order; add a segment to the current group when its speaker is already present or the group has room under `caps.MaxSpeakers`; otherwise close and start a new group.
  - Close a group when appending would exceed `caps.MaxCharsPerRequest` or `caps.MaxTokensPerRequest`; when a single segment exceeds the limit, split it with `SplitCompleteSentences` into as many groups as needed.
  - Populate `ClipGroup.SegmentIndexes` with the original segment indices.
  - Default `MaxSpeakers` of zero is treated as 1.
- [ ] **Step 3.2**: Write tests in `pkg/media/group_test.go`:
  - Adjacent same-speaker segments merge into one group.
  - A different speaker starts a new group when `MaxSpeakers == 1`.
  - Two speakers alternate into one group when `MaxSpeakers == 2`; a third speaker closes it.
  - A char limit splits a single long segment at a sentence boundary, never mid-sentence.
  - A segment with no speakable text is skipped and does not break adjacency.
- [ ] **Step 3.3**: Run `go test ./pkg/media/...` and verify it passes.
- [ ] **Step 3.4**: Commit: `git commit -am "feat(media): add pure group planning for turn segments"`

---

### Task 4: Pipeline grouping API

- [ ] **Step 4.1**: In `pkg/media/tts.go`, add:
  ```go
  func (p *TTSPipeline) GroupClipKeys(segments []entity.TurnSegment, narratorVoice *entity.VoiceConfig,
      voiceFor func(speakerID string) *entity.VoiceConfig) []ClipGroup
  ```
  Fill each group's `Key` with `ComputeGroupCacheKey(provider, model, lines)` and mark `Cached` from `p.cachedClip`.
- [ ] **Step 4.2**: Add `SynthesizeGroups(ctx, groups []ClipGroup) ([]ClipGroup, error)`:
  - For each uncached group, take the per-key single-flight lock (`keyLock`).
  - Single distinct speaker → `p.client.Synthesize(ctx, concatenatedText, voice)`.
  - Multiple speakers and the client implements `GroupTTSClient` → `SynthesizeGroup(ctx, lines)`; otherwise fall back to per-line `Synthesize` and **do not** write a group clip (the plan collapses the group; see Step 4.4).
  - Normalise with `DecodeProviderAudio` + `opus.Encode`, write via `p.cache.Put("audio", key+".opus", encoded)`, record usage (`p.setLastUsage`).
  - On failure, apply the bisection fallback from the design (§9) before reporting.
- [ ] **Step 4.3**: Add `SynthesizeTurn(ctx, segments, narratorVoice, voiceFor) ([]ClipGroup, error)` = `GroupClipKeys` then `SynthesizeGroups`.
- [ ] **Step 4.4**: Keep `SynthesizeSegmentClips` and `SegmentClipKeys` unchanged; `SynthesizeSegmentClips` remains the per-segment fallback used when a group cannot be rendered.
- [ ] **Step 4.5**: Write `pkg/media/tts_group_test.go` with a mock client that counts `Synthesize` calls: assert a three-sentence single-segment narration issues exactly one request; assert two alternating speakers with `MaxSpeakers == 1` issue two; assert a cache hit issues none.
- [ ] **Step 4.6**: Run `go test ./pkg/media/...` and verify it passes.
- [ ] **Step 4.7**: Commit: `git commit -am "feat(media): group segments into single tts requests"`

---

### Task 5: Configuration

- [ ] **Step 5.1**: In `pkg/config/types.go`, extend `TTSConfig`:
  ```go
  Grouping     string     `yaml:"grouping,omitempty" json:"grouping,omitempty"`
  MultiSpeaker string     `yaml:"multi_speaker,omitempty" json:"multi_speaker,omitempty"`
  Limits       *TTSLimits `yaml:"limits,omitempty" json:"limits,omitempty"`

  type TTSLimits struct {
      MaxChars    int `yaml:"max_chars,omitempty" json:"max_chars,omitempty"`
      MaxTokens   int `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
      MaxSpeakers int `yaml:"max_speakers,omitempty" json:"max_speakers,omitempty"`
  }
  ```
- [ ] **Step 5.2**: Add accessor helpers `TTSGrouping() string` and `TTSMultiSpeaker() string` returning `"auto"` for empty, so call sites never special-case the zero value.
- [ ] **Step 5.3**: Write tests in `pkg/config/types_test.go` for YAML round-trip and the `auto` default.
- [ ] **Step 5.4**: Run `go test ./pkg/config/...` and verify it passes.
- [ ] **Step 5.5**: Commit: `git commit -am "feat(config): add tts grouping and limit settings"`

---

### Task 6: GUI clip groups and DTO

- [ ] **Step 6.1**: In `pkg/gui/types.go`, add `ClipGroupDTO{Key, AudioURLs, SegmentIndexes}` and a `ClipGroup string` field on `SegmentDTO`.
- [ ] **Step 6.2**: In `pkg/gui/service.go`, replace the per-segment `clipKeyResolver` use in `segmentDTOs` with `GroupClipKeys`; populate each segment's `audio_urls` and `clip_group` from the group that covers it, and attach the turn-level group list to the turn DTO.
- [ ] **Step 6.3**: In `pkg/gui/turn_audio.go`, make `finishTurnAudio` use `SynthesizeGroups`; the clip set enqueues each group clip once, and covered segments do not re-enqueue it.
- [ ] **Step 6.4**: Make regen operate on the group key: `GetSegmentClips` for a segment that belongs to a group forces the group.
- [ ] **Step 6.5**: Write `pkg/gui/clip_group_test.go` asserting the DTO's group keys equal `GroupClipKeys` output for the same turn, and that two segments in one group share one URL.
- [ ] **Step 6.6**: Run `go test ./pkg/gui/...` and verify it passes.
- [ ] **Step 6.7**: Commit: `git commit -am "feat(gui): expose clip groups and render one clip per group"`

---

### Task 7: Frontend group-aware controls

- [ ] **Step 7.1**: In `frontend/src/types.ts`, add `ClipGroupDTO` and `SegmentDTO.clip_group`, plus the turn-level group array.
- [ ] **Step 7.2**: In `frontend/src/components/SegmentAudioControls.tsx`, render a single play/stop/regen control for a group; a segment that is not the group's first member renders no control (the hover belongs to the whole group).
- [ ] **Step 7.3**: In `frontend/src/components/TurnSegments.tsx`, group consecutive segments by `clip_group` and pass the group to the control.
- [ ] **Step 7.4**: Update `frontend/src/hooks/useSegmentPlayback.ts` and `frontend/src/lib/audio.ts` so playback and regen address the group's clip URL.
- [ ] **Step 7.5**: Run `cd frontend && npx tsc --noEmit` and verify it passes.
- [ ] **Step 7.6**: Commit: `git commit -am "feat(frontend): one audio control per clip group"`

---

### Task 8: Export and video parity

- [ ] **Step 8.1**: In `pkg/scene/scene.go`, add `ClipGroup{Key string; AudioPaths []string; BeatIndexes []int}` and carry `[]ClipGroup` on the scene or script.
- [ ] **Step 8.2**: In `pkg/scene/compile.go`, build groups from the same `GroupPlan`; call `resolveAudio` once per group and assign the group's clip list and duration to every covered beat.
- [ ] **Step 8.3**: In `pkg/export/script.go`, add a group-aware `SpeechResolver` entry point; keep `SegmentAudio` for the non-grouped case.
- [ ] **Step 8.4**: In `pkg/export/web.go` and the video exporter, iterate groups for audio (one audio input per group) so a shared clip plays once.
- [ ] **Step 8.5**: Add a parity test asserting the export group keys equal the GUI group keys for the same turn.
- [ ] **Step 8.6**: Run `go test ./pkg/scene/... ./pkg/export/...` and verify it passes.
- [ ] **Step 8.7**: Commit: `git commit -am "feat(export): consume grouped clips for parity with the app"`

---

### Task 9: Stage 1 verification and docs

- [ ] **Step 9.1**: Regenerate embedded docs: `go test ./pkg/gui -update-docs`.
- [ ] **Step 9.2**: Run `mise run test` (backend tests + `tsc --noEmit`).
- [ ] **Step 9.3**: Run `mise run lint`.
- [ ] **Step 9.4**: Update the AGENTS.md gotchas with the group-key rule and the `grouping: off` revert.
- [ ] **Step 9.5**: Commit: `git commit -am "docs: document tts grouping and clip groups"`

---

## Stage 2 — Gemini Batch Backfill

### Task 10: `BatchTTSClient` interface

- [ ] **Step 10.1**: Create `pkg/media/batch.go` with `BatchRequest`, `BatchJobHandle`, `BatchStatus`, `BatchResult`, `BatchTTSClient` exactly as specified.
- [ ] **Step 10.2**: Write `pkg/media/batch_test.go` with a fake client asserting the interface is satisfiable and that a result's `Key` maps to a cache entry.
- [ ] **Step 10.3**: Run `go test ./pkg/media/...` and verify it passes.
- [ ] **Step 10.4**: Commit: `git commit -am "feat(media): add batch tts client interface"`

---

### Task 11: `tts_jobs` migration and storage

- [ ] **Step 11.1**: In `pkg/storage/migrate.go`, add `{version: 10, apply: addTTSJobsTable}` creating `tts_jobs` (see the design §8.1) with an index on `(game_id, status)`.
- [ ] **Step 11.2**: Create `pkg/storage/ttsjobs.go` with `UpsertTTSJob`, `GetTTSJob`, `ListTTSJobs(gameID)`, `UpdateTTSJobStatus`.
- [ ] **Step 11.3**: Write `pkg/storage/ttsjobs_test.go` using `t.TempDir()` and `storage.OpenGameStore`.
- [ ] **Step 11.4**: Run `go test ./pkg/storage/...` and verify it passes.
- [ ] **Step 11.5**: Commit: `git commit -am "feat(storage): persist tts batch jobs"`

---

### Task 12: Gemini batch implementation

- [ ] **Step 12.1**: Create `pkg/provider/ttsgemini/batch.go` implementing `BatchTTSClient`: build a JSONL of `generateContent` requests (one per `BatchRequest`, using `SynthesizeGroup`'s request shape), upload via the File API, `client.Batches.Create`, `PollBatch` via `Get`, `FetchBatch` via the output file, `CancelBatch` via `Cancel`.
- [ ] **Step 12.2**: Map the batch discount and token accounting into `media.Usage`.
- [ ] **Step 12.3**: Write tests with a stubbed transport asserting the JSONL shape and status mapping (no live API).
- [ ] **Step 12.4**: Run `go test ./pkg/provider/ttsgemini/...` and verify it passes.
- [ ] **Step 12.5**: Commit: `git commit -am "feat(ttsgemini): support the batch api"`

---

### Task 13: `pkg/ttsbatch` engine

- [ ] **Step 13.1**: Create `pkg/ttsbatch/engine.go`: plan uncached groups, chunk into submissions bounded by token caps and a max request count, write JSONL, submit, poll with exponential backoff, fetch, normalise to Opus, write to the content-addressed cache, update the job row, record usage.
- [ ] **Step 13.2**: Make it resumable: skip groups whose clip is cached; retry only persisted failed keys.
- [ ] **Step 13.3**: Write `pkg/ttsbatch/engine_test.go` with a fake `BatchTTSClient` covering a clean run, a partial failure, and a restart mid-job.
- [ ] **Step 13.4**: Run `go test ./pkg/ttsbatch/...` and verify it passes.
- [ ] **Step 13.5**: Commit: `git commit -am "feat(ttsbatch): add resumable batch backfill engine"`

---

### Task 14: CLI and GUI triggers

- [ ] **Step 14.1**: Add `localrpg tts batch <game-id>` with `--wait`, `--status`, `--cancel` in `cmd/localrpg/tts.go`.
- [ ] **Step 14.2**: Add `Service.StartTTSBatch`, `Service.TTSBatchStatus` in `pkg/gui/service.go` and routes in `pkg/gui/server.go`; update `frontend/src/api/client.ts` and `frontend/src/types.ts` together.
- [ ] **Step 14.3**: Add a batch jobs panel to `frontend/src/components/SettingsStudio.tsx` showing status, counts and cost.
- [ ] **Step 14.4**: Run `go test ./cmd/... ./pkg/gui/...` and `cd frontend && npx tsc --noEmit`.
- [ ] **Step 14.5**: Commit: `git commit -am "feat(gui): start and monitor tts batch jobs"`

---

### Task 15: Batch pricing

- [ ] **Step 15.1**: Add a batch marker to `harness.Usage` and apply a configurable batch multiplier (default 0.5) in `pricing.CostMicros`.
- [ ] **Step 15.2**: Write tests asserting a batch usage record costs half of an equivalent interactive one.
- [ ] **Step 15.3**: Run `go test ./pkg/pricing/... ./pkg/harness/...` and verify it passes.
- [ ] **Step 15.4**: Commit: `git commit -am "feat(pricing): apply the batch api discount"`

---

### Task 16: Stage 2 verification

- [ ] **Step 16.1**: Run `mise run test` and `mise run lint`.
- [ ] **Step 16.2**: Regenerate embedded docs: `go test ./pkg/gui -update-docs`.
- [ ] **Step 16.3**: Commit: `git commit -am "chore: verify tts batch milestone"`

---

## Stage 3 — Gemini Multi-Speaker

### Task 17: Gemini `SynthesizeGroup`

- [ ] **Step 17.1**: In `pkg/provider/ttsgemini/client.go`, implement `TTSCapabilities()` returning `MaxSpeakers: 2`, `SupportsGrouping: true`, `SupportsBatch: true`, `SupportsStreaming: true`, and the documented `MaxCharsPerRequest`/`MaxTokensPerRequest`.
- [ ] **Step 17.2**: Implement `SynthesizeGroup(ctx, lines []SpeakerLine)`: build a `SpeechConfig.MultiSpeakerVoiceConfig` with one `SpeakerVoiceConfig` per line (`Speaker` = label, `voiceConfig.prebuiltVoiceConfig.voiceName` = voice ID) and a labelled transcript whose names match; require exactly two speakers and return an error otherwise.
- [ ] **Step 17.3**: Reuse the existing PCM wrapping and error mapping.
- [ ] **Step 17.4**: Write tests asserting the request carries two `SpeakerVoiceConfig`s with the expected names and voices, and that one speaker is rejected.
- [ ] **Step 17.5**: Run `go test ./pkg/provider/ttsgemini/...` and verify it passes.
- [ ] **Step 17.6**: Commit: `git commit -am "feat(ttsgemini): add multi-speaker synthesis"`

---

### Task 18: Multi-speaker partitioning and gating

- [ ] **Step 18.1**: In `pkg/media/group.go`, extend `GroupPlan` to honour `MaxSpeakers > 1` (already modelled in Task 3) and add the same-voice fallback: when two speakers in a candidate run resolve to the same voice ID, close the group so they render separately.
- [ ] **Step 18.2**: Gate multi-speaker on `media.tts.multi_speaker`: `off` forces `MaxSpeakers = 1`; `auto`/`always` use the provider value.
- [ ] **Step 18.3**: Add tests for the same-voice fallback and the `off` gate.
- [ ] **Step 18.4**: Run `go test ./pkg/media/...` and verify it passes.
- [ ] **Step 18.5**: Commit: `git commit -am "feat(media): partition two-speaker scenes with a same-voice fallback"`

---

### Task 19: Stage 3 verification

- [ ] **Step 19.1**: Run `mise run test` and `mise run lint`.
- [ ] **Step 19.2**: Add an end-to-end test: a two-speaker turn renders in one Gemini request and its group clip resolves identically in the GUI and export.
- [ ] **Step 19.3**: Commit: `git commit -am "chore: verify tts multi-speaker milestone"`

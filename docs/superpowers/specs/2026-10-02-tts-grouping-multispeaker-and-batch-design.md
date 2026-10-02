# TTS Grouping, Multi-Speaker and Batch Rendering Design

**Date:** 2026-10-02
**Status:** Approved
**Scope:** Media speech synthesis: request grouping, multi-speaker scene rendering, offline Batch API backfill, clip/cache model, GUI and export parity
**Related:** `docs/superpowers/specs/2026-09-28-streaming-tts-design.md`, `docs/superpowers/specs/2026-09-29-sentence-scoped-audio-and-streamed-narration-design.md`, `docs/superpowers/specs/2026-09-23-gemini-tts-provider-design.md`, `docs/superpowers/specs/2026-09-26-usage-cost-and-provider-limits-design.md`, `pkg/media/tts.go`, `pkg/media/cache.go`, `pkg/media/sentence.go`, `pkg/provider/ttsgemini/client.go`, `pkg/gui/service.go`, `pkg/scene/compile.go`

---

## 1. Overview & Problem Statement

Speech synthesis is currently split per sentence per speaker. `TTSPipeline.SynthesizeSegmentClips` (`pkg/media/tts.go:254`) resolves a segment to units through `clipUnits` (`pkg/media/tts.go:221`), which calls `SplitSentences` (`pkg/media/sentence.go:16`), and issues one `TTSClient.Synthesize` request per sentence. A twenty-sentence turn is therefore twenty provider requests.

This is the root cause of two systemic problems:

1. **Cost.** Every request re-sends context and pays the per-request overhead. Providers that bill or discount per request (or per token, as Gemini TTS does) are used inefficiently.
2. **Rate limits.** Gemini TTS enforces RPM, TPM and RPD limits. Twenty small requests exhaust a per-request budget far faster than two large ones, and the pipeline has no grouping, no batch path and no multi-speaker path.

Gemini TTS additionally supports **multi-speaker** (exactly two speakers per request), **streaming** (3.1+ models), and the **Batch API** (all TTS models, 50% discount, 24-hour SLO). None are used today. The provider interface, `TTSClient.Synthesize(ctx, text, voice)`, is single-voice only.

This specification makes the **group** the unit of synthesis: the pipeline sends as much text as a provider will accept in one request, groups adjacent segments of the same speaker (and, where supported, of two speakers), and adds an offline Batch API path for backfilling a campaign. It preserves the content-addressed clip cache, the streaming path for local providers, and GUI/export parity.

---

## 2. Design Principles & Invariants

1. **The clip is content-addressed and immutable.** A clip's identity is a hash of everything that changes its audio. Nothing is ever mutated in place; a change produces a new key.
2. **Grouping is additive.** Per-utterance keys remain valid. A provider without grouping, or a configuration with `grouping: off`, behaves exactly as today.
3. **Pure Go, no external model in the default build.** No CGO whisper.cpp and no Python sidecar by default. A multi-speaker response is never split back into per-speaker audio; it is cached and played as one group clip.
4. **Never lose a turn.** A synthesis failure degrades to smaller units and then to silence; it never aborts the turn.
5. **One source of truth for what a clip covers.** A pure `GroupPlan` function is shared by synthesis, the GUI DTO builder and export, so the URL handed to a client and the audio that fills it can never disagree.
6. **Group boundaries are segmentation-stable.** The group key hashes the effective lines, not the segment boundaries, so re-segmenting the same text does not miss the cache.

---

## 3. The Group Model

### 3.1 Units and the grouping rule

- The **segment** (`entity.TurnSegment`) replaces the sentence as the base unit of synthesis for grouping-capable providers.
- A **group** is a maximal run of adjacent segments that a provider can render in a single request. Its text is the concatenation of the segments' speakable text, joined with a sentence-separating space.
- **Adjacency** is strict: a segment that is not adjacent to the current group starts a new group.
- **Speaker rule**: a segment joins the current group when its speaker is already a member, or when the group has fewer than the provider's `MaxSpeakers` distinct speakers. A segment whose speaker would exceed `MaxSpeakers` closes the group and starts a new one.
- **Limit rule**: if adding a segment's text would push the group past `MaxCharsPerRequest` or `MaxTokensPerRequest`, close the group. If a *single* segment exceeds the limit, split that segment's text at the last sentence boundary before the offending sentence (`SplitCompleteSentences`), emitting as many groups as needed. A group never splits mid-sentence.
- A segment that reduces to no speakable text is skipped, exactly as today (`ErrNoSpeakableText`).

### 3.2 The group key

```
GroupKey = "v4:" + sha256(provider + "\x00" + model + "\x00" + canonicalJSON(lines))
lines    = [{label, speakerID, voiceID, pitch, rate, options, text}, ...]   // in order
```

- `provider` is the voice's provider (or the pipeline's provider key); `model` is the provider model.
- `label` is the display name used to address the speaker in a multi-speaker transcript. It changes the provider prompt, so it is part of the key.
- `speakerID`, `voiceID`, `pitch`, `rate`, `options` are the per-line voice identity, the same payload `ComputeAudioCacheKeyForVoice` hashes.
- `text` is the speakable text of that line.
- The key hashes the **effective lines**, so the same total text in the same order under the same voices yields the same key regardless of how it was segmented. `v4:` is a new namespace; existing `v3:` clips remain valid.

A single-line group's key is deliberately **not** the same as the legacy per-utterance key: the group is a distinct cache entry. The per-utterance key remains the fallback for non-grouping providers and for the streaming path.

### 3.3 Clip model and caching

- **One group, one clip.** The provider returns one audio blob for a group; it is normalised to Ogg/Opus (`DecodeProviderAudio` → `opus.Encode`) and written to the content-addressed cache under the group key, exactly as a per-utterance clip is today.
- **Segment → clip mapping.** Every segment in a group resolves to the same clip key. A segment's clip list is the group's clip list (length one for a grouped clip), preserving the `[]string` shape the DTO, export and video code already consume.
- **Additive.** Nothing is deleted. Disabling grouping leaves old group clips in the cache, unused and harmless.
- **No splitting.** Because the default build never splits a multi-speaker response, a multi-speaker group is played as one clip. This is a deliberate trade: cost and request-count wins over per-speaker clip granularity.

---

## 4. Provider Capability Model

New optional interfaces in `pkg/media`, matching the existing `SpeechCueAdvertiser`/`VoiceCatalog` opt-in pattern. A client that implements none of them keeps today's per-sentence behaviour.

### 4.1 `TTSCapabilities`

```go
type TTSCapabilities struct {
    MaxSpeakers         int  // 1 for single-voice providers; 2 for Gemini
    MaxCharsPerRequest  int  // 0 means unbounded / unknown
    MaxTokensPerRequest int  // 0 means unbounded / unknown
    SupportsGrouping    bool // may accept a multi-segment, single-speaker group
    SupportsBatch       bool // implements BatchTTSClient
    SupportsStreaming   bool // provider-side chunked audio (advertised, unused)
}
```

Advertised by an optional interface, so the pipeline can discover limits without a type switch:

```go
type TTSCapabilityReporter interface {
    TTSCapabilities() TTSCapabilities
}
```

### 4.2 Single-speaker grouping needs no provider code

The existing `TTSClient.Synthesize(ctx, text, voice)` already accepts arbitrary text. A single-speaker group is therefore rendered by concatenating its lines and calling `Synthesize` once. Every provider with a known `MaxCharsPerRequest` benefits immediately, with no provider changes.

### 4.3 `GroupTTSClient` (multi-speaker only)

```go
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
```

The provider owns prompt construction: for Gemini, a `SpeechConfig.MultiSpeakerVoiceConfig` with one `SpeakerVoiceConfig` per line, and a labelled transcript (`<Label>: <text>`) whose speaker names match `SpeakerVoiceConfig.Speaker`. The pipeline never builds the provider prompt.

`SynthesizeGroup` is used only when a group has more than one distinct speaker. A single-speaker group always goes through `Synthesize`.

### 4.4 `BatchTTSClient`

```go
type BatchRequest struct {
    Key   string
    Lines []SpeakerLine
}

type BatchJobHandle struct {
    ID    string
    Model string
}

type BatchStatus struct {
    State     string // pending|running|succeeded|failed|cancelled|expired
    Total     int
    Completed int
}

type BatchResult struct {
    Key   string
    Audio []byte
    Err   error
}

type BatchTTSClient interface {
    SubmitBatch(ctx context.Context, reqs []BatchRequest) (BatchJobHandle, error)
    PollBatch(ctx context.Context, h BatchJobHandle) (BatchStatus, error)
    FetchBatch(ctx context.Context, h BatchJobHandle) ([]BatchResult, error)
    CancelBatch(ctx context.Context, h BatchJobHandle) error
}
```

The interface is provider-agnostic; Gemini is the first implementer.

---

## 5. Pipeline Placement

New, in `pkg/media`:

```go
type ClipGroup struct {
    SegmentIndexes []int
    Lines          []SpeakerLine
    Key            string
    Cached         bool
}

// GroupPlan is pure: it maps a turn's segments to groups under a provider's
// capabilities. It synthesizes nothing and is shared by every caller.
func GroupPlan(segments []entity.TurnSegment, narratorVoice *entity.VoiceConfig,
    voiceFor func(speakerID string) *entity.VoiceConfig, caps TTSCapabilities) []ClipGroup
```

- `GroupPlan` resolves speakers and voices exactly as `prepareSegment` does today (`pkg/media/tts.go:281`), reuses `SpeakableTextFor` for reduction, and applies the rules in §3.1.
- `TTSPipeline` gains:
  - `GroupClipKeys(segments, narrator, voiceFor) []ClipGroup` — `GroupPlan` with `Key` filled, no synthesis, for the DTO builder (mirrors `SegmentClipKeys`).
  - `SynthesizeGroups(ctx, groups []ClipGroup) ([]ClipGroup, error)` — renders each uncached group (single-speaker via `Synthesize`, multi-speaker via `SynthesizeGroup`), normalises to Opus, writes the cache, returns groups with `Key` and `Cached` set.
  - `SynthesizeTurn(ctx, segments, narrator, voiceFor) ([]ClipGroup, error)` — `GroupPlan` + `SynthesizeGroups`, the turn-level entry point.
- `SynthesizeSegmentClips` and `SegmentClipKeys` are retained unchanged for the one-segment and streaming paths.
- The group cap (`MaxCharsPerRequest`, `MaxSpeakers`) is resolved from the provider's `TTSCapabilities`, overridden by `media.tts.limits.*` when set, and defaults to unbounded/one-speaker when the provider advertises nothing and grouping is not forced.

---

## 6. Multi-Speaker Scene Partitioning

- A group may hold up to `MaxSpeakers` distinct speakers; the §3.1 speaker rule already expresses this. The narrator is an ordinary speaker, not privileged.
- Fall back to single-speaker groups when:
  - `MaxSpeakers < 2`, or
  - two speakers in a candidate run resolve to the **same voice ID** (indistinguishable output), or
  - the provider does not implement `GroupTTSClient`.
- Gated by `media.tts.multi_speaker`: `auto` (default) uses multi-speaker when a run has two distinct speakers and the provider supports it; `off` disables it; `always` forces it where supported.
- Gemini's two-speaker ceiling is a hard provider limit. A run with three or more distinct speakers becomes a sequence of two-speaker groups (for example A,B then B,C then C,...), never more than two per request.

---

## 7. Realtime vs Offline Paths

- **Grouping is the primary strategy for remote/metered providers.** For these providers the turn is synthesized at finalise through `SynthesizeTurn`. This is compatible with the existing default: `stream_sentences` is nil-enabled *except* for metered providers (`pkg/config/types.go:167`), and Gemini is metered, so sentence streaming is already off for it.
- **Sentence streaming remains the strategy for local/free providers.** `pkg/gui/streaming_tts.go` is unchanged; the `sentenceStreamer` keeps pre-synthesizing narration sentences for providers where latency matters more than request count.
- **The two are mutually exclusive per turn.** A tri-state `media.tts.grouping` selects the strategy:
  - `auto` (default): group when the provider advertises grouping (or is metered) and streaming was not explicitly forced.
  - `off`: today's per-sentence behaviour, streaming allowed.
  - `always`: group regardless, streaming disabled for the turn.
- Provider-side streaming (`generate_content_stream`) is **deferred**; it reduces latency, not request count, and composes awkwardly with grouping. `SupportsStreaming` is advertised but unused.

---

## 8. Offline Batch Subsystem (`pkg/ttsbatch`)

A resumable, idempotent engine over `BatchTTSClient`, for backfilling a campaign or exporting without spending interactive quota.

### 8.1 Job model

One job is one submission: a JSONL file of `BatchRequest`s, one line per uncached group. A `tts_jobs` table (storage migration 10) tracks it:

```
tts_jobs(
  id            TEXT PRIMARY KEY,   -- provider batch job id
  game_id       TEXT NOT NULL,
  provider      TEXT NOT NULL,
  model         TEXT NOT NULL,
  status        TEXT NOT NULL,      -- submitted|running|succeeded|failed|cancelled
  input_uri     TEXT,               -- File API URI of the uploaded JSONL
  request_count INTEGER NOT NULL DEFAULT 0,
  completed     INTEGER NOT NULL DEFAULT 0,
  failed_keys   TEXT NOT NULL DEFAULT '',  -- newline-separated group keys
  cost_micros   INTEGER NOT NULL DEFAULT 0,
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
)
```

The **cache is the source of truth for completion**: a group whose clip exists is done. The job row tracks only what is in flight and what failed, so a restart resumes cleanly.

### 8.2 Lifecycle

The job row's `status` is a lifecycle phase, so a manager can say what stage a job is at rather than only that the provider accepted it:

`queued` → `processing` → `processed` → `downloading` → `storing` → `completed` (or `failed`/`cancelled`/`expired`). **`completed` means the clips are downloaded *and* stored in the local cache**; a job whose provider has finished but whose output is not yet fetched is `processed`. The download and store therefore start automatically on the `processed` transition, and the store reports progress as each clip lands, so "0 of 8" is never mistaken for "done".

1. `GroupPlan` the campaign's turns; keep groups whose key is not cached, unless `Force` is set, in which case every group is submitted and its clip overwrites the cached one.
2. Write a JSONL file (bounded by the provider's request cap and a configurable max requests per job) and upload it through the File API.
3. `SubmitBatch`; persist the job row as `queued`.
4. Poll, updating the phase as the provider's state changes (`processing`, then `processed`); a cancelled poll leaves the job resumable.
5. On `processed`, fetch the output (`downloading`), normalise each result to Opus and write it to the content-addressed cache (`storing`, reporting progress per clip), then mark `completed` with the stored and failed counts.
6. On partial failure, persist the failed keys and retry them in a later job.

A job is resumable while its phase is in flight (`queued`/`processing`/`processed`/`downloading`/`storing`), or while it is a finished row whose stored plus failed count is below its request count. `ResumePendingBatches` runs at launch and finishes such jobs in the background, skipping any whose recorded provider differs from the configured one (a different provider's client cannot poll it). Starting a backfill while one is already in flight returns the existing job rather than queueing a second, so the action is idempotent.

### 8.3 Triggering

- CLI: `localrpg tts batch <game-id> [--wait] [--status] [--cancel]`.
- GUI: an action in the TTS settings that starts a job and a jobs panel that shows status, counts and cost.
- Export: an option to backfill via batch before rendering.

Usage is recorded with a batch marker so pricing can apply the 50% discount (see §11).

---

## 9. Failure Handling & Rate Limits

Group-level failures degrade by bisection, preserving "never lose a turn":

1. A group request fails. Classify the error with the existing taxonomy (`harness.ClassifyProviderError`, `pkg/harness/failure.go`).
2. **Rate limit / `RESOURCE_EXHAUSTED` (429):** record a `LimitRegistry` block for provider+role with the advertised `RetryAfterMS`; retry the group intact.
3. **Token-limit / 400:** bisect the group at its largest sentence boundary and retry each half. Recurse until a single segment remains.
4. **Single segment still fails:** fall back to per-sentence synthesis (today's path).
5. **All units fail:** the segment is silent, as today; the turn is never aborted.
6. **Auth / not-found:** surface immediately; do not retry.

Batch jobs report per-line results; a failed line is recorded and retried on the next run, while successful lines are already in the cache.

---

## 10. GUI: Clip Groups, Playback and Regen

- `SegmentDTO` keeps `audio_urls` (now usually length one for a grouped clip) and gains `clip_group` (the group key or index).
- A new turn-level `ClipGroupDTO` is the cleanest target for group-scoped controls:

```go
type ClipGroupDTO struct {
    Key            string   `json:"key"`
    AudioURLs      []string `json:"audio_urls"`
    SegmentIndexes []int    `json:"segment_indexes"`
}
```

- The frontend renders **one** play/stop/regen control per group, spanning all its segments, rather than one per segment. This is the UI rule: the hover maps to the actually available clip.
- **Regen targets the group**: delete the group clip and force-synthesize the group; every segment in it updates. There is no per-segment regen inside a multi-segment group. A single-segment group behaves exactly as today.
- A voice or options change alters the group key, so the next synthesis is a miss and auto-regenerates; no manual invalidation is needed.
- The DTO builder uses `GroupClipKeys`, so the URLs it hands out before synthesis are the same keys `SynthesizeGroups` will fill.

---

## 11. Export & Video Parity

- Beats stay per-segment for text, portraits and timing. A new scene-level grouping links the beats a group covers:

```go
type ClipGroup struct {
    Key         string
    AudioPaths  []string
    BeatIndexes []int
}
```

- `SpeechResolver` gains a group-aware entry point; the scene compiler builds groups from the same `GroupPlan`, so a clip is never synthesized twice and export matches the GUI exactly.
- The web and video exporters iterate **groups** for audio (one audio input per group) while advancing all covered beats together, so a shared clip plays once rather than once per beat.
- `Beat.AudioPaths` for a grouped beat is the group's clip list; `Beat.AudioDuration` is the group's duration shared by its beats.

---

## 12. Configuration

New on `config.TTSConfig`:

```go
Grouping     string     `yaml:"grouping,omitempty"`      // auto (default) | off | always
MultiSpeaker string     `yaml:"multi_speaker,omitempty"` // auto (default) | off | always
Limits       *TTSLimits `yaml:"limits,omitempty"`

type TTSLimits struct {
    MaxChars    int `yaml:"max_chars,omitempty"`
    MaxTokens   int `yaml:"max_tokens,omitempty"`
    MaxSpeakers int `yaml:"max_speakers,omitempty"`
}
```

- `grouping: auto` groups when the provider advertises grouping or is metered; `off` reverts to per-sentence; `always` forces grouping.
- `multi_speaker: auto` uses multi-speaker when a run has two distinct speakers and the provider supports it.
- `limits` overrides advertised caps, for proxied or self-hosted endpoints whose limits cannot be queried.
- Batch has no config beyond pricing: it is driven by CLI and the GUI action.
- Settings Studio exposes the grouping and multi-speaker toggles in the TTS tab, the limits override, and a batch jobs panel.
- Regenerate the embedded docs after the struct change: `go test ./pkg/gui -update-docs`.

---

## 13. Staging & Rollout

**Stage 1 — single-speaker grouping everywhere (the cost fix).** `GroupPlan`, `SynthesizeTurn`, the capability struct, the DTO/export changes, and config, behind `grouping: auto`. No provider code changes; every provider benefits. This alone collapses a turn's requests to roughly one per speaker-run.

**Stage 2 — Gemini batch backfill.** `pkg/ttsbatch`, `BatchTTSClient`, the `tts_jobs` migration, CLI and GUI.

**Stage 3 — Gemini multi-speaker.** `GroupTTSClient`, `SynthesizeGroup`, multi-speaker partitioning, and the `multi_speaker` gate.

Each stage ships independently behind its flag and reverts with `off`.

---

## 14. Non-Goals / Deferred

- **Audio diarization / forced alignment.** No CGO whisper.cpp and no Python sidecar in the default build. The pipeline never splits a multi-speaker response. A pluggable aligner interface may be added later; it is out of scope here.
- **Provider-side audio streaming** (`generate_content_stream`, Gemini 3.1+). Advertised via `SupportsStreaming` but unused.
- **More than two speakers per request.** Gemini guarantees two; quality beyond that is unspecified, so groups never exceed two distinct speakers.
- **Per-speaker clips from a multi-speaker group.** A group clip is opaque and plays as one unit by design.

---

## 15. Success Criteria

- A turn whose narration is one long paragraph issues **one** synthesis request instead of one per sentence, with identical audio quality and no cache regressions for existing clips.
- A metered provider's request count per turn drops by roughly the number of sentences per speaker-run, observable in usage records.
- `grouping: off` reproduces today's behaviour exactly, byte for byte in cache keys.
- Export and the GUI resolve identical clip keys for the same turn (parity test).
- A Gemini batch job backfills an uncached campaign at the discounted rate, resuming after a restart without re-submitting completed groups.
- A two-speaker scene renders in a single Gemini request via `SynthesizeGroup`.
- No test requires a network, a GPU, or CGO.

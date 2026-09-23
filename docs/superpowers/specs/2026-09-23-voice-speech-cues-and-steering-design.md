# Voice Speech Steering Cues & Provider Capabilities Design

**Date:** 2026-09-23  
**Status:** Approved  
**Topic:** Configurable TTS Speech Cues, Provider Capabilities Advertisement, GM Voice Steering, and Transcript Rendering  

---

## 1. Background & Motivation

LocalRPG turns are voiced through a uniform Text-to-Speech (TTS) pipeline that supports multiple providers (including local in-process engines like `sherpa-onnx` with Kokoro models, local HTTP endpoints like Kokoro FastAPI, and remote cloud providers like ElevenLabs).

Different TTS engines have vastly different capabilities regarding vocal performance and steering hints:
- **ElevenLabs** (particularly Eleven v3) interprets bracketed performance directions—such as `[whispers]`, `[sighs]`, `[laughs]`, `[clears throat]`, `[gasp]`, `[excited]`, `[angry]`—as vocal acting directions rather than text to be spoken.
- **Kokoro FastAPI / Markdown-Aware HTTP**: Interprets Markdown emphasis (e.g. `*emphasis*`, `**strong**`) for cadence and volume modulation.
- **Sherpa-ONNX (Embedded Kokoro)**: Pure plain-text phonemizer; does not support speech steering tags or Markdown syntax. Sending bracketed tags like `[whispers]` causes the engine to literally pronounce *"left bracket whispers right bracket"*.

Currently, LocalRPG's GM prompt assembly statically instructs the model:
> *"Do not write voice IDs, voice tags, or profile names into the narration."*

This prevents modern models (like Eleven v3) from steering emotive delivery. Furthermore, if a GM *does* produce stage directions or tags, there is no mechanism to strip them out when sending text to engines that do not support them, nor is there a structured way for the frontend reader to display them cleanly as stage directions instead of raw text.

---

## 2. Goals & Non-Goals

### Goals
1. **Provider Capabilities Advertising**: Allow TTS clients to advertise what speech cue formats they interpret (`AudioTags`, `MarkdownEmphasis`, canonical tag lists, and guidance).
2. **Dynamic GM Prompt Steering**: Tailor the GM prompt instructions to the active provider's enabled capabilities—instructing the GM on valid tags/cues when supported, or strictly forbidding them when unsupported.
3. **Audio Synthesis Sanitization**: Ensure that segments sent to non-supporting TTS engines have bracketed audio tags stripped cleanly so no engine mispronounces syntax characters.
4. **Configurable Transcript Rendering**: Allow players to choose how speech cues are displayed in the story reader: as styled stage directions (`stage_directions`), completely hidden (`hidden`), or unstyled (`raw`).
5. **Settings Studio Integration**: Surface advertised capabilities in the Settings UI and allow players to toggle cues and configure transcript display preferences.

### Non-Goals
- Modifying the canonical `history.jsonl` turn segment schema: the raw GM prose remains canonical; sanitization and formatting happen dynamically at synthesis and display time.
- Custom phoneme/IPA authoring in the prompt.
- Restricting speech cues solely to dialogue: both dialogue and narrative beats support steering if the corresponding voice supports it.

---

## 3. Architecture & Data Flow

```mermaid
flowchart TD
    Config["TTSConfig (speech_cues settings)"] --> Resolver["Effective Capabilities Resolver"]
    Client["TTSClient (SpeechCueAdvertiser)"] --> Resolver
    
    Resolver -->|Effective Cues| Orchestrator["TurnOrchestrator"]
    Orchestrator -->|ContextRequest| Harness["harness.AssembleContextWithProfiles"]
    Harness -->|Dynamic Instructions| GM["GM Model (Generate)"]
    
    GM -->|Raw Prose with [whispers]| Timeline["Timeline.RecordTurn (history.jsonl)"]
    
    Timeline -->|Raw TurnSegments| AudioPipeline["TTSPipeline (SpeakableTextFor)"]
    Resolver -->|Supports AudioTags?| AudioPipeline
    AudioPipeline -->|If supported: [whispers] 'Hi'| SynthTagged["TTSClient.Synthesize (ElevenLabs)"]
    AudioPipeline -->|If unsupported: 'Hi'| SynthStripped["TTSClient.Synthesize (Sherpa)"]
    
    Timeline -->|Raw TurnSegments| Frontend["TurnSegments / MarkdownProse"]
    Config -->|display_mode| Frontend
    Frontend -->|stage_directions| RenderStyled["Render: [WHISPERS] 'Hi'"]
    Frontend -->|hidden| RenderHidden["Render: 'Hi'"]
    Frontend -->|raw| RenderRaw["Render: [whispers] 'Hi'"]
```

---

## 4. Subsystems Detail

### 4.1 Provider Capabilities Interface (`pkg/media`)

In `pkg/media/tts.go`:

```go
// SpeechCueCapabilities describes the vocal steering hints a TTS engine can interpret.
type SpeechCueCapabilities struct {
	AudioTags        bool     // E.g. [whispers], [sighs], [laughs], [excited]
	MarkdownEmphasis bool     // E.g. *whispers*, **shouts**
	SupportedTags    []string // Canonical list of tags (if provider has a known set)
	PromptGuidance   string   // Optional provider-specific steering advice for the GM
}

// SpeechCueAdvertiser is an optional interface implemented by TTS clients that
// declare speech cue capabilities.
type SpeechCueAdvertiser interface {
	SpeechCueCapabilities() SpeechCueCapabilities
}
```

#### Provider Implementations:
1. **`ElevenLabsTTSClient`** (`pkg/media/elevenlabs_tts.go`):
   - Implements `SpeechCueAdvertiser`.
   - Returns:
     ```go
     SpeechCueCapabilities{
         AudioTags: true,
         MarkdownEmphasis: false,
         SupportedTags: []string{
             "whispers", "sighs", "laughs", "gasp", "clears throat",
             "chuckles", "softly", "loudly", "excited", "angry",
             "nervous", "sad", "playful", "tired",
         },
         PromptGuidance: "Use bracketed tags immediately before dialogue or narration beats to steer performance.",
     }
     ```
2. **`SherpaTTSClient`** (`pkg/media/sherpa_tts.go`):
   - Implements `SpeechCueAdvertiser`.
   - Returns:
     ```go
     SpeechCueCapabilities{
         AudioTags: false,
         MarkdownEmphasis: false,
     }
     ```
3. **`httpTTSClient`** (`pkg/media/providers.go`):
   - Implements `SpeechCueAdvertiser`.
   - Returns:
     ```go
     SpeechCueCapabilities{
         AudioTags: false,
         MarkdownEmphasis: true, // or dynamically configured from options
     }
     ```
4. **Fallback for clients without `SpeechCueAdvertiser`**:
   - Inspects `MarkdownAware` interface: if `SupportsMarkdown()` is true, `MarkdownEmphasis` is true; `AudioTags` defaults to `false`.

---

### 4.2 Configuration Schema (`pkg/config`, `frontend/src/types.ts`)

In `pkg/config/types.go`:

```go
type SpeechCuesConfig struct {
	Enabled          bool   `yaml:"enabled" json:"enabled"`                                 // Master toggle (default true)
	AudioTags        *bool  `yaml:"audio_tags,omitempty" json:"audio_tags,omitempty"`         // Override provider default
	MarkdownEmphasis *bool  `yaml:"markdown_emphasis,omitempty" json:"markdown_emphasis,omitempty"` // Override provider default
	DisplayMode      string `yaml:"display_mode,omitempty" json:"display_mode,omitempty"`     // "stage_directions" (default), "hidden", "raw"
}

type TTSConfig struct {
	// ... existing fields ...
	SpeechCues SpeechCuesConfig `yaml:"speech_cues,omitempty" json:"speech_cues,omitempty"`
}
```

#### Resolution Logic:
```go
func ResolveEffectiveSpeechCues(cfg config.SpeechCuesConfig, client media.TTSClient) media.SpeechCueCapabilities {
	if !cfg.Enabled {
		return media.SpeechCueCapabilities{}
	}
	var caps media.SpeechCueCapabilities
	if adv, ok := client.(media.SpeechCueAdvertiser); ok {
		caps = adv.SpeechCueCapabilities()
	} else if aware, ok := client.(media.MarkdownAware); ok && aware.SupportsMarkdown() {
		caps.MarkdownEmphasis = true
	}

	if cfg.AudioTags != nil {
		caps.AudioTags = *cfg.AudioTags
	}
	if cfg.MarkdownEmphasis != nil {
		caps.MarkdownEmphasis = *cfg.MarkdownEmphasis
	}
	return caps
}
```

---

### 4.3 Prompt Orchestration & GM Context Steering (`pkg/harness`, `pkg/engine`)

In `pkg/harness/context.go`:

```go
type SpeechCueContext struct {
	AudioTags        bool
	MarkdownEmphasis bool
	SampleTags       []string
	CustomGuidance   string
}
```

We add `SpeechCues SpeechCueContext` to `ContextRequest`.

`FormatSpeechFormattingInstructions(cues SpeechCueContext) string` replaces static `speechFormattingInstruction`:
- **When `AudioTags` is enabled**:
  ```markdown
  ## VOICE ACTING & SPEECH STEERING
  You may steer the vocal delivery of spoken lines and narration beats using bracketed
  performance tags immediately before dialogue or delivery. Common supported cues:
  - Delivery / Volume: [whispers], [softly], [shouts], [loudly]
  - Reactions: [sighs], [laughs], [chuckles], [gasp], [clears throat]
  - Moods / Emotions: [excited], [angry], [sad], [nervous], [playful], [tired]
  Example: Garrick: "[whispers] Keep your head down."
  Example: [sighs] It has been a long winter in the northern reaches.
  Use cues purposefully to enhance drama; do not clutter every sentence.
  ```
- **When `MarkdownEmphasis` is enabled**:
  ```markdown
  You may use *single asterisks* for vocal emphasis and pacing.
  ```
- **When cues are disabled or unsupported**:
  ```markdown
  Do not write stage directions, voice tags, brackets, or profile names into the narration or dialogue (e.g. do not write [whispers]), as the voice synthesizer will mispronounce them.
  ```

In `pkg/engine/orchestrator.go`:
- `TurnOrchestrator` holds `speechCues harness.SpeechCueContext` (configured via `SetSpeechCues`).
- Passed to `ContextRequest.SpeechCues` during `ProcessAction`.

---

### 4.4 Audio Synthesis Pipeline & Sanitization (`pkg/media/speakable.go`)

In `pkg/media/speakable.go`:

```go
var audioTagRe = regexp.MustCompile(`(?i)\[[a-z][a-z\s_-]{1,28}\]`)

// StripAudioTags removes bracketed performance tags and normalizes whitespace.
func StripAudioTags(text string) string {
	if text == "" {
		return ""
	}
	replaced := audioTagRe.ReplaceAllString(text, " ")
	return strings.Join(strings.Fields(replaced), " ")
}
```

In `SpeakableTextFor(policy TextPolicy, client TTSClient, text string) string`:
1. If `client` does not implement `SpeechCueAdvertiser` with `AudioTags: true`, call `StripAudioTags`.
2. Preserves cache key integrity: `ComputeAudioCacheKeyForVoice` receives the clean text when stripped, and the tagged text when preserved, avoiding collisions.

---

### 4.5 Frontend UI Rendering (`frontend/src/components/MarkdownProse.tsx`, `TurnSegments.tsx`)

In `frontend/src/components/MarkdownProse.tsx`:
- Add `displayMode?: 'stage_directions' | 'hidden' | 'raw'` prop.
- In `'stage_directions'` mode (default):
  - Add single-bracket tags `(\[[a-zA-Z][a-zA-Z\s_-]{1,28}\])` to inline tokenization.
  - Render as a styled stage-direction chip:
    ```tsx
    <span
      key={key++}
      className="inline-flex items-center text-[0.78em] font-sans font-semibold uppercase tracking-wider text-amber-400/90 bg-amber-950/40 border border-amber-700/40 px-1.5 py-0.2 rounded-md mx-1 select-none not-italic align-baseline"
      title="Voice performance cue"
    >
      {tagText}
    </span>
    ```
- In `'hidden'` mode:
  - Strip tags before rendering blocks: `text.replace(/\[[a-zA-Z][a-zA-Z\s_-]{1,28}\]/g, '')`.
- In `'raw'` mode:
  - Keep unstyled text `[whispers]`.

In `frontend/src/components/TurnSegments.tsx`:
- Passes `displayMode` from active game settings or global TTS configuration to `MarkdownProse`.

---

### 4.6 Settings Studio (`frontend/src/components/SettingsStudio.tsx`)

Under the **Text-to-Speech** section:
- Shows provider's advertised capabilities banner returned from `/api/tts/inspect`.
- Master toggle for **Speech Steering Cues** (`speech_cues.enabled`).
- Selector for **Transcript Display Mode** (`stage_directions` | `hidden` | `raw`).
- Optional override checkboxes for `Audio Tags` and `Markdown Emphasis`.

---

## 5. Error Handling & Edge Cases

1. **Provider Switching**:
   - If a campaign has turns generated with `[whispers]` and the player switches provider to `sherpa-onnx`, `SpeakableTextFor` automatically strips `[whispers]` before synthesis. Sherpa will synthesize `"Keep low"` cleanly rather than pronouncing the brackets.
2. **Missing or Mismatched Brackets**:
   - Wikilinks (`[[target|label]]`) use double brackets; `audioTagRe` only matches single brackets with letter boundaries (`[a-z][a-z\s_-]{1,28}]`), so wikilinks are never confused with audio tags.
3. **Empty Spoken Text**:
   - If a segment consisted solely of an audio tag (e.g. `"[sighs]"` with no dialogue), stripping it yields empty string, which `SynthesizeSegments` safely skips via `ErrNoSpeakableText` without erroring.

---

## 6. Verification & Test Plan

1. **Unit Tests (`pkg/media`)**:
   - Test `StripAudioTags` with various tags (`[whispers]`, `[clears throat]`, `[sighs]`).
   - Test `SpeakableTextFor` with `AudioTags` enabled (preserves tags) vs disabled (strips tags).
   - Test `ElevenLabsTTSClient`, `SherpaTTSClient`, and `httpTTSClient` capability advertising.
2. **Prompt Assembly Tests (`pkg/harness`)**:
   - Verify prompt output when `AudioTags` is true (includes steering guide and examples).
   - Verify prompt output when `AudioTags` is false (includes explicit prohibition).
3. **Frontend Tests**:
   - Verify `MarkdownProse` renders stage direction chips in `'stage_directions'` mode.
   - Verify `MarkdownProse` strips tags in `'hidden'` mode.
   - Run `mise run test:frontend` and `mise run test:backend`.

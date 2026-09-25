# Voice Speech Steering Cues & Provider Capabilities Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow TTS providers to advertise speech cue capabilities (e.g. ElevenLabs audio tags `[whispers]`, Kokoro Markdown emphasis `*bold*`), dynamically steer the GM prompt with instructions and examples when supported, sanitize audio text before sending to non-supporting engines, and render speech cues in the transcript as styled stage directions, hidden, or raw text.

**Architecture:** A provider capability interface (`SpeechCueAdvertiser`) returns `SpeechCueCapabilities`. The configuration layer (`SpeechCuesConfig`) allows user overrides and display preferences. The engine layer dynamically injects performance directions into the GM prompt. The audio synthesis pipeline (`SpeakableTextFor`) strips unsupported audio tags before synthesis so non-supporting engines never mispronounce brackets. The frontend (`MarkdownProse`) formats audio tags into distinct stage-direction chips or strips them according to `display_mode`.

**Tech Stack:** Go 1.27.1, TypeScript 5.8, React 19, Tailwind CSS v4, Vite.

---

## File Map

| Path | Action | Responsibility |
|---|---|---|
| `pkg/media/tts.go` | Modify | Define `SpeechCueCapabilities` and `SpeechCueAdvertiser` interface |
| `pkg/media/elevenlabs_tts.go` | Modify | Implement `SpeechCueAdvertiser` returning `AudioTags: true` and canonical tag set |
| `pkg/media/sherpa_tts.go` | Modify | Implement `SpeechCueAdvertiser` returning `AudioTags: false, MarkdownEmphasis: false` |
| `pkg/media/providers.go` | Modify | Implement `SpeechCueAdvertiser` on `httpTTSClient` |
| `pkg/media/speech_cues_test.go` | Create | Test `SpeechCueAdvertiser` implementations across TTS clients |
| `pkg/media/speakable.go` | Modify | Define `audioTagRe`, `StripAudioTags`, `ClientSupportsAudioTags`, enhance `SpeakableTextFor` |
| `pkg/media/speakable_cues_test.go` | Create | Test `StripAudioTags` and `SpeakableTextFor` stripping vs preserving tags |
| `pkg/config/types.go` | Modify | Add `SpeechCuesConfig` and wire into `TTSConfig` |
| `pkg/harness/context.go` | Modify | Define `SpeechCueContext`, add to `ContextRequest`, implement `FormatSpeechFormattingInstructions` |
| `pkg/harness/speech_cues_test.go` | Create | Test prompt instruction generation under different cue capabilities |
| `pkg/engine/orchestrator.go` | Modify | Add `speechCues` field, `SetSpeechCues`, pass to `ContextRequest.SpeechCues` |
| `pkg/gui/types.go` | Modify | Add `SpeechCueCapabilities` to `TTSInspectResponseDTO` |
| `pkg/gui/service.go` | Modify | Populate speech cue capabilities in inspect response and wire orchestrator cues |
| `frontend/src/types.ts` | Modify | Add `SpeechCuesConfig` and `SpeechCueCapabilities` to types |
| `frontend/src/components/MarkdownProse.tsx` | Modify | Support `displayMode` prop, render stage direction chips or strip tags |
| `frontend/src/components/TurnSegments.tsx` | Modify | Pass `displayMode` to `MarkdownProse` |
| `frontend/src/components/SettingsStudio.tsx` | Modify | Add Speech Steering Cues panel with capability banner, toggles, and display mode selector |

---

## Tasks

### Task 1: Provider Capability Interface & Advertisers

**Files:**
- Modify: `pkg/media/tts.go`
- Modify: `pkg/media/elevenlabs_tts.go`
- Modify: `pkg/media/sherpa_tts.go`
- Modify: `pkg/media/providers.go`
- Create: `pkg/media/speech_cues_test.go`

- [x] **Step 1: Write unit tests for `SpeechCueAdvertiser` in `pkg/media/speech_cues_test.go`**

```go
package media

import (
	"testing"
)

func TestSpeechCueAdvertiserImplementations(t *testing.T) {
	eleven := &ElevenLabsTTSClient{}
	adv, ok := interface{}(eleven).(SpeechCueAdvertiser)
	if !ok {
		t.Fatal("expected ElevenLabsTTSClient to implement SpeechCueAdvertiser")
	}
	caps := adv.SpeechCueCapabilities()
	if !caps.AudioTags {
		t.Errorf("expected ElevenLabs to support AudioTags, got false")
	}
	if len(caps.SupportedTags) == 0 {
		t.Errorf("expected ElevenLabs to advertise SupportedTags, got empty")
	}

	sherpa := &SherpaTTSClient{}
	advSherpa, ok := interface{}(sherpa).(SpeechCueAdvertiser)
	if !ok {
		t.Fatal("expected SherpaTTSClient to implement SpeechCueAdvertiser")
	}
	capsSherpa := advSherpa.SpeechCueCapabilities()
	if capsSherpa.AudioTags || capsSherpa.MarkdownEmphasis {
		t.Errorf("expected Sherpa to support neither AudioTags nor Markdown, got %+v", capsSherpa)
	}

	httpCli := &httpTTSClient{}
	advHTTP, ok := interface{}(httpCli).(SpeechCueAdvertiser)
	if !ok {
		t.Fatal("expected httpTTSClient to implement SpeechCueAdvertiser")
	}
	capsHTTP := advHTTP.SpeechCueCapabilities()
	if !capsHTTP.MarkdownEmphasis {
		t.Errorf("expected httpTTSClient to support MarkdownEmphasis, got false")
	}
}
```

- [x] **Step 2: Run test to verify failure**

Run: `go test -v -run TestSpeechCueAdvertiserImplementations ./pkg/media/`  
Expected: FAIL (compilation error: `SpeechCueAdvertiser` undefined)

- [x] **Step 3: Define `SpeechCueCapabilities` and `SpeechCueAdvertiser` in `pkg/media/tts.go`**

In `pkg/media/tts.go`:
```go
// SpeechCueCapabilities describes the steering hints a TTS engine can interpret.
type SpeechCueCapabilities struct {
	AudioTags        bool     `json:"audio_tags"`
	MarkdownEmphasis bool     `json:"markdown_emphasis"`
	SupportedTags    []string `json:"supported_tags,omitempty"`
	PromptGuidance   string   `json:"prompt_guidance,omitempty"`
}

// SpeechCueAdvertiser is an optional interface implemented by TTS clients that
// declare vocal steering and performance cue capabilities.
type SpeechCueAdvertiser interface {
	SpeechCueCapabilities() SpeechCueCapabilities
}
```

- [x] **Step 4: Implement `SpeechCueAdvertiser` on `ElevenLabsTTSClient`, `SherpaTTSClient`, and `httpTTSClient`**

In `pkg/media/elevenlabs_tts.go`:
```go
func (c *ElevenLabsTTSClient) SpeechCueCapabilities() SpeechCueCapabilities {
	return SpeechCueCapabilities{
		AudioTags:        true,
		MarkdownEmphasis: false,
		SupportedTags: []string{
			"whispers", "sighs", "laughs", "gasp", "clears throat",
			"chuckles", "softly", "loudly", "excited", "angry",
			"nervous", "sad", "playful", "tired",
		},
		PromptGuidance: "Use bracketed tags immediately before dialogue or delivery beats to steer voice acting.",
	}
}
```

In `pkg/media/sherpa_tts.go`:
```go
func (s *SherpaTTSClient) SpeechCueCapabilities() SpeechCueCapabilities {
	return SpeechCueCapabilities{
		AudioTags:        false,
		MarkdownEmphasis: false,
	}
}
```

In `pkg/media/providers.go`:
```go
func (h *httpTTSClient) SpeechCueCapabilities() SpeechCueCapabilities {
	return SpeechCueCapabilities{
		AudioTags:        false,
		MarkdownEmphasis: true,
	}
}
```

- [x] **Step 5: Run tests to verify pass**

Run: `go test -v -run TestSpeechCueAdvertiserImplementations ./pkg/media/`  
Expected: PASS

- [x] **Step 6: Commit**

```bash
git add pkg/media/tts.go pkg/media/elevenlabs_tts.go pkg/media/sherpa_tts.go pkg/media/providers.go pkg/media/speech_cues_test.go
git commit -m "feat(media): add SpeechCueCapabilities and SpeechCueAdvertiser interface"
```

---

### Task 2: Audio Tag Sanitization & `SpeakableTextFor` Pipeline

**Files:**
- Modify: `pkg/media/speakable.go`
- Create: `pkg/media/speakable_cues_test.go`

- [x] **Step 1: Write unit tests in `pkg/media/speakable_cues_test.go`**

```go
package media

import (
	"strings"
	"testing"
)

type mockCueClient struct {
	audioTags bool
	markdown  bool
}

func (m *mockCueClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	return []byte("audio"), nil
}

func (m *mockCueClient) SpeechCueCapabilities() SpeechCueCapabilities {
	return SpeechCueCapabilities{
		AudioTags:        m.audioTags,
		MarkdownEmphasis: m.markdown,
	}
}

func TestStripAudioTags(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{input: `[whispers] "Be quiet!"`, expected: `"Be quiet!"`},
		{input: `[sighs] We made it.`, expected: `We made it.`},
		{input: `Garrick [clears throat] answered.`, expected: `Garrick answered.`},
		{input: `[laughs] [chuckles] "That is good."`, expected: `"That is good."`},
		{input: `[[alden-tavern|The Tavern]] was warm.`, expected: `[[alden-tavern|The Tavern]] was warm.`}, // Wikilinks preserved
		{input: `[whispers]`, expected: ``},
	}

	for _, c := range cases {
		got := StripAudioTags(c.input)
		if got != c.expected {
			t.Errorf("StripAudioTags(%q) = %q, want %q", c.input, got, c.expected)
		}
	}
}

func TestSpeakableTextForSpeechCues(t *testing.T) {
	tagClient := &mockCueClient{audioTags: true, markdown: false}
	plainClient := &mockCueClient{audioTags: false, markdown: false}

	input := `[whispers] "Careful, *adventurer*!"`

	// Client supporting audio tags keeps bracketed cue, strips markdown asterisks
	gotTagged := SpeakableTextFor(TextPolicyAuto, tagClient, input)
	if !strings.Contains(gotTagged, "[whispers]") {
		t.Errorf("expected tagged client to keep [whispers], got %q", gotTagged)
	}
	if strings.Contains(gotTagged, "*") {
		t.Errorf("expected markdown asterisks to be stripped, got %q", gotTagged)
	}

	// Client not supporting audio tags strips both
	gotPlain := SpeakableTextFor(TextPolicyAuto, plainClient, input)
	if strings.Contains(gotPlain, "[whispers]") {
		t.Errorf("expected plain client to strip [whispers], got %q", gotPlain)
	}
	if !strings.Contains(gotPlain, `"Careful, adventurer!"`) {
		t.Errorf("expected clean prose, got %q", gotPlain)
	}
}
```

- [x] **Step 2: Run test to verify failure**

Run: `go test -v -run "TestStripAudioTags|TestSpeakableTextForSpeechCues" ./pkg/media/`  
Expected: FAIL (`StripAudioTags` undefined)

- [x] **Step 3: Implement `audioTagRe`, `StripAudioTags`, and `ClientSupportsAudioTags` in `pkg/media/speakable.go`**

In `pkg/media/speakable.go`:
```go
var audioTagRe = regexp.MustCompile(`(?i)\[[a-z][a-z\s_-]{1,28}\]`)

// StripAudioTags removes bracketed performance tags and normalizes whitespace.
func StripAudioTags(text string) string {
	if text == "" {
		return ""
	}
	replaced := audioTagRe.ReplaceAllString(text, " ")
	return strings.TrimSpace(strings.Join(strings.Fields(replaced), " "))
}

// ClientSupportsAudioTags reports whether a client declares AudioTags support.
func ClientSupportsAudioTags(client TTSClient) bool {
	if client == nil {
		return false
	}
	if adv, ok := client.(SpeechCueAdvertiser); ok {
		return adv.SpeechCueCapabilities().AudioTags
	}
	return false
}
```

Update `SpeakableTextFor` in `pkg/media/speakable.go`:
```go
func SpeakableTextFor(policy TextPolicy, client TTSClient, text string) string {
	var processed string
	switch policy {
	case TextPolicyKeep:
		processed = text
	case TextPolicyStrip:
		processed = SpeakableText(text)
	default:
		if aware, ok := client.(MarkdownAware); ok && aware.SupportsMarkdown() {
			processed = text
		} else {
			processed = SpeakableText(text)
		}
	}

	if !ClientSupportsAudioTags(client) {
		processed = StripAudioTags(processed)
	}

	return strings.TrimSpace(processed)
}
```

- [x] **Step 4: Run tests to verify pass**

Run: `go test -v -run "TestStripAudioTags|TestSpeakableTextForSpeechCues" ./pkg/media/`  
Expected: PASS

- [x] **Step 5: Run all tests in `pkg/media`**

Run: `go test -v -count=1 ./pkg/media/`  
Expected: PASS

- [x] **Step 6: Commit**

```bash
git add pkg/media/speakable.go pkg/media/speakable_cues_test.go
git commit -m "feat(media): add audio tag stripping and sanitization in SpeakableTextFor"
```

---

### Task 3: Dynamic Prompt Orchestration in Harness & Engine

**Files:**
- Modify: `pkg/harness/context.go`
- Create: `pkg/harness/speech_cues_test.go`
- Modify: `pkg/engine/orchestrator.go`

- [x] **Step 1: Write unit tests in `pkg/harness/speech_cues_test.go`**

```go
package harness

import (
	"strings"
	"testing"
)

func TestFormatSpeechFormattingInstructions(t *testing.T) {
	// Mode 1: Audio tags enabled (ElevenLabs)
	withAudio := FormatSpeechFormattingInstructions(SpeechCueContext{
		AudioTags:        true,
		MarkdownEmphasis: false,
		SampleTags:       []string{"[whispers]", "[sighs]", "[laughs]"},
	})
	if !strings.Contains(withAudio, "VOICE ACTING & SPEECH STEERING") {
		t.Errorf("expected instructions to include speech steering section")
	}
	if !strings.Contains(withAudio, "[whispers]") {
		t.Errorf("expected instructions to include sample tags")
	}

	// Mode 2: Audio tags disabled (Sherpa)
	withoutAudio := FormatSpeechFormattingInstructions(SpeechCueContext{
		AudioTags:        false,
		MarkdownEmphasis: false,
	})
	if strings.Contains(withoutAudio, "VOICE ACTING & SPEECH STEERING") {
		t.Errorf("expected no speech steering section when disabled")
	}
	if !strings.Contains(withoutAudio, "Do not write stage directions") && !strings.Contains(withoutAudio, "Do not write voice IDs, voice tags") {
		t.Errorf("expected instructions to forbid tags when disabled")
	}
}
```

- [x] **Step 2: Run test to verify failure**

Run: `go test -v -run TestFormatSpeechFormattingInstructions ./pkg/harness/`  
Expected: FAIL (`FormatSpeechFormattingInstructions` undefined)

- [x] **Step 3: Define `SpeechCueContext` and `FormatSpeechFormattingInstructions` in `pkg/harness/context.go`**

In `pkg/harness/context.go`:
```go
// SpeechCueContext describes the vocal steering hints allowed in generation.
type SpeechCueContext struct {
	AudioTags        bool
	MarkdownEmphasis bool
	SampleTags       []string
	CustomGuidance   string
}

// FormatSpeechFormattingInstructions builds the speech and prose formatting prompt
// tailored to the active engine's speech steering capabilities.
func FormatSpeechFormattingInstructions(cues SpeechCueContext) string {
	var sb strings.Builder
	sb.WriteString("## SPEECH FORMATTING\n")
	sb.WriteString("Write each spoken line on its own line, formatted as  Name: \"the words spoken\"\n")
	sb.WriteString("Use a character's established name, or [[their note name]] to link them.\n")
	sb.WriteString("Keep narration on its own lines with no leading name. If you cannot name the\n")
	sb.WriteString("speaker, leave the words in the narration instead of inventing a name.\n\n")

	sb.WriteString("## PROSE FORMATTING\n")
	sb.WriteString("Separate narration beats with blank lines, one beat per paragraph.\n")
	sb.WriteString("Use plain prose. Do not emit headings, tables, or code fences in narration.\n")
	if cues.MarkdownEmphasis {
		sb.WriteString("You may use *single asterisks* for vocal emphasis and --- for a scene break.\n\n")
	} else {
		sb.WriteString("You may use *single asterisks* for emphasis and --- for a scene break.\n\n")
	}

	if cues.AudioTags {
		sb.WriteString("## VOICE ACTING & SPEECH STEERING\n")
		sb.WriteString("You may steer the vocal delivery of spoken lines and narration beats using bracketed\n")
		sb.WriteString("performance tags immediately before dialogue or delivery. Common supported cues:\n")
		if len(cues.SampleTags) > 0 {
			sb.WriteString("- Supported cues: " + strings.Join(cues.SampleTags, ", ") + "\n")
		} else {
			sb.WriteString("- Delivery/Volume: `[whispers]`, `[softly]`, `[shouts]`, `[loudly]`\n")
			sb.WriteString("- Reactions: `[sighs]`, `[laughs]`, `[chuckles]`, `[gasp]`, `[clears throat]`\n")
			sb.WriteString("- Moods: `[excited]`, `[angry]`, `[sad]`, `[nervous]`, `[playful]`, `[tired]`\n")
		}
		sb.WriteString("Example: Garrick: \"[whispers] Keep your head down.\"\n")
		sb.WriteString("Example: [sighs] It has been a long winter in the northern reaches.\n")
		sb.WriteString("Use cues purposefully to enhance drama; do not clutter every sentence.\n\n")
	}

	sb.WriteString("## CONTINUITY\n")
	sb.WriteString("Never rename a character who has already appeared. Once someone is introduced,\n")
	sb.WriteString("reuse exactly the same name, and link them with [[that name]] every time.\n")
	sb.WriteString("Continue the conversation the player is having; do not restart the scene.\n")
	if cues.AudioTags {
		sb.WriteString("Do not write voice IDs or profile names into the narration.")
	} else {
		sb.WriteString("Do not write stage directions, voice tags, brackets, or profile names into the narration or dialogue (e.g. do not write [whispers]), as the voice synthesizer will mispronounce them.")
	}
	return sb.String()
}
```

In `ContextRequest` in `pkg/harness/context.go`, add:
```go
SpeechCues SpeechCueContext
```
And replace usage of `speechFormattingInstruction` in `AssembleContextWithProfiles` with `FormatSpeechFormattingInstructions(req.SpeechCues)`.

- [x] **Step 4: Update `pkg/engine/orchestrator.go` to hold and pass `SpeechCueContext`**

In `pkg/engine/orchestrator.go`:
Add field to `TurnOrchestrator`:
```go
speechCues harness.SpeechCueContext
```
Add method:
```go
func (o *TurnOrchestrator) SetSpeechCues(cues harness.SpeechCueContext) {
	o.speechCues = cues
}
```
In `ProcessAction` where `o.assembler.Assemble` is called:
```go
	assembly, err := o.assembler.Assemble(harness.ContextRequest{
		LocationID:  locationID,
		PlayerID:    o.playerID,
		Action:      generationPrompt,
		RulesPrompt: o.rulesPrompt,
		LorePrompt:  o.lorePrompt,
		Profiles:    o.timeline.VoiceProfiles(),
		Recent:      recent,
		TurnNumber:  turnNum,
		Summary:     summary,
		Threads:     threads,
		SpeechCues:  o.speechCues,
	})
```

- [x] **Step 5: Run tests**

Run: `go test -v -run TestFormatSpeechFormattingInstructions ./pkg/harness/`  
Run: `go test -v -count=1 ./pkg/harness/`  
Run: `go test -v -count=1 ./pkg/engine/`  
Expected: ALL PASS

- [x] **Step 6: Commit**

```bash
git add pkg/harness/context.go pkg/harness/speech_cues_test.go pkg/engine/orchestrator.go
git commit -m "feat(harness,engine): add dynamic speech formatting instructions for voice steering"
```

---

### Task 4: Configuration Schema & GUI Service Inspection Wiring

**Files:**
- Modify: `pkg/config/types.go`
- Modify: `pkg/gui/types.go`
- Modify: `pkg/gui/service.go`

- [x] **Step 1: Add `SpeechCuesConfig` in `pkg/config/types.go`**

```go
type SpeechCuesConfig struct {
	Enabled          bool   `yaml:"enabled" json:"enabled"`
	AudioTags        *bool  `yaml:"audio_tags,omitempty" json:"audio_tags,omitempty"`
	MarkdownEmphasis *bool  `yaml:"markdown_emphasis,omitempty" json:"markdown_emphasis,omitempty"`
	DisplayMode      string `yaml:"display_mode,omitempty" json:"display_mode,omitempty"`
}

type TTSConfig struct {
	// ...
	SpeechCues SpeechCuesConfig `yaml:"speech_cues,omitempty" json:"speech_cues,omitempty"`
}
```

- [x] **Step 2: Add `SpeechCueCapabilities` to `TTSInspectResponseDTO` in `pkg/gui/types.go`**

```go
type TTSInspectResponseDTO struct {
	ProviderKey    string                       `json:"provider_key"`
	Catalog        VoiceCatalogDTO              `json:"catalog"`
	Options        []VoiceOptionDTO             `json:"options"`
	Error          string                       `json:"error,omitempty"`
	SpeechCues     media.SpeechCueCapabilities `json:"speech_cues"`
}
```

- [x] **Step 3: Update `InspectTTS` and orchestrator setup in `pkg/gui/service.go`**

In `InspectTTS`:
```go
	var cueCaps media.SpeechCueCapabilities
	if adv, ok := client.(media.SpeechCueAdvertiser); ok {
		cueCaps = adv.SpeechCueCapabilities()
	} else if aware, ok := client.(media.MarkdownAware); ok && aware.SupportsMarkdown() {
		cueCaps.MarkdownEmphasis = true
	}
	dto.SpeechCues = cueCaps
```

In `s.beginTurnLocked` where `orch` is constructed:
Resolve effective cues from config and client:
```go
	effCues := harness.SpeechCueContext{
		AudioTags:        true,
		MarkdownEmphasis: false,
	}
	if client, ok := s.ttsClient.(media.SpeechCueAdvertiser); ok {
		caps := client.SpeechCueCapabilities()
		effCues.AudioTags = caps.AudioTags
		effCues.MarkdownEmphasis = caps.MarkdownEmphasis
		effCues.SampleTags = caps.SupportedTags
		effCues.CustomGuidance = caps.PromptGuidance
	}
	if !cfg.Media.TTS.SpeechCues.Enabled {
		effCues.AudioTags = false
		effCues.MarkdownEmphasis = false
	} else {
		if cfg.Media.TTS.SpeechCues.AudioTags != nil {
			effCues.AudioTags = *cfg.Media.TTS.SpeechCues.AudioTags
		}
		if cfg.Media.TTS.SpeechCues.MarkdownEmphasis != nil {
			effCues.MarkdownEmphasis = *cfg.Media.TTS.SpeechCues.MarkdownEmphasis
		}
	}
	orch.SetSpeechCues(effCues)
```

- [x] **Step 4: Run Go tests**

Run: `go test -v -count=1 ./pkg/gui/`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/config/types.go pkg/gui/types.go pkg/gui/service.go
git commit -m "feat(config,gui): wire speech cues configuration and inspect response"
```

---

### Task 5: Frontend Transcript Stage Directions & Display Mode

**Files:**
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/components/MarkdownProse.tsx`
- Modify: `frontend/src/components/TurnSegments.tsx`

- [x] **Step 1: Update `frontend/src/types.ts`**

Add `SpeechCuesConfig` and `SpeechCueCapabilities`:
```typescript
export interface SpeechCueCapabilities {
  audio_tags: boolean;
  markdown_emphasis: boolean;
  supported_tags?: string[];
  prompt_guidance?: string;
}

export interface SpeechCuesConfig {
  enabled: boolean;
  audio_tags?: boolean;
  markdown_emphasis?: boolean;
  display_mode?: 'stage_directions' | 'hidden' | 'raw';
}

export interface TTSConfig {
  // ...
  speech_cues?: SpeechCuesConfig;
}

export interface TTSInspectResponse {
  provider_key: string;
  catalog: VoiceCatalog;
  options: VoiceOption[];
  error?: string;
  speech_cues?: SpeechCueCapabilities;
}
```

- [x] **Step 2: Update `frontend/src/components/MarkdownProse.tsx` to support `displayMode`**

Add `displayMode?: 'stage_directions' | 'hidden' | 'raw'` to `MarkdownProseProps`.

Update `inlinePattern` in `MarkdownProse.tsx`:
```typescript
const inlinePattern = /(\[\[[^\]]+\]\])|(\[[a-zA-Z][a-zA-Z\s_-]{1,28}\])|(`[^`]+`)|(\*\*[^*]+\*\*)|(\*[^*]+\*)|(_[^_]+_)/g;
```

In `renderInline`:
When matching single bracket tags `token.startsWith('[') && !token.startsWith('[[')`:
- If `displayMode === 'hidden'`: skip (do not push to nodes).
- If `displayMode === 'raw'`: push plain text `{token}`.
- If `displayMode === 'stage_directions'` (default):
  Push styled stage-direction chip:
  ```tsx
  nodes.push(
    <span
      key={key++}
      className="inline-flex items-center text-[0.76em] font-sans font-semibold uppercase tracking-wider text-amber-400/90 bg-amber-950/40 border border-amber-700/40 px-1.5 py-0.2 rounded-md mx-1 select-none not-italic align-baseline"
      title="Performance direction"
    >
      {token.slice(1, -1)}
    </span>
  );
  ```

In `MarkdownProse` component:
If `displayMode === 'hidden'`:
Strip tags before paragraph processing:
```typescript
const stripped = displayMode === 'hidden' ? normalized.replace(/\[[a-zA-Z][a-zA-Z\s_-]{1,28}\]/g, '') : normalized;
```

- [x] **Step 3: Update `frontend/src/components/TurnSegments.tsx`**

Pass `displayMode` prop to `TurnSegmentsProps` and pass down to `<MarkdownProse displayMode={displayMode} ... />`.

- [x] **Step 4: Run frontend typecheck**

Run: `mise run test:frontend`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add frontend/src/types.ts frontend/src/components/MarkdownProse.tsx frontend/src/components/TurnSegments.tsx
git commit -m "feat(frontend): render speech cues as styled stage directions or hidden"
```

---

### Task 6: Settings Studio Speech Cues Panel

**Files:**
- Modify: `frontend/src/components/SettingsStudio.tsx`

- [x] **Step 1: Add Speech Steering Cues panel to `SettingsStudio.tsx`**

Under the TTS section in `SettingsStudio.tsx` (around line 1300):
```tsx
            {config.media.tts.type !== 'disabled' && (
              <div className="space-y-3 p-4 rounded-xl bg-stone-900/40 border border-stone-800">
                <div className="flex items-center justify-between">
                  <div className="space-y-0.5">
                    <div className="text-xs font-cinzel font-bold text-stone-200">
                      Speech Steering & Acting Cues
                    </div>
                    <div className="text-[11px] text-stone-400">
                      Instruct the GM to use emotive directions (e.g. [whispers], [sighs]) when supported.
                    </div>
                  </div>
                  <input
                    type="checkbox"
                    checked={config.media.tts.speech_cues?.enabled ?? true}
                    onChange={(e) =>
                      setConfig({
                        ...config,
                        media: {
                          ...config.media,
                          tts: {
                            ...config.media.tts,
                            speech_cues: {
                              ...config.media.tts.speech_cues,
                              enabled: e.target.checked,
                            },
                          },
                        },
                      })
                    }
                    className="w-4 h-4 rounded border-stone-700 bg-stone-900 text-amber-500 focus:ring-amber-500/40 cursor-pointer"
                  />
                </div>

                {inspect?.speech_cues && (
                  <div className="text-[11px] p-2.5 rounded-lg bg-stone-950/60 border border-stone-800/80 text-stone-400">
                    <span className="font-semibold text-stone-300">Provider Capabilities: </span>
                    {inspect.speech_cues.audio_tags ? (
                      <span className="text-amber-400">
                        Supports bracketed vocal cues ({inspect.speech_cues.supported_tags?.slice(0, 5).map(t => `[${t}]`).join(', ')}...)
                      </span>
                    ) : inspect.speech_cues.markdown_emphasis ? (
                      <span className="text-stone-300">Supports Markdown emphasis (*emphasis*)</span>
                    ) : (
                      <span className="text-stone-500">Plain text only; vocal tags are stripped before synthesis.</span>
                    )}
                  </div>
                )}

                {(config.media.tts.speech_cues?.enabled ?? true) && (
                  <div className="space-y-1.5 pt-1">
                    <label className="text-xs font-cinzel uppercase text-stone-300">
                      Transcript Display Mode
                    </label>
                    <select
                      value={config.media.tts.speech_cues?.display_mode || 'stage_directions'}
                      onChange={(e) =>
                        setConfig({
                          ...config,
                          media: {
                            ...config.media,
                            tts: {
                              ...config.media.tts,
                              speech_cues: {
                                ...config.media.tts.speech_cues,
                                enabled: config.media.tts.speech_cues?.enabled ?? true,
                                display_mode: e.target.value as any,
                              },
                            },
                          },
                        })
                      }
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs text-stone-100 focus:outline-none focus:border-amber-500/60 cursor-pointer"
                    >
                      <option value="stage_directions">Stage Directions (styled tags in transcript)</option>
                      <option value="hidden">Hidden (acted out in audio, hidden in transcript)</option>
                      <option value="raw">Raw text (unmodified brackets)</option>
                    </select>
                  </div>
                )}
              </div>
            )}
```

- [x] **Step 2: Run frontend typecheck**

Run: `mise run test:frontend`  
Expected: PASS

- [x] **Step 3: Commit**

```bash
git add frontend/src/components/SettingsStudio.tsx
git commit -m "feat(frontend): add speech steering cues panel to SettingsStudio"
```

---

### Task 7: Full Verification and Build

- [x] **Step 1: Run frontend typecheck**

Run: `mise run test:frontend`  
Expected: PASS

- [x] **Step 2: Run frontend production build**

Run: `mise run build:frontend`  
Expected: PASS

- [x] **Step 3: Restore dist placeholder**

Run: `git checkout -- pkg/gui/dist/.gitkeep 2>/dev/null || true`

- [x] **Step 4: Run Go linter**

Run: `mise run lint`  
Expected: PASS

- [x] **Step 5: Run all backend tests**

Run: `mise run test:backend`  
Expected: PASS

- [x] **Step 6: Run full binary build**

Run: `mise run build`  
Expected: PASS (`bin/localrpg` created)

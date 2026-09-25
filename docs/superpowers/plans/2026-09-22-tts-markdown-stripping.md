# Speakable Text for Markdown-Unaware TTS Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop Markdown-unaware TTS engines reading formatting characters aloud by reducing narrator prose to speakable text before synthesis, while letting Markdown-aware providers opt out.

**Architecture:** Add a pure `media.SpeakableText` reducer that mirrors the frontend's constrained Markdown grammar, give `TTSClient` an optional `MarkdownAware` capability, add an `auto|strip|keep` policy to `TTSPipeline`, reduce before the cache key is computed, and skip segments that reduce to nothing.

**Tech Stack:** Go 1.27.1, React 19, TypeScript.

**Spec:** `docs/superpowers/specs/2026-09-22-tts-markdown-stripping-design.md`

---

## File Structure Map

| File Path | Responsibility |
| :--- | :--- |
| `pkg/media/speakable.go` | Reducer, capability interface, policy helper, sentinel |
| `pkg/media/speakable_test.go` | Table tests for every reduction rule |
| `pkg/media/tts.go` | `TextPolicy`, `SetTextPolicy`, reduction in the pipeline, trace fields |
| `pkg/media/tts_test.go` | Pipeline behaviour and a Markdown-aware fake |
| `pkg/config/types.go` | `TTSConfig.Markdown` |
| `pkg/gui/service.go` | Policy wiring; preview reduction |
| `pkg/export/script.go` | Policy wiring |
| `frontend/src/components/SettingsStudio.tsx` | Markdown selector |

---

### Task 1: Implement the speakable-text reducer

**Files:**
- Create: `pkg/media/speakable.go`
- Create: `pkg/media/speakable_test.go`

- [x] **Step 1: Write the failing table test**

```go
func TestSpeakableText(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"emphasis", "She *hesitates*, then **commits**.", "She hesitates, then commits."},
		{"strong emphasis", "***Now*** or never.", "Now or never."},
		{"underscore words survive", "The snake_case_field stays intact.", "The snake_case_field stays intact."},
		{"wikilink labelled", "Ask [[lady-evelyn|Lady Evelyn]] about it.", "Ask Lady Evelyn about it."},
		{"wikilink bare", "See [[the-ashen-bastion]].", "See the-ashen-bastion."},
		{"inline code", "Type `1d20+5` to roll.", "Type 1d20+5 to roll."},
		{"heading", "## The Gate\nIt looms.", "The Gate It looms."},
		{"list", "- First\n- Second", "First Second"},
		{"ordered list", "1. First\n2. Second", "First Second"},
		{"blockquote", "> Beware the mist.", "Beware the mist."},
		{"scene break", "Before\n\n---\n\nAfter", "Before After"},
		{"entities", "Salt &amp; iron, &quot;cold&quot;.", "Salt & iron, \"cold\"."},
		{"keeps dashes and ellipses", "Wait—no… perhaps.", "Wait—no… perhaps."},
		{"blank reduces empty", "   \n\n  ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SpeakableText(tt.in); got != tt.want {
				t.Errorf("SpeakableText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
```

- [x] **Step 2: Run and confirm failure**

Run: `go test -run TestSpeakableText ./pkg/media/`
Expected: FAIL, undefined `SpeakableText`.

- [x] **Step 3: Implement `pkg/media/speakable.go`**

Handle, in order: line-ending normalisation, fenced code fences, wikilinks, inline code, emphasis (with the underscore guard), blockquotes, headings, list markers, scene breaks, HTML entities, then whitespace collapse. Preserve `. , ! ? ; : — … - ( ) " '`.

```go
var (
	wikilinkRe   = regexp.MustCompile(`\[\[([^\]\|]+)(?:\|([^\]]+))?\]\]`)
	inlineCodeRe = regexp.MustCompile("`([^`]+)`")
	strongRe     = regexp.MustCompile(`\*\*\*([^*]+)\*\*\*|\*\*([^*]+)\*\*`)
	emRe         = regexp.MustCompile(`\*([^*]+)\*|(?:^|\s)_([^_\s][^_]*[^_\s]|[^_])_(?:\s|$|[.,!?;:])`)
	headingRe    = regexp.MustCompile(`(?m)^\s{0,3}#{1,6}\s+`)
	listRe       = regexp.MustCompile(`(?m)^\s*(?:[-*+]|\d+[.)])\s+`)
	quoteRe      = regexp.MustCompile(`(?m)^\s*>\s?`)
	sceneBreakRe = regexp.MustCompile(`(?m)^\s*(?:-{3,}|\*{3,}|_{3,})\s*$`)
)

func SpeakableText(text string) string {
	// ... replace, strip, decode, then strings.Fields + join
}
```

Decode entities with `html.UnescapeString` from the standard library and keep any unrecognised entity as written. Collapse whitespace with `strings.Fields` and a single-space join, then `strings.TrimSpace`. A newline becomes a space; punctuation already provides the pause.

- [x] **Step 4: Run the test**

Run: `go test -run TestSpeakableText ./pkg/media/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/media/speakable.go pkg/media/speakable_test.go
git commit -m "feat(media): reduce narrator markdown to speakable text"
```

---

### Task 2: Add the capability interface and policy

**Files:**
- Modify: `pkg/media/speakable.go`
- Modify: `pkg/media/tts.go`
- Modify: `pkg/media/tts_test.go`

- [x] **Step 1: Write the failing tests**

```go
type markdownFake struct{ got []string; aware bool }

func (m *markdownFake) Synthesize(_ context.Context, text string, _ *entity.VoiceConfig) ([]byte, error) {
	m.got = append(m.got, text)
	return []byte("RIFF"), nil
}
func (m *markdownFake) SupportsMarkdown() bool { return m.aware }

func TestPipelineStripsMarkdownByDefault(t *testing.T) {
	client := &markdownFake{}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))
	_, err := pipeline.SynthesizeSegment(context.Background(), entity.TurnSegment{Kind: entity.SegmentNarration, Text: "A *soft* word."}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if client.got[0] != "A soft word." {
		t.Errorf("engine received %q, want %q", client.got[0], "A soft word.")
	}
}

func TestPipelineKeepsMarkdownForAwareClient(t *testing.T) {
	client := &markdownFake{aware: true}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))
	_, _ = pipeline.SynthesizeSegment(context.Background(), entity.TurnSegment{Kind: entity.SegmentNarration, Text: "A *soft* word."}, nil, nil)
	if client.got[0] != "A *soft* word." {
		t.Errorf("aware engine received %q, want raw markdown", client.got[0])
	}
}

func TestPipelineSkipsEmptyAfterStrip(t *testing.T) {
	client := &markdownFake{}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))
	_, err := pipeline.SynthesizeSegment(context.Background(), entity.TurnSegment{Kind: entity.SegmentNarration, Text: "---"}, nil, nil)
	if !errors.Is(err, ErrNoSpeakableText) {
		t.Fatalf("expected ErrNoSpeakableText, got %v", err)
	}
	if len(client.got) != 0 {
		t.Errorf("empty segment reached the engine: %q", client.got)
	}
}

func TestPipelineSharesCacheForEquivalentText(t *testing.T) {
	// "**bold**" and "bold" must produce the same cached path.
}
```

- [x] **Step 2: Run and confirm failure**

Run: `go test -run 'TestPipeline' ./pkg/media/`
Expected: FAIL.

- [x] **Step 3: Implement**

In `pkg/media/speakable.go`:

```go
// MarkdownAware clients consume narrator Markdown themselves. A client that does
// not implement this is sent plain speakable text.
type MarkdownAware interface {
	SupportsMarkdown() bool
}

type TextPolicy int

const (
	TextPolicyAuto TextPolicy = iota
	TextPolicyStrip
	TextPolicyKeep
)

// ErrNoSpeakableText reports that a segment reduced to nothing and has no audio.
var ErrNoSpeakableText = errors.New("segment has no speakable text")

// SpeakableTextFor applies a policy to one segment's text.
func SpeakableTextFor(policy TextPolicy, client TTSClient, text string) string
```

In `pkg/media/tts.go`, add a `policy TextPolicy` field, `SetTextPolicy`, and in `SynthesizeSegment`:

```go
spoken := SpeakableTextFor(p.policy, p.client, segment.Text)
if strings.TrimSpace(spoken) == "" {
	return "", ErrNoSpeakableText
}
return p.SynthesizeUtterance(ctx, speakerID, voice, spoken)
```

Move the existing empty-input guard so the sentinel is the single empty case.

- [x] **Step 4: Run the media tests**

Run: `go test -count=1 ./pkg/media/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/media/speakable.go pkg/media/tts.go pkg/media/tts_test.go
git commit -m "feat(media): strip markdown for engines that do not read it"
```

---

### Task 3: Skip empty segments instead of failing

**Files:**
- Modify: `pkg/media/tts.go`
- Modify: `pkg/gui/service.go`
- Modify: `pkg/export/script.go`
- Modify: `pkg/media/tts_test.go`

- [x] **Step 1: Write the failing test**

```go
func TestSynthesizeSegmentsSkipsUnspeakableBeats(t *testing.T) {
	// segments: "---" (reduces to nothing) then "Hello."
	// assert one clip returned and no error
}
```

- [x] **Step 2: Implement**

In `SynthesizeSegments`, skip `ErrNoSpeakableText` rather than returning it:

```go
clip, err := p.SynthesizeSegment(ctx, segment, narratorVoice, voiceFor)
if errors.Is(err, ErrNoSpeakableText) {
	continue
}
if err != nil {
	return nil, err
}
```

`PlayTurnAudio` already skips synthesis errors, so `ErrNoSpeakableText` needs no change there; confirm it. In the export compiler's `resolveAudio`, count the sentinel as silent so the progress message stays honest.

- [x] **Step 3: Run tests**

Run: `go test -count=1 ./pkg/media/ ./pkg/gui/ ./pkg/export/ ./pkg/scene/`
Expected: PASS.

- [x] **Step 4: Commit**

```bash
git add pkg/media/tts.go pkg/gui/service.go pkg/export/script.go pkg/media/tts_test.go
git commit -m "fix(media): skip beats that reduce to no speakable text"
```

---

### Task 4: Add the configuration field

**Files:**
- Modify: `pkg/config/types.go`
- Modify: `pkg/config/types_test.go`

- [x] **Step 1: Write the failing test**

```go
func TestTTSMarkdownDefaultsToAuto(t *testing.T) {
	cfg := config.DefaultConfig()
	if cfg.Media.TTS.Markdown != "" && cfg.Media.TTS.Markdown != "auto" {
		t.Fatalf("expected auto/empty, got %q", cfg.Media.TTS.Markdown)
	}
}
```

- [x] **Step 2: Implement**

Add to `TTSConfig`:

```go
// Markdown selects how narration Markdown is treated before synthesis:
// "auto" (default) reduces it unless the provider is Markdown-aware, "strip"
// always reduces it, and "keep" sends it unchanged.
Markdown string `yaml:"markdown,omitempty" json:"markdown,omitempty"`
```

- [x] **Step 3: Map the string to a policy**

Add `media.TextPolicyFromConfig(cfg config.TTSConfig) TextPolicy` (accepting `""`/`"auto"` as auto, `"strip"`, `"keep"`, and logging/ignoring anything else as auto).

- [x] **Step 4: Run tests**

Run: `go test -count=1 ./pkg/config/ ./pkg/media/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/config/types.go pkg/config/types_test.go pkg/media/speakable.go
git commit -m "feat(config): let tts markdown handling be configured"
```

---

### Task 5: Wire the policy and fix the preview

**Files:**
- Modify: `pkg/gui/service.go`
- Modify: `pkg/export/script.go`
- Modify: `pkg/gui/service_test.go`

- [x] **Step 1: Write the failing test**

Add a GUI test asserting that a TTS preview whose prompt contains `**bold**` synthesises "bold" (inspect via the echo/mock client or the returned bytes for a fake).

- [x] **Step 2: Set the policy on every pipeline**

Where `media.NewTTSPipeline(...)` is constructed, follow it with:

```go
pipeline.SetTextPolicy(media.TextPolicyFromConfig(cfg.Media.TTS))
```

Do this in `GetSegmentAudio` (`pkg/gui/service.go`) and in the export compiler's pipeline construction (`pkg/export/script.go`).

- [x] **Step 3: Reduce the preview**

In the TTS test branch (`pkg/gui/service.go` around line 2163), replace the direct call with:

```go
spoken := media.SpeakableTextFor(media.TextPolicyFromConfig(ttsCfg), client, prompt)
if strings.TrimSpace(spoken) == "" {
	return &TestProviderResponseDTO{Success: false, Message: "The test phrase reduced to no speakable text"}, nil
}
audio, err := client.Synthesize(ctx, spoken, voice)
```

- [x] **Step 4: Run tests**

Run: `go test -count=1 ./pkg/gui/ ./pkg/export/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/gui/service.go pkg/export/script.go pkg/gui/service_test.go
git commit -m "fix(gui): apply markdown handling to playback and the tts preview"
```

---

### Task 6: Expose the policy in Settings

**Files:**
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/components/SettingsStudio.tsx`

- [x] **Step 1: Types**

Add `markdown?: 'auto' | 'strip' | 'keep'` to `TTSConfig`.

- [x] **Step 2: Selector**

In the TTS panel add a small select labelled "Narration Markdown" with the three options and help text: auto reduces formatting unless the provider understands it, strip always reduces, keep sends it unchanged.

- [x] **Step 3: Typecheck**

Run: `cd frontend && npx tsc --noEmit`
Expected: clean.

- [x] **Step 4: Commit**

```bash
git add frontend/src/types.ts frontend/src/components/SettingsStudio.tsx
git commit -m "feat(frontend): configure tts markdown handling in settings"
```

---

### Task 7: Verification

- [x] **Step 1: Backend gate**

Run: `mise run test:backend` and `mise run lint`
Expected: all tests pass, `go vet` clean.

- [x] **Step 2: Frontend gate**

Run: `cd frontend && npx tsc --noEmit`
Expected: clean.

- [x] **Step 3: Manual smoke**

1. With the built-in Sherpa-ONNX TTS, play a turn whose prose contains `*emphasis*`, a heading, a list, and a `[[wikilink]]`; confirm none of the marks are spoken.
2. Confirm a scene break produces no audio and no error.
3. Use the Settings preview with `**bold**` and confirm "bold" is spoken.
4. Set the policy to `keep` and confirm the raw text (including the marks) is sent.

# Design Spec: Speakable Text for Markdown-Unaware TTS Providers

**Date:** 2026-09-22
**Status:** Draft
**Target:** `pkg/media`, `pkg/config`, `pkg/gui`, `pkg/export`

---

## 1. Executive Summary

Narrator prose is Markdown. The built-in Sherpa-ONNX renderer does not interpret Markdown, so a line like `*she hesitates*` is spoken as "asterisk she hesitates asterisk", and `[[Lady Evelyn|Evelyn]]` is read with its brackets and pipe. The same problem affects every provider that expects plain text: native OS speech, OpenAI-compatible HTTP endpoints, and piper.

This spec introduces a **speakable-text reduction**: before a clip is synthesised, narrator Markdown is reduced to the words a person would actually read aloud, using the same constrained grammar the frontend already renders. Providers that genuinely consume Markdown (future SSML-aware engines) can opt out through a capability interface and a per-provider policy.

---

## 2. Findings

### 2.1 Segments carry raw Markdown

`SynthesizeSegment` passes `segment.Text` straight to the client (`pkg/media/tts.go:142`); `SynthesizeUtterance` sends that text verbatim and hashes it into the cache key (`pkg/media/tts.go:148-154`). Nothing between the model output and the engine removes formatting.

### 2.2 The frontend already has the grammar

`MarkdownProse` renders a deliberately small grammar (`frontend/src/components/MarkdownProse.tsx`):

- inline: `[[wikilinks]]`, `` `code` ``, `**bold**`, `*italic*`, `_italic_`
- block: headings `#{1,6}`, unordered lists `- `, blockquotes `> `, and scene breaks (`---`, `***`, `___` on their own line)
- raw HTML is never interpreted; it is shown as plain text

Speech reduction should mirror exactly this grammar so what is spoken matches what is read.

### 2.3 There is no capability signal

`TTSClient` is a single method (`pkg/media/tts.go:81`). There is no way for a provider to say "I understand Markdown" or for configuration to say "keep it anyway". `TTSConfig` (`pkg/config/types.go:92`) has no text-handling field.

### 2.4 The settings preview bypasses the pipeline

The TTS test/preview path calls `client.Synthesize(ctx, prompt, voice)` directly (`pkg/gui/service.go:2163`), so a preview containing Markdown is read literally even once the pipeline is fixed.

### 2.5 Empty-after-reduction segments exist

A scene break or an empty heading reduces to nothing. Passing an empty string to an engine is either an error or a spurious clip, so the reduction needs a sentinel the callers can skip.

---

## 3. Design

### 3.1 `media.SpeakableText`

Create `pkg/media/speakable.go`:

```go
// SpeakableText reduces narrator Markdown to the words a person would read aloud.
// The grammar mirrors the frontend's constrained MarkdownProse renderer, so what
// is spoken matches what is shown: wikilinks, emphasis, inline code, headings,
// lists, blockquotes, scene breaks, and common HTML entities.
func SpeakableText(text string) string
```

Reduction rules, applied in order:

1. Normalise line endings and tabs.
2. Fenced code blocks: drop the fences, keep the content (narrator prose rarely uses them; dropping content would lose words).
3. Wikilinks: `[[target|label]]` -> `label`; `[[target]]` -> `target` (reuse `entity.WikilinkTarget` semantics without importing the entity parser's stricter rules).
4. Inline code: `` `x` `` -> `x`.
5. Emphasis: `***x***`, `**x**`, `*x*`, `_x_` -> `x`, with the guard that `_` is only emphasis when it pairs around a run with no internal whitespace, so `snake_case` survives intact.
6. Blockquotes: strip a leading `>` and optional space from each line.
7. Headings: strip the leading `#{1,6}` and whitespace.
8. Lists: strip a leading `- `, `* `, `+ `, or `N.`/`N)` marker; join items into prose.
9. Scene breaks (`---`, `***`, `___` alone on a line) and thematic breaks: remove, leaving a paragraph break.
10. Decode common HTML entities (`&amp;`, `&lt;`, `&gt;`, `&quot;`, `&#39;`, `&nbsp;`, and numeric `&#NN;`); leave unrecognised entities as text.
11. Collapse whitespace: single newlines become a sentence-separating space (the renderer treats them as beats, the voice needs a pause, not a literal); runs of blank lines become one space; trim.
12. Punctuation that prosody depends on is preserved untouched: `. , ! ? ; : — … - ( ) " '` and the ellipsis character. Em dashes and ellipses are in the Sherpa token set and must not be rewritten.

`SpeakableText` is pure, allocation-light, and has no configuration dependence.

### 3.2 Capability interface

```go
// MarkdownAware clients consume narrator Markdown themselves, for example an
// engine that maps emphasis to SSML. A client that does not implement this is
// sent plain speakable text.
type MarkdownAware interface {
	SupportsMarkdown() bool
}
```

Every current provider is Markdown-unaware, so none implements it and all receive reduced text. The interface exists so a future engine can opt out without changing the pipeline.

### 3.3 Per-provider policy

Add to `TTSConfig`:

```go
// Markdown selects how narration Markdown is treated before synthesis:
// "auto" (default) reduces it unless the provider is Markdown-aware, "strip"
// always reduces it, and "keep" sends it unchanged.
Markdown string `yaml:"markdown,omitempty" json:"markdown,omitempty"`
```

Add a policy to `TTSPipeline`:

```go
type TextPolicy int

const (
	TextPolicyAuto TextPolicy = iota // reduce unless the client is MarkdownAware
	TextPolicyStrip
	TextPolicyKeep
)

func (p *TTSPipeline) SetTextPolicy(policy TextPolicy)
```

`NewTTSPipeline` defaults to `TextPolicyAuto`, so existing callers and tests keep working. `Service` and the export compiler set the policy from `cfg.Media.TTS.Markdown`.

Effective text:

```go
func (p *TTSPipeline) speakable(text string) string {
	switch p.policy {
	case TextPolicyKeep:
		return text
	case TextPolicyStrip:
		return SpeakableText(text)
	default:
		if aware, ok := p.client.(MarkdownAware); ok && aware.SupportsMarkdown() {
			return text
		}
		return SpeakableText(text)
	}
}
```

### 3.4 Pipeline integration

In `SynthesizeSegment`, reduce once and pass the reduced text onward:

- the reduced text is what `SynthesizeUtterance` sends **and** what it hashes, so `**bold**` and `bold` share one cached clip and a Markdown-only edit does not re-render;
- the `media.tts.request` trace event gains `chars_raw` and `chars_spoken`, so a trace shows exactly what was read;
- if the reduced text is empty, return a new sentinel `ErrNoSpeakableText`.

Callers:

- `SynthesizeSegments` (export) skips a segment returning `ErrNoSpeakableText` rather than aborting;
- `GetSegmentAudio` returns the sentinel, and `PlayTurnAudio` skips it as it already skips synthesis failures;
- the export compiler counts it as silent, as it does a missing clip.

### 3.5 Preview

The settings preview (`pkg/gui/service.go:2163`) must reduce before calling the client. Use the same policy helper rather than duplicating logic: introduce `media.SpeakableTextFor(cfg config.TTSConfig, client TTSClient, text string) string` that implements the policy decision, and call it from both the pipeline and the preview.

### 3.6 Frontend and exports

No frontend change: `MarkdownProse` already renders formatting. Export audio and video use the same pipeline, so both are fixed by 3.4; their on-screen text is unaffected.

### 3.7 Out of scope

- SSML generation for providers that support it.
- Rewriting abbreviations, numbers, or units for pronunciation.
- Language-specific normalisation beyond what `espeak-ng` already does inside Sherpa.

---

## 4. Data Flow

```text
GM prose (Markdown)
  └─ dialogue.Parse / buildTurnSegments ─► segment.Text (raw Markdown)
       └─ TTSPipeline.SynthesizeSegment
            └─ speakable(text)  [auto: strip unless client is MarkdownAware]
                 ├─ empty ─► ErrNoSpeakableText ─► skipped by playback/export
                 └─ text  ─► cache key(speaker, voice, prosody, speakable)
                             └─ client.Synthesize(ctx, speakable, voice)
```

---

## 5. File Map

| Action | Path | Description |
| :--- | :--- | :--- |
| Create | `pkg/media/speakable.go` | `SpeakableText`, `SpeakableTextFor`, `MarkdownAware`, `ErrNoSpeakableText` |
| Create | `pkg/media/speakable_test.go` | Table tests for every reduction rule |
| Modify | `pkg/media/tts.go` | `TextPolicy`, `SetTextPolicy`, reduction in `SynthesizeSegment`, trace fields |
| Modify | `pkg/media/tts_test.go` | Pipeline reduction, cache sharing, empty-segment skip, Markdown-aware fake |
| Modify | `pkg/config/types.go` | `TTSConfig.Markdown` |
| Modify | `pkg/config/types_test.go` | Default is `auto` |
| Modify | `pkg/gui/service.go` | Set the policy on playback pipelines; reduce the preview |
| Modify | `pkg/export/script.go` | Set the policy on the export pipeline |
| Modify | `frontend/src/components/SettingsStudio.tsx` | Optional Markdown selector (auto/strip/keep) in the TTS panel |

---

## 6. Acceptance Criteria

1. Sherpa-ONNX never speaks asterisks, underscores, backticks, `#`, `>` list markers, wikilink brackets, or pipes.
2. `**bold**` and `bold` produce one cached clip; changing only Markdown formatting does not re-render.
3. A provider implementing `MarkdownAware` and returning true receives the raw text under the default policy.
4. `strip`, `keep`, and `auto` behave as documented, and `auto` is the default.
5. A segment that reduces to nothing is skipped by playback and by export, never synthesised or fatal.
6. The settings preview reduces Markdown for Markdown-unaware providers.
7. `go test -count=1 ./...`, `go vet ./...`, and `npx tsc --noEmit` pass.

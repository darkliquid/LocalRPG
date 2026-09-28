# Voice Input Reliability Implementation Plan

> **Status:** Implemented and verified against the code on 2026-09-28.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop web-speech from silently recording audio a backend cannot transcribe, make the unavailable case obvious in the mic button and Settings, and give the browser-only provider an honest, actionable catalogue description.

**Architecture:** A shared `hasWebSpeechSupport()` probe replaces the inline constructor lookup. `useVoiceInput` derives an explicit mode and refuses to start the backend recorder when web-speech is selected but unsupported. Settings annotates and disables the option from the same probe. The backend returns an actionable `provider_unavailable` failure for web-speech instead of a bare factory error.

**Tech Stack:** React 19 + TypeScript (`strict`, `noUnusedLocals`), Go 1.27 stdlib tests.

**Spec:** `docs/superpowers/specs/2026-09-28-voice-input-reliability-design.md`

## Global Constraints

- Use `interface{}`, never `any`; `go vet` clean.
- Standard library only in Go tests; no testify.
- `frontend/` `tsc --noEmit` must pass.
- Regenerate embedded docs after a descriptor change: `go test ./pkg/gui -update-docs`.
- A frontend test runner does not exist and is not added here.

---

### Task 1: Shared web-speech capability probe

**Files:** Create `frontend/src/lib/webSpeech.ts`; test by `tsc`.

- [x] **Step 1:** Add:

```ts
// hasWebSpeechSupport reports whether this window exposes the browser speech
// recognition API. It is absent in the Wails webview (WebKit etc.), so callers
// must offer a server-side provider there.
export function hasWebSpeechSupport(): boolean {
  const w = window as unknown as {
    SpeechRecognition?: unknown;
    webkitSpeechRecognition?: unknown;
  };
  return Boolean(w.SpeechRecognition || w.webkitSpeechRecognition);
}
```

- [x] **Step 2:** Verify `cd frontend && npx tsc --noEmit`.

---

### Task 2: `useVoiceInput` refuses an unrunnable provider

**Files:** Modify `frontend/src/hooks/useVoiceInput.ts`.

- [x] **Step 1:** Import the probe and derive a mode:

```ts
export type VoiceMode = 'web-speech' | 'backend' | 'unavailable';

const webSpeechSupported = hasWebSpeechSupport();
const mode: VoiceMode =
  sttType === 'web-speech' ? (webSpeechSupported ? 'web-speech' : 'unavailable') : 'backend';
```

- [x] **Step 2:** In `startRecording`, when `mode === 'unavailable'`, set a clear
  error and return **without** starting the recorder:

```ts
if (mode === 'unavailable') {
  setError(
    "Web Speech isn't available in this window. Choose a Whisper STT provider in Settings, or run the app in a Chromium browser."
  );
  return;
}
```

- [x] **Step 3:** Enter the browser path on `mode === 'web-speech'`, and if
  `recognition.start()` throws, set an error and return rather than falling
  through to the backend recorder (a `web-speech` backend cannot serve the blob).

- [x] **Step 4:** Return `mode` and `available: mode !== 'unavailable'`.

- [x] **Step 5:** Verify `npx tsc --noEmit`.

---

### Task 3: `ActionConsole` disables the mic when unrunnable

**Files:** Modify `frontend/src/components/ActionConsole.tsx`.

- [x] **Step 1:** Read `available` and `mode` from the hook; disable the mic
  button when `!available`, and set its title to the explanation.
- [x] **Step 2:** Render the hook's `error` as before.
- [x] **Step 3:** Verify `npx tsc --noEmit`.

---

### Task 4: Settings annotates the web-speech option

**Files:** Modify `frontend/src/components/SettingsStudio.tsx`.

- [x] **Step 1:** Compute `webSpeechAvailable = hasWebSpeechSupport()` at the top
  of the component.
- [x] **Step 2:** Render the `web-speech` option disabled with a suffix when
  unavailable, and show a one-line hint under the provider select explaining the
  fallback to a Whisper provider.
- [x] **Step 3:** Rename the "Builtin / Mock" STT option to "Builtin" (the mock
  type was retired).
- [x] **Step 4:** Verify `npx tsc --noEmit`.

---

### Task 5: Backend returns actionable web-speech failures

**Files:** Modify `pkg/gui/service.go`; test `pkg/gui/stt_failure_test.go`.

- [x] **Step 1:** In `TranscribeAudio`, special-case `web-speech` before building
  a client:

```go
if cfg.Media.STT.Type == "web-speech" {
    return "", &harness.GenerationFailure{
        Code:    harness.FailureProviderUnavailable,
        Message: "the web-speech STT provider runs in the browser; configure an HTTP or CLI Whisper provider",
    }
}
```

- [x] **Step 2:** In `TestProvider`'s `stt` case, return the same actionable
  message when the factory rejects `web-speech`.
- [x] **Step 3:** Update the descriptor description in
  `pkg/provider/sttwebspeech/sttwebspeech.go` to say it is Chromium-only and that
  the Wails webview needs a Whisper provider.
- [x] **Step 4:** Test `TranscribeAudio` with `web-speech` returns
  `FailureProviderUnavailable` and a message naming Whisper.
- [x] **Step 5:** `go test ./pkg/gui/ ./pkg/provider/...` and
  `go test ./pkg/gui -update-docs`.

---

### Task 6: Full verification

- [x] **Step 1:** `go build ./...`, `go test ./...`, `go vet ./...`.
- [x] **Step 2:** `cd frontend && npx tsc --noEmit`.

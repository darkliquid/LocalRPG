# Voice Input Reliability Design

**Date:** 2026-09-28
**Status:** Proposed
**Scope:** Make speech-to-text honest and usable: detect Web Speech capability before offering it, stop the silent fall-through that turns an unsupported provider into a confusing backend error, gate settings on real capability, and cover the full mic-to-action path with tests
**Related:** `frontend/src/hooks/useVoiceInput.ts`, `frontend/src/components/ActionConsole.tsx`, `frontend/src/components/SettingsStudio.tsx`, `frontend/src/hooks/useProviderCatalog.ts`, `pkg/media` (`webspeech_stt.go`, `providers.go`, `exports.go`), `pkg/provider/sttwebspeech`, `pkg/config` (`types.go`, `presets.go`), `pkg/gui` (`service.go`, `server.go`, `catalogue.go`); builds on `docs/superpowers/specs/2026-09-21-real-media-pipelines-and-voice-input-design.md` and `docs/superpowers/specs/2026-09-26-provider-error-surfacing-design.md`

## 1. Overview & Goals

Voice input has never worked end to end, for two independent reasons.

First, the browser Web Speech path is unavailable in the app's own webview. The
frontend only uses `window.SpeechRecognition`/`webkitSpeechRecognition`
(`frontend/src/hooks/useVoiceInput.ts:47-49`), and only enters that branch when
the constructor exists (`:51`). In the Wails v3 webview (WebKitGTK, and
WKWebView/WebView2 without the Google speech backend) the constructor is absent,
so the branch is skipped and the hook **silently falls through to MediaRecorder**
(`:84-137`), records audio, and POSTs it to `/api/stt` with
`media.stt.type = "web-speech"` still configured. The backend then fails, because
`web-speech` has no backend client
(`pkg/media/webspeech_stt.go:11-20`) and `STTKeyFor` deliberately rejects it
(`pkg/media/exports.go:66-76`), yielding `unsupported stt provider type:
web-speech` (`pkg/media/providers.go:92-93`).

Second, the shipped default is `disabled`. `DefaultConfig` sets
`media.stt.type = "disabled"` (`pkg/config/types.go:433-435`), and
`STTPresets` contains no browser preset at all — only `faster-whisper`,
`whisper-cli`, and `openai-whisper` (`pkg/config/presets.go:198-214`). A fresh
install therefore cannot transcribe anything, and the Settings picker offers
`web-speech` as a hard-coded option with no capability check
(`frontend/src/components/SettingsStudio.tsx:2456`).

**Goals:**

- Never offer, or silently attempt, a speech provider the runtime cannot execute.
- When a chosen provider cannot run where it is running, say so plainly and name
  the alternative.
- Make a working local/remote Whisper provider the obvious, documented choice.
- Stop the silent fall-through from web-speech to a backend that cannot serve it.
- Cover the mic-to-action path with a test that would have caught this.

**Non-Goals:**

- Bundling a local STT model or changing the transcription engine.
- Browser Text-to-Speech (none exists; all TTS is server-side).
- Streaming/partial transcription UI (out of scope; `interimResults` stays
  false).
- Changing the provider key grammar (owned by the key-identity spec).

**Success Criteria:**

- Selecting `web-speech` where the Web Speech API is absent shows a clear,
  immediate message naming the fix, and does not record-and-fail.
- `web-speech` is visibly marked unavailable in Settings when the runtime cannot
  use it, and the picker recommends a Whisper provider.
- A disabled/unconfigured STT produces a clear "no speech engine is configured"
  message, not a generic failure.
- A Go test drives the multipart `POST /api/stt` path with a fake engine and
  asserts the transcript round-trips; a test asserts `web-speech` yields a
  `provider_unavailable` failure naming the cause.
- The manual test matrix (Wails Linux/macOS/Windows, browser, no-mic) is recorded
  and passes.

## 2. Investigation Findings

- **Hook decision logic.** `useVoiceInput.ts`: detection at `:47-49`, branch
  condition `sttType === 'web-speech' && SpeechRecognition` at `:51`. When the
  constructor is missing the `&&` fails and execution continues to MediaRecorder
  with no error and no signal (`:84-137`); the blob is POSTed at `:117` via
  `APIClient.transcribeAudio` → `POST /api/stt` (`frontend/src/api/client.ts:476-480`).
  `sttType` comes from `config.media.stt.type` (`App.tsx:813`,
  `ActionConsole.tsx:10,18,27-30`).
- **Backend placeholder.** `ErrWebSpeechIsClientSide`
  (`pkg/media/webspeech_stt.go:11`); `webSpeechSTTClient.Transcribe` returns it
  (`:13-17`). Descriptor `stt:web-speech`: `Source: "builtin"`, features
  `[offline]`, preset `{id: web-speech, config: {type: web-speech}}`
  (`pkg/provider/sttwebspeech/sttwebspeech.go:12-30`).
- **Factory rejects it.** `STTKeyFor` returns `ok=false` for `web-speech`/
  `builtin` (`pkg/media/exports.go:66-76`); `NewSTTClient`'s default branch
  returns `unsupported stt provider type: web-speech`
  (`pkg/media/providers.go:80-95`). `TranscribeAudio` wraps *client construction*
  errors as `FailureProviderUnavailable` (`pkg/gui/service.go:3059-3066`), but a
  `Transcribe` error is returned raw (`:3076-3078`) — for the placeholder path it
  is never reached because construction already fails.
- **Disabled is the default.** `STTConfig` fields
  (`pkg/config/types.go:174-182`); default `disabled` (`:433-435`).
  `TranscribeAudio` on disabled returns `provider_unavailable` with "STT engine
  is disabled or unconfigured" (`service.go:3055-3057`).
- **Routes and probes.** `POST /api/stt` → `handleSTTRoute`
  (`pkg/gui/server.go:103,1057-1110`), multipart `audio`/`file`, 25 MB cap, errors
  via `writeGenerationFailure` (`:1103`). `POST /api/settings/test-provider`
  (`server.go:97`, `handleTestProviderRoute:964`) with `TestProvider` STT case
  (`service.go:2988-3017`) synthesises a tone WAV and calls `Transcribe`; the
  failure carries a `harness.GenerationFailure` (`:3004-3010`).
- **Capabilities are published but unused for STT.** `GET /api/providers`
  (`server.go:98`, `Service.ListProviders` `pkg/gui/catalogue.go:15-17`) returns
  descriptors with `Features` (`pkg/provider/descriptor.go:14-74`). The frontend
  already fetches and types them (`useProviderCatalog.ts:16-60`,
  `types.ts:684-737`), but `SettingsStudio.tsx` renders the STT provider
  `<select>` statically, including a hard-coded `web-speech` option
  (`:2456`), and never consults `features`.
- **No runtime capability endpoint.** The route table is `server.go:91-108`;
  nothing reports `ffmpeg`-style runtime facts for speech. Wails webview identity
  is known only to the backend (`pkg/gui/middleware.go:11-29` trusted origins);
  `README.md:29` names WebKit.
- **Tests today.** Go: `pkg/media/stt_test.go:16`,
  `pkg/media/fallback_test.go:55`, `pkg/media/key_test.go:56` (asserts
  `web-speech`/`builtin` have no key), `pkg/media/providers_test.go:26,58`,
  `pkg/gui/stt_failure_test.go:10`, `pkg/gui/server_test.go:712,749`,
  `pkg/provider/sttwhisperhttp/client_test.go:14`. Frontend: **none** — no test
  runner exists (`frontend/package.json` has no test script).

## 3. Design

### 3.1 Detect capability, then be honest

In `useVoiceInput`, split the decision into explicit states:

```ts
type VoiceMode = 'web-speech' | 'backend' | 'unavailable';
```

- Compute `hasWebSpeech` once (module-level or lazy) from
  `window.SpeechRecognition || window.webkitSpeechRecognition`.
- If `sttType === 'web-speech'` and `!hasWebSpeech`: set a terminal error like
  *"Web Speech isn't available in this app's window. Choose a Whisper provider in
  Settings, or run the GUI in a Chromium browser."* and **do not** start a
  MediaRecorder recording. Do not silently POST to a backend that cannot serve
  `web-speech`.
- If `sttType === 'web-speech'` and `hasWebSpeech`: existing browser path.
- Otherwise: MediaRecorder → `/api/stt`.
- Expose `mode` and `available` from the hook so `ActionConsole` can hide or
  annotate the mic button instead of failing on use.

Optionally, when the backend STT is configured but the browser path is
unavailable, offer a one-tap "use Whisper instead" that switches the effective
mode for the session.

### 3.2 Gate the provider picker on capability

`SettingsStudio` already has the catalog (`useProviderCatalog`). Use it:

- Annotate the `web-speech` option with a client-side capability check: when
  `hasWebSpeech` is false, render it as `Web Speech API (unavailable in this
  window)` and either disable it or refuse to save it with an inline warning.
- Surface the descriptor `features` (`offline`, `key_required`, `metered`) beside
  each STT option so the tradeoffs are visible, consistent with how the rest of
  the provider UI is moving (`2026-09-24-provider-capability-model-design.md`).
- Remove the hard-coded option list in favour of catalog-driven presets plus the
  browser entry, so a preset added to the catalogue appears without a frontend
  change.

### 3.3 Make a working provider the obvious default

- Keep the neutral `disabled` default (the app cannot assume a local server),
  but on the STT settings panel, when disabled, show a "voice input is off"
  message that names the two supported paths: a local Whisper server
  (`faster-whisper` preset) or `whisper-cli`.
- Order the STT presets so a local option is first, and add model/setup hints in
  the preset descriptions (which are catalogue data, so no frontend change).
- The catalogue remains the single source of provider copy
  (`pkg/provider/sttwebspeech`, `sttwhisperhttp`, `sttwhispercli`).

### 3.4 Backend clarity

- Keep the `web-speech` placeholder, but make its failure message actionable end
  to end. The construction error already maps to `provider_unavailable`
  (`service.go:3059-3066`); ensure the message includes the provider name and the
  reason, e.g. *"stt:web-speech runs in the browser; configure a Whisper
  provider"*.
- Make `TestProvider` for STT return the same actionable message (it already
  carries `GenerationFailure`; verify the message text).
- Do not add a backend `web-speech` implementation; it would be a lie.

### 3.5 Runtime capability signal (small, optional)

A tiny `GET /api/capabilities` returning `{ "webview": "wails"|"browser",
"web_speech": false }` is tempting but cannot know the browser's speech support,
which is the fact that matters. Prefer client-side detection only. If the app
wants to distinguish Wails from browser for messaging, `window._wails` presence
is sufficient and needs no endpoint. Record this as a deliberate non-goal.

### 3.6 Test the full path

- **Go (already strong):** extend `pkg/gui/server_test.go` so the multipart
  `POST /api/stt` path is driven end to end with a fake engine via the existing
  `sttClientFactory` seam if present, or via `Service` injection; assert the JSON
  `{text}` round-trips. Add a test that a configured `web-speech` yields
  `provider_unavailable` with the actionable message.
- **Frontend (new):** the repo has no JS test runner and adding one is a
  larger decision. Instead, provide a `--dry-run`-style manual matrix and, if a
  runner is added later, cover `useVoiceInput` with a fake `SpeechRecognition`
  and a fake `MediaRecorder`.
- **Manual matrix (recorded in the plan):** Wails Linux, Wails macOS, Wails
  Windows, Chromium browser, no-microphone, permission denied, and both STT
  modes against a local Whisper server.

## 4. Interfaces

```ts
// frontend/src/hooks/useVoiceInput.ts
type VoiceMode = 'web-speech' | 'backend' | 'unavailable';
interface UseVoiceInputResult {
  isRecording: boolean;
  isTranscribing: boolean;
  error: string | null;
  mode: VoiceMode;
  available: boolean;
  startRecording: () => void;
  stopRecording: () => void;
}
```

No new HTTP routes are required. `TestProvider`'s STT message text is tightened.

## 5. Error Handling

| Situation | Today | After |
| --- | --- | --- |
| `web-speech` selected, API absent | Silent record → backend error | Immediate message naming the fix; no recording |
| `web-speech` selected, API present | Browser recognition | Unchanged |
| STT disabled | `provider_unavailable` | Same, plus a settings hint naming local options |
| STT provider misconfigured | `provider_unavailable` | Same, with the provider name and reason |
| Mic permission denied | "Microphone permission denied" | Unchanged |
| Empty recording | Silently returns | Unchanged (documented) |

## 6. Testing & Verification

Go (stdlib `testing`):

- `pkg/gui`: multipart `POST /api/stt` round-trips a transcript through a fake
  engine; disabled returns 503 with `provider_unavailable`; `web-speech` returns
  `provider_unavailable` with the actionable message.
- `pkg/media`: `NewSTTClient` for `web-speech` returns the sentinel; `STTKeyFor`
  still reports no key.
- `pkg/gui`: `TestProvider` STT for `web-speech` reports `Success:false` with the
  same message.

Frontend: `tsc` only; verify the unavailable branch by inspection and the manual
matrix. If a test runner is introduced, add the two fake-object tests above.

Manual matrix (must pass and be recorded in the implementation plan): Wails
Linux/macOS/Windows, Chromium, no-mic, permission denied, local Whisper server.

## 7. Compatibility & Rollout

- No config schema change; existing `web-speech` configs begin producing a clear
  error instead of a confusing one.
- The picker change is frontend-only and can ship before the backend message
  tweak.
- The hook's return shape grows; `ActionConsole` must be updated in the same
  change (`noUnusedLocals`/`strict` will catch a missed field).
- Adding a JS test runner is explicitly deferred.

## 8. Open Questions

- Should the app ship a bundled one-tap local Whisper (e.g. a download like the
  sherpa TTS models) so voice input works with zero setup?
- Should the mic button be hidden entirely, or shown disabled with a tooltip,
  when no usable STT is configured?
- Is `interimResults` worth enabling for a faster-feeling web-speech path where
  available?
- Do we want a one-session "use Whisper instead" fallback when web-speech is
  unavailable but a backend engine is configured?
- Should the manual matrix become an automated Wails-driver test
  (`2026-09-25-automated-app-driver-and-otel-debugger-design.md`)?

## 9. References

- Code: `frontend/src/hooks/useVoiceInput.ts:47-157`,
  `frontend/src/components/ActionConsole.tsx:10-30`,
  `frontend/src/components/SettingsStudio.tsx:2406-2459`,
  `frontend/src/hooks/useProviderCatalog.ts:16-60`,
  `frontend/src/api/client.ts:476-480`, `pkg/media/webspeech_stt.go:11-20`,
  `pkg/media/exports.go:66-76`, `pkg/media/providers.go:80-95`,
  `pkg/provider/sttwebspeech/sttwebspeech.go:12-30`,
  `pkg/config/types.go:174-182,433-435`, `pkg/config/presets.go:198-214`,
  `pkg/gui/service.go:2988-3017,3053-3078`, `pkg/gui/server.go:103,1057-1110`.
- Specs: `2026-09-21-real-media-pipelines-and-voice-input-design.md`,
  `2026-09-26-provider-error-surfacing-design.md`,
  `2026-09-24-provider-capability-model-design.md`.

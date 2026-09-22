# Settings TTS Engine Options & Model Download Prompt Design

## Problem
1. **Ambiguous Engine Selector**: In the Settings Studio Media tab, TTS was selected via a generic `builtin` provider type that defaulted to `native-os` (OS synthesizer or procedural audio beeps). `sherpa-onnx` was not exposed as a top-level option or in the sub-dropdown, making it unclear whether built-in speech was using native OS speech or local neural Kokoro voices.
2. **Missing Download Prompt on Preview**: Previewing/testing speech synthesis in Settings Studio when `sherpa-onnx` was configured did not trigger `<ModelDownloadModal>`, but instead either played OS speech (if `native-os` was still selected) or failed with an unformatted red error message without offering to install the missing model.

## Goals
- Flatten and disambiguate TTS provider options in Settings Studio: provide distinct, explicit choices for **Sherpa-ONNX (Kokoro Neural Voice)** and **Native OS Speech (Zero-GPU Fallback)**.
- Provide clear presets in `providerPresets.ts` for both `sherpa-onnx` and `native-os`.
- Display a live model status badge and download button for `kokoro-tts` directly inside Settings Studio when Sherpa-ONNX is active.
- Intercept preview / test requests in Settings Studio (both the main test button and individual NPC voice archetype previews) to immediately launch `ModelDownloadModal` if model weights are not installed.
- Update `Service.TestProvider` backend diagnostics to resolve canonical model paths and return structured `model_missing` responses.

---

## Architecture & Detailed Design

### 1. Settings Studio TTS Engine Selector
In `frontend/src/components/SettingsStudio.tsx`:
- Flatten the primary "TTS Provider" select dropdown so users can directly choose the engine without nested sub-dropdown confusion:
  - `Disabled` (`type: 'disabled'`)
  - `Built-in: Sherpa-ONNX (Kokoro Neural Voice)` (`type: 'builtin', builtin_name: 'sherpa-onnx'`)
  - `Built-in: Native OS Speech (spd-say / SAPI / procedural)` (`type: 'builtin', builtin_name: 'native-os'`)
  - `HTTP Endpoint (Kokoro-FastAPI, AllTalk, OpenAI Speech)` (`type: 'http'`)
  - `CLI Command (e.g. piper)` (`type: 'cli'`)
- When a `builtin` option is selected, `builtin_name` is set accordingly.
- Keep the `providerPresets.ts` dropdown aligned by adding `sherpa-onnx` with default voice `af_bella` and labeling `native-os` clearly.

### 2. Live Model Status in Settings Studio
- When `config.media.tts.type === 'builtin'` and `config.media.tts.builtin_name === 'sherpa-onnx'`:
  - `SettingsStudio` calls `APIClient.getModels()` on mount and listens for updates via `APIClient.subscribeModelEvents()`.
  - An inline card appears beneath the engine selector:
    - **Installed**: Displays a green badge `<CheckCircle /> Kokoro Voice Pack (Installed)`.
    - **Downloading**: Displays a live progress bar with percentage and downloaded megabytes.
    - **Not Installed**: Displays an amber warning `<AlertCircle /> Kokoro Voice Pack (~86 MB) — Not installed` and a `[Download Model]` button that triggers `<ModelDownloadModal>`.

### 3. Speech Test & Preview Interception
- When the user clicks "Test Speech Synthesis" or the play button on any NPC archetype in the Voice Profiles Library:
  - If `config.media.tts.type === 'builtin'` and `config.media.tts.builtin_name === 'sherpa-onnx'`:
    - If `kokoroModelStatus?.installed !== true`:
      - Do not make a failing synthesis request.
      - Immediately open `<ModelDownloadModal>` for `kokoro-tts` (`name: "Kokoro Voice Pack"`, `size_bytes: 90177536`).
  - As a defense-in-depth fallback, if `APIClient.testProvider` returns `{ success: false, model_missing: true }`, open `<ModelDownloadModal>` as well.
- Once downloaded in the modal, the user can click "Done" to dismiss it and then click test/preview to hear the audio.

### 4. Backend Diagnostics (`pkg/gui/service.go`)
- In `Service.TestProvider` for `category == "tts"`:
  - If `ttsCfg.Type == "builtin"` and `(ttsCfg.BuiltinName == "sherpa-onnx" || ttsCfg.BuiltinName == "kokoro")`:
    - If `ttsCfg.ModelPath == ""`, populate it from `s.modelsManager.ModelDir("kokoro-tts")`.
    - Query `s.modelsManager.Status("kokoro-tts")`.
    - If `!status.Installed`, return early with:
      ```go
      return &TestProviderResponseDTO{
          Success:      false,
          ModelMissing: true,
          ModelID:      "kokoro-tts",
          Message:      "Kokoro voice pack is not installed; download required",
      }, nil
      ```
- In `pkg/gui/types.go` and `frontend/src/types.ts`:
  - Add `ModelMissing bool json:"model_missing,omitempty"` and `ModelID string json:"model_id,omitempty"` to `TestProviderResponseDTO` / `TestProviderResponse`.

---

## Testing Plan
1. **Backend Tests**:
   - `pkg/gui/service_test.go`: Test that `TestProvider` for `sherpa-onnx` when model is uninstalled returns `Success: false`, `ModelMissing: true`, `ModelID: "kokoro-tts"`.
   - Test that when model is installed, `TestProvider` resolves the canonical model path and synthesizes audio.
2. **Frontend Type Checks & Build**:
   - `mise run test:frontend` (`npx tsc --noEmit`) passes with 0 errors.
   - `mise run lint` (`go vet ./...`) passes.
   - `mise run build` builds frontend and backend binary cleanly.
3. **Manual / Functional Verification**:
   - In Settings Studio -> Media Engines:
     - Verify dropdown shows distinct choices for Sherpa-ONNX and Native OS Speech.
     - Verify preset loader contains Sherpa-ONNX.
     - Verify status badge displays current install status for Kokoro.
     - Verify clicking Test or preview on uninstalled model triggers `ModelDownloadModal`.

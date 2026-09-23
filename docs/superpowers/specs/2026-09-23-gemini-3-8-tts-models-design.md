# Gemini 3.8 Flash & Flash-Lite TTS Models Design

**Date:** 2026-09-23  
**Status:** Approved

---

## Goal

Add Google's newly released `gemini-3.8-flash-tts` and `gemini-3.8-flash-lite-tts` models to LocalRPG's Gemini TTS provider, updating default presets and Settings Studio controls.

---

## Background

Google released two new dedicated audio text-to-speech models:
- **`gemini-3.8-flash-tts`**: Built for deep creative direction, nuanced character design, and line-by-line acting cues.
- **`gemini-3.8-flash-lite-tts`**: Optimized for high-volume, cost-efficient expressive voice generation with low latency.

These models run on the Gemini API via the same `GenerateContent` audio modality mechanism with prebuilt voices and audio tags.

---

## Changes

### 1. Backend Presets & Defaults

- **`pkg/config/presets.go`**:
  Add `gemini-3.8-flash-tts` and `gemini-3.8-flash-lite-tts` to `TTSPresets`:
  - `gemini-3.8-flash-tts`: Model `gemini-3.8-flash-tts`, default voice `Aoede`
  - `gemini-3.8-flash-lite-tts`: Model `gemini-3.8-flash-lite-tts`, default voice `Aoede`
- **`pkg/media/gemini_tts.go`**:
  Update default model fallback from `gemini-3.1-flash-tts-preview` to `gemini-3.8-flash-tts`.
- **`pkg/config/presets_test.go`**:
  Update preset tests to verify the two new 3.8 presets.

### 2. Frontend Presets & Settings Studio

- **`frontend/src/templates/providerPresets.ts`**:
  Add `gemini-3.8-flash-tts` and `gemini-3.8-flash-lite-tts` presets to `TTS_PRESETS`.
- **`frontend/src/components/SettingsStudio.tsx`**:
  - Update default Gemini TTS model fallback to `gemini-3.8-flash-tts`.
  - Add quick-select pills for:
    - `gemini-3.8-flash-tts` ("3.8 Flash TTS")
    - `gemini-3.8-flash-lite-tts` ("3.8 Flash-Lite TTS")
  - Retain `3.1 Flash TTS`, `2.5 Flash TTS`, and `2.5 Pro TTS` pills for backwards compatibility.

---

## Verification Plan

- Run Go tests: `go test -v ./pkg/config/... ./pkg/media/...`
- Run frontend typecheck: `mise run test:frontend`
- Run full build: `mise run build`

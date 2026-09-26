# Ogg/Opus On-Disk Audio Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Store every generated audio clip as Ogg/Opus, whatever the provider returned, and decode only Opus on playback.

**Architecture:** New `pkg/media/opus` (pion/opus codec + an in-repo Ogg muxer, using pion's `oggreader` to read). `pkg/media.DecodeProviderAudio` normalises PCM/WAV/MP3 to PCM; the TTS pipeline encodes it to Ogg/Opus and caches `.opus`; the player decodes Opus. No migration: the cache is disposable.

**Tech Stack:** Go 1.27, `github.com/pion/opus` (MIT, pinned `@main`), `github.com/gopxl/beep` (resampler), `github.com/hajimehoshi/go-mp3` (MP3 decode).

**Spec:** `docs/superpowers/specs/2026-09-26-ogg-opus-audio-storage-design.md`

## Global Constraints

- Pure Go: no CGO, no shelling out to ffmpeg for audio.
- Every stored clip is Ogg/Opus; playback decodes only Opus.
- No migration or backwards compatibility for existing `.wav`/`.mp3` clips.
- Gates: `mise run test:backend`, `mise run lint`.
- Do not commit unless the user asks.

---

### Task 1: Ogg/Opus codec package

**Files:** `pkg/media/opus/mux.go`, `pkg/media/opus/opus.go`, `pkg/media/opus/opus_test.go`

- [x] Ogg muxer: `Writer` with `OpusHead`/`OpusTags` header pages, lacing, CRC-32 (`0x04c11db7`).
- [x] `Encode(pcm, sampleRate, channels, bitrate)` — resample to 48 kHz mono, pion encoder in 960-sample frames, mux with pre-skip 312.
- [x] `Decode(data)` — `oggreader` + pion decoder, trim pre-skip and trailing padding.
- [x] Tests: round-trip, header parse via `oggreader` (validates CRC), empty-input error.

### Task 2: Provider decode

**Files:** `pkg/media/audiodecode.go`

- [x] `DecodeProviderAudio(data, mimeType)` for headerless PCM, WAV (8/16-bit), and MP3.

### Task 3: Pipeline normalisation

**Files:** `pkg/media/tts.go`, `pkg/media/providers.go`, tests

- [x] Decode provider output then `opus.Encode`; cache `base+".opus"`.
- [x] `cachedClip` accepts only `.opus` (validating the `OggS` header); `AudioExtension`/`AudioContentType` map Ogg to `.opus`/`audio/ogg`.
- [x] `SetOpusBitrate` on the pipeline; default 32 kbps.
- [x] Built-in `echo` provider returns real PCM; test stubs return valid audio.

### Task 4: Playback

**Files:** `pkg/media/playback/player.go`, tests

- [x] `decodeFile` reads the file and decodes Opus to 48 kHz stereo; removed the beep mp3/wav branches.
- [x] Player tests use generated Ogg/Opus fixtures.

### Task 5: Configuration

**Files:** `pkg/config/types.go`, `pkg/config/types_test.go`, wiring

- [x] `media.tts.opus_bitrate` + `Config.OpusBitrate()` (default 32000, clamp 6–510 kbps), wired at the four `NewTTSPipeline` sites.

### Task 6: Verification

- [x] `mise run test:backend` passes; `mise run lint` clean; `pkg/media/opus`, `pkg/media`, `pkg/media/playback`, `pkg/export`, `pkg/config` green.

---

## Self-Review

**Spec coverage:** codec + muxer (Task 1), provider decode (Task 2), normalisation + cache (Task 3), playback (Task 4), config (Task 5).

**Placeholder scan:** none.

**Type consistency:** `opus.Encode(pcm, sampleRate, channels, bitrate)` / `Decode(data)` are used verbatim by the pipeline and player; `SetOpusBitrate` matches `Config.OpusBitrate()`.

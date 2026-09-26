# Design Spec: Ogg/Opus On-Disk Audio

**Date:** 2026-09-26
**Status:** Approved
**Target:** `pkg/media` (new `opus` package, decode/normalise, `tts.go`), `pkg/media/playback`, `pkg/gui`, `pkg/export`, `pkg/config`

---

## 1. Executive Summary

Every audio clip the app caches or bundles is whatever the provider returned: Gemini TTS returns headerless 24 kHz PCM, HTTP TTS returns MP3, other built-ins return WAV. This produced the recent PCM-as-WAV bug and leaves clips several times larger than they need to be.

This spec standardises all locally stored audio on **Ogg/Opus**: any provider output is decoded to PCM, resampled to 48 kHz, encoded as Opus, and written as a standard Ogg/Opus file. Playback then decodes one format only.

Codec: **`github.com/pion/opus`** (MIT, pure Go). Container: an in-repo Ogg muxer (~150 lines; pion ships a reader, `pkg/oggreader`, but no writer). No CGO, no ffmpeg, no copyleft.

Backwards compatibility is explicitly not required: the app is unreleased, so existing cached `.wav`/`.mp3` clips are simply abandoned and regenerated.

---

## 2. Spike Findings (recorded here as the decision basis)

Real 24 kHz mono speech, 35.36 s, PCM 1658 KB, encoded at 48 kHz:

| Bitrate | Opus size | vs PCM | Encode | Decode |
|---|---|---|---|---|
| 16 kbps | 70.8 KB | 23.4x | 114 ms | 28 ms |
| **24 kbps** | 105.3 KB | 15.7x | 124 ms | 31 ms |
| 32 kbps | 139.9 KB | 11.9x | 133 ms | 35 ms |
| 64 kbps | 278.0 KB | 5.96x | 159 ms | 43 ms |

- Ogg container overhead measured at ~108 bytes (0.1%) for a 35 s clip.
- Opus at 24 kbps is ~2.6x smaller than MP3 at 64 kbps.
- Binary cost: +293 KB.
- `kazzmir/opus-go` was also spiked: same bitstream size, but 3.5x slower encode, +720 KB binary, lower-trust dependency. Rejected.
- pion's encoder has no `Lookahead`/`PreSkip`, so a fixed Opus pre-skip of 312 samples at 48 kHz is written into `OpusHead` and trimmed on decode.

---

## 3. Architecture

```
provider bytes (PCM | WAV | MP3)
        |
        v
media.DecodeProviderAudio -> PCM s16 interleaved + rate + channels
        |
        v
beep resample -> 48 kHz mono
        |
        v
pion/opus encoder (960-sample frames, 20 ms) -> Opus packets
        |
        v
media/opus Ogg muxer -> OpusHead | OpusTags | audio pages  ->  <key>.opus
        |
        v
playback: media/opus Decode -> 48 kHz PCM -> oto device
```

---

## 4. Components

### 4.1 `pkg/media/opus` (new)

- `mux.go`: Ogg page writer. `NewWriter(w io.Writer, channels int, inputSampleRate int, preSkip uint16)`, `WritePacket(packet []byte, granule uint64, eos bool) error`, `Close() error`. Writes `OpusHead` (BOS, page 0), `OpusTags` (page 1), then laces audio packets into pages of up to 255 segments with a correct Ogg CRC-32 (poly `0x04c11db7`, no reflection, init 0) and monotonically increasing page sequence. Serial is derived from the clip content or a fixed constant.
- `opus.go`:
  - `const SampleRate = 48000`, `const FrameSamples = 960`, `const PreSkip = 312`, `const DefaultBitrate = 32000`.
  - `Encode(pcm []int16, sampleRate int) ([]byte, error)` — resample to 48 kHz mono (beep), encode 960-sample frames with pion (`WithBitrate(DefaultBitrate)`, `WithComplexity(10)`, `WithApplication(ApplicationVoIP)`, `WithVBR(true)`), mux with `PreSkip`.
  - `Decode(data []byte) (pcm []int16, sampleRate int, channels int, err error)` — parse with `pion/opus/pkg/oggreader`, decode packets with the pion decoder to 48 kHz s16, drop the pre-skip from the start and trim to the final granule at the end. Mono is returned as mono; a stereo stream is downmixed to mono (all provider speech is mono).

### 4.2 `pkg/media` provider decode (`audiodecode.go`)

- `DecodeProviderAudio(data []byte, mimeType string) (pcm []int16, sampleRate int, channels int, err error)`:
  - PCM (`media.IsPCMAudio`): treat `data` as s16le at `PCMSampleRate(mimeType)`.
  - WAV (RIFF/WAVE): parse `fmt ` and `data` chunks, support 16-bit PCM (and skip/convert 8-bit PCM and float if present).
  - MP3 (ID3 tag or MPEG frame sync): decode with `github.com/hajimehoshi/go-mp3` (Apache-2.0).
  - Anything else: error naming the MIME type.

### 4.3 Pipeline (`pkg/media/tts.go`)

- After `client.Synthesize`, normalise: `DecodeProviderAudio` → `opus.Encode`. The cache holds `base+".opus"` only.
- `cachedClip` looks for `.opus` and validates the `OggS` header; a stale or malformed clip is removed and regenerated.
- `AudioExtension` gains an `OggS` → `.opus` case; `AudioContentType` maps `.opus` → `audio/ogg`. The `.wav`/`.mp3` sniffing paths for *provider* input move into `audiodecode.go`.

### 4.4 Playback (`pkg/media/playback/player.go`)

- Replace the beep mp3/wav decode branch with `media/opus.Decode`, which returns 48 kHz s16 mono. The device already runs at 48 kHz stereo, so the player only folds mono into two channels; `beep.Resample` is no longer needed for format conversion (kept only if a source rate ever differs).
- `ErrUnsupportedFormat` now means "not a readable Ogg/Opus clip".

### 4.5 Export

- The web viewer copies the `.opus` asset and plays it with `new Audio(...)`: fine on Chrome/Firefox/Edge; Safari is out of scope for now (worst case, decode to WAV at export time in a follow-up).
- Video export passes the `.opus` file to `ffmpeg`, which reads Ogg/Opus directly; duration probing (`ffprobe`) is unchanged.

### 4.6 Config

- `media.tts.opus_bitrate` (int, default 32000) with an `OpusBitrate()` accessor clamped to 6–510 kbps; surfaced as a numeric field on the Media settings tab.

---

## 5. Non-Goals

- No migration of existing `.wav`/`.mp3` clips; the cache is disposable.
- No Safari-specific decode for the web export (documented follow-up).
- No change to the provider model or the `TTSClient` interface.
- No FLAC/MP3 on disk; Opus only.

---

## 6. Test Strategy

1. `pkg/media/opus`: mux/demux round-trip on a generated tone (pion's `oggreader` validates our CRC and page structure); encode/decode round-trip asserting sample count within one frame and monotonic non-empty packets; pre-skip trimming.
2. `pkg/media`: `DecodeProviderAudio` for raw PCM, WAV, and MP3; pipeline writes a `.opus` cache file and a second synthesis is a cache hit.
3. `pkg/media/playback`: a decode test for a generated Ogg/Opus byte stream.
4. `pkg/config`: `OpusBitrate()` default and clamp.
5. Gates: `mise run test:backend`, `mise run lint`.

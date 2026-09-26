# Pure-Go shirei GUI — Portability Prep Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove two of the three cgo dependencies so the default binary is pure Go: make the built-in sherpa-onnx TTS provider opt-in behind a build tag, and replace the `oto` audio device backend with go-shirei's purego audio.

**Architecture:** Gate `pkg/provider/ttssherpa` (and the one test that imports it) behind `//go:build sherpa`, moving its blank import out of `pkg/provider/all` into tag-guarded files. Rewrite `pkg/media/playback/player.go` on `go.hasen.dev/shirei/audio` plus `app.StartAudio`, keeping the existing `Player` public surface so `pkg/gui/service.go` is the only consumer that changes shape. Output drops from stereo int16 to mono float32, which the accepted-mono decision covers.

**Tech Stack:** Go 1.27.1, `go.hasen.dev/shirei/audio` + `.../app`, `pkg/media/opus` (48 kHz mono s16 decode), `pkg/gui`.

**Spec:** `docs/superpowers/specs/2026-09-26-pure-go-shirei-gui-design.md` (Phase 2, §4.6)

## Global Constraints

- Desktop only; the CGO-free gate (`CGO_ENABLED=0 go build ./...`) cannot pass until Phase 8 removes Wails. This plan only removes the `oto` and sherpa cgo sources.
- Go style: `any` over `interface{}`; `go vet ./...` clean.
- Tests use the standard library only; no testify. No audio device may be required by a test.
- The SPA must keep working: `pkg/gui/service.go`'s calls must still behave (play, stop, status).
- Mono is accepted; do not add a resampler or stereo handling.
- Commits: Conventional Commits with a scope; end with the attribution block shown in Task 1 Step 5.
- `mise run test` must pass (`pkg/gui` is flaky ~1 in 6 on temp-dir cleanup; re-run before calling it a regression).

---

### Task 1: Gate `ttssherpa` behind a `sherpa` build tag

sherpa-onnx is imported at `pkg/provider/ttssherpa/client.go:18` and registered by a blank import at `pkg/provider/all/all.go:24`. An import line cannot carry a build constraint, so the blank import must move to a tag-guarded file.

**Files:**
- Modify: `pkg/provider/all/all.go:24` (remove the import)
- Create: `pkg/provider/all/all_sherpa.go`
- Modify: `pkg/provider/ttssherpa/client.go:1` (add build tag), `pkg/provider/ttssherpa/ttssherpa.go:1`
- Modify: `pkg/provider/ttssherpa/client_test.go:1`, `pkg/provider/ttssherpa/catalog_test.go:1`
- Modify: `pkg/media/speech_cues_test.go:1` (the only other importer)

**Interfaces:**
- Consumes: nothing.
- Produces: `provider.Lookup("tts-sherpa-onnx")` returns `false` by default and `true` under `-tags sherpa`.

- [ ] **Step 1: Write a failing test for the default build**

Create `pkg/provider/all/sherpa_gate_test.go`:

```go
package all

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/provider"
)

func TestSherpaProviderIsOptIn(t *testing.T) {
	if _, ok := provider.Lookup("tts-sherpa-onnx"); ok {
		t.Fatal("tts-sherpa-onnx must not be registered in the default build")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/provider/all/ -run TestSherpaProviderIsOptIn -v`
Expected: FAIL — the provider is currently registered by the blank import.

- [ ] **Step 3: Move the blank import behind the tag**

In `pkg/provider/all/all.go`, delete line 24 (`_ ".../ttssherpa"`).

Create `pkg/provider/all/all_sherpa.go`:

```go
//go:build sherpa

package all

import (
	// Registers the opt-in Sherpa-ONNX Kokoro TTS provider. Requires cgo.
	_ "github.com/darkliquid/localrpg/pkg/provider/ttssherpa"
)
```

Add `//go:build sherpa` as the first line (followed by a blank line) of every `.go` file in `pkg/provider/ttssherpa/`, including both test files, so the package is excluded entirely.

Add `//go:build sherpa` as the first line of `pkg/media/speech_cues_test.go`, which imports `ttssherpa` directly.

- [ ] **Step 4: Verify both configurations**

Run: `go test ./pkg/provider/all/ -run TestSherpaProviderIsOptIn -v`
Expected: PASS.

Run: `go build ./... && go vet ./...`
Expected: clean (the default build no longer compiles sherpa).

Run: `go build -tags sherpa ./pkg/provider/all/`
Expected: clean (sherpa compiles when explicitly requested; it needs cgo, so leave `CGO_ENABLED` at its default).

Run: `go test ./pkg/provider/all/ -tags sherpa -run TestSherpaProviderIsOptIn -v`
Expected: FAIL — with the tag, the provider is registered, which the default-only assertion correctly rejects. This confirms the tag is what controls registration; do not "fix" the test. As a positive check instead, run:

```bash
go test ./pkg/provider/all/ -tags sherpa -run TestNonexistent -count=1
```

Expected: compiles and reports no tests to run, proving the tagged build links sherpa.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
build(tts): make the built-in sherpa provider opt-in

sherpa-onnx is cgo-only and was linked into every binary, which blocked
CGO-free desktop builds. Move its registration behind a `sherpa` build
tag so the default binary is pure Go and the other TTS providers cover
the default; opt in with `go build -tags sherpa`.

💘 Generated with Crush

Assisted-by: Crush:deepseek-v4.1-flash
EOF
)"
```

---

### Task 2: Reimplement the player on shirei audio

`oto` is imported only at `pkg/media/playback/player.go:25`, and `pkg/media/playback` is imported only by `pkg/gui/service.go:26`. Rewrite the player's device backend on `shirei/audio`, keeping `Open`, `Available`, `Playing`, `SetVolume`, `PlayFiles`, `Stop`, `Close`, and both sentinel errors.

**Files:**
- Modify: `pkg/media/playback/player.go` (full rewrite)
- Modify: `pkg/media/playback/player_test.go`
- Modify: `go.mod`, `go.sum`

**Interfaces:**
- Consumes: `opus.Decode` (`pkg/media/opus/opus.go:95`, returns 48 kHz mono `[]int16`), shirei `audio.NewMixer`, `Mixer.SetVolume`, `Mixer.Add`, `Mixer.Fill`, `audio.NewStreamVoice`, `StreamVoice.Write/Close/Release/Render`.
- Produces: unchanged `playback.Player` surface consumed by `pkg/gui/service.go:1750,1762,1771,1777,1867,1868,1874,1883,1884`, plus an injectable device seam for tests.

- [ ] **Step 1: Write the failing tests**

Replace the body of `pkg/media/playback/player_test.go` with device-injected tests. The seam is a package var `startDevice` (defined in Step 3).

```go
package playback

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/media/opus"
)

// useFakeDevice replaces the audio boundary with a draining sink so tests need
// no sound card.
func useFakeDevice(t *testing.T) {
	t.Helper()
	prev := startDevice
	startDevice = func(rate int, fill func([]float32)) error {
		buf := make([]float32, 1024)
		stop := make(chan struct{})
		go func() {
			for {
				select {
				case <-stop:
					return
				default:
					fill(buf)
					time.Sleep(time.Millisecond)
				}
			}
		}()
		t.Cleanup(func() { close(stop) })
		return nil
	}
	t.Cleanup(func() { startDevice = prev })
}

func writeOpusClip(t *testing.T, samples int) string {
	t.Helper()
	pcm := make([]int16, samples)
	for i := range pcm {
		pcm[i] = int16(6000 * ((i / 40) % 2 * 2 - 1))
	}
	data, err := opus.Encode(pcm, opus.SampleRate, 1, opus.DefaultBitrate)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	path := filepath.Join(t.TempDir(), "clip.opus")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestOpenReportsUnavailableWhenDeviceFails(t *testing.T) {
	prev := startDevice
	startDevice = func(int, func([]float32)) error { return ErrUnavailable }
	t.Cleanup(func() { startDevice = prev })

	if _, err := Open(1); err == nil {
		t.Fatal("Open must fail when the device fails")
	}
}

func TestPlayFilesPlaysAndFinishes(t *testing.T) {
	useFakeDevice(t)
	p, err := Open(1)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !p.Available() {
		t.Fatal("player should be available")
	}

	clip := writeOpusClip(t, opus.SampleRate/10) // 100ms
	if err := p.PlayFiles([]string{clip}); err != nil {
		t.Fatalf("PlayFiles: %v", err)
	}
	if !p.Playing() {
		t.Fatal("player should report playing right after PlayFiles")
	}

	deadline := time.Now().Add(3 * time.Second)
	for p.Playing() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if p.Playing() {
		t.Fatal("player should stop reporting playing once the clip is drained")
	}
}

func TestPlayFilesRejectsUndecodableInput(t *testing.T) {
	useFakeDevice(t)
	p, err := Open(1)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	bad := filepath.Join(t.TempDir(), "bad.opus")
	if err := os.WriteFile(bad, []byte("not opus"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := p.PlayFiles([]string{bad}); err != ErrUnsupportedFormat {
		t.Fatalf("PlayFiles(bad) = %v, want ErrUnsupportedFormat", err)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/media/playback/ -run 'TestOpenReportsUnavailable|TestPlayFiles' -v`
Expected: FAIL — `undefined: startDevice` (and the current player uses oto).

- [ ] **Step 3: Rewrite the player**

Replace `pkg/media/playback/player.go` with a shirei-backed implementation. Keep the package doc and both sentinels.

```go
package playback

import (
	"errors"
	"fmt"
	"os"
	"sync"

	app "go.hasen.dev/shirei/app"
	"go.hasen.dev/shirei/audio"

	"github.com/darkliquid/localrpg/pkg/media/opus"
	"github.com/darkliquid/localrpg/pkg/trace"
)

var ErrUnavailable = errors.New("audio playback is unavailable")

var ErrUnsupportedFormat = errors.New("unsupported audio format")

const deviceSampleRate = opus.SampleRate // 48000, mono

// startDevice is the platform audio boundary. Tests replace it with a draining
// sink so no sound card is required.
var startDevice = func(rate int, fill func([]float32)) error {
	return app.StartAudio(rate, app.AudioFillFn(fill))
}

// Player plays decoded speech clips through shirei's mono mixer. The output
// device is process-global and started once (Open); Stop silences the current
// voice rather than closing the device.
type Player struct {
	mu         sync.Mutex
	mixer      *audio.Mixer
	voice      *clipVoice
	playing    bool
	gain       float64
	logger     trace.Logger
	generation uint64
	closed     bool
}

// clipVoice wraps a StreamVoice so the player can observe end-of-clip.
type clipVoice struct {
	stream *audio.StreamVoice
	done   chan struct{}
	once   sync.Once
}

func (v *clipVoice) Render(out []float32) bool {
	alive := v.stream.Render(out)
	if !alive {
		v.once.Do(func() { close(v.done) })
	}
	return alive
}

// Open starts the output device and returns a player at the given volume.
func Open(volume float64) (*Player, error) {
	if volume <= 0 {
		volume = 1.0
	}
	mixer := audio.NewMixer()
	mixer.SetVolume(float32(volume))
	if err := startDevice(deviceSampleRate, mixer.Fill); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return &Player{mixer: mixer, gain: volume}, nil
}

func (p *Player) SetLogger(logger trace.Logger) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.logger = logger
	p.mu.Unlock()
}

// Available reports whether the player can play. Unlike the oto backend it
// cannot be re-opened: the device lives for the process.
func (p *Player) Available() bool {
	return p != nil && p.mixer != nil && !p.closed
}

// Playing reports whether a clip is still sounding.
func (p *Player) Playing() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.playing
}

// SetVolume updates the master volume.
func (p *Player) SetVolume(volume float64) {
	if p == nil {
		return
	}
	if volume <= 0 {
		volume = 1.0
	}
	p.mu.Lock()
	p.gain = volume
	mixer := p.mixer
	p.mu.Unlock()
	if mixer != nil {
		mixer.SetVolume(float32(volume))
	}
}

// PlayFiles decodes and plays the given clips in order, replacing whatever is
// currently sounding. Clips that fail to decode are skipped; if none decode it
// returns ErrUnsupportedFormat.
func (p *Player) PlayFiles(paths []string) error {
	if p == nil {
		return ErrUnavailable
	}
	samples, err := decodeSamples(paths)
	if err != nil {
		return err
	}

	p.mu.Lock()
	if p.closed || p.mixer == nil {
		p.mu.Unlock()
		return ErrUnavailable
	}
	previous := p.voice
	p.generation++
	gen := p.generation
	voice := &clipVoice{
		stream: audio.NewStreamVoice(deviceSampleRate / 2),
		done:   make(chan struct{}),
	}
	p.voice = voice
	p.playing = true
	logger := p.logger
	mixer := p.mixer
	p.mu.Unlock()

	if previous != nil {
		previous.stream.Release()
	}
	if logger != nil {
		logger.Event("audio.play", map[string]any{"clips": len(paths), "volume": p.gain})
	}

	mixer.Add(voice)
	go func() {
		defer voice.stream.Close()
		const chunk = 4096
		for pos := 0; pos < len(samples); pos += chunk {
			end := min(pos+chunk, len(samples))
			if _, err := voice.stream.Write(samples[pos:end]); err != nil {
				return
			}
		}
	}()
	go func() {
		<-voice.done
		p.mu.Lock()
		if p.generation == gen && p.voice == voice {
			p.playing = false
			p.voice = nil
		}
		p.mu.Unlock()
	}()
	return nil
}

// Stop silences the current clip.
func (p *Player) Stop() {
	if p == nil {
		return
	}
	p.mu.Lock()
	voice := p.voice
	p.voice = nil
	p.playing = false
	p.generation++
	p.mu.Unlock()
	if voice != nil {
		voice.stream.Release()
	}
}

// Close silences playback and marks the player unusable. The device stays open
// for the process, matching shirei's StartAudio contract.
func (p *Player) Close() error {
	if p == nil {
		return nil
	}
	p.Stop()
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	return nil
}

// decodeSamples concatenates the clips into one mono float32 buffer. Opus
// decoding always yields 48 kHz mono s16, matching the device rate.
func decodeSamples(paths []string) ([]float32, error) {
	var out []float32
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		pcm, _, _, err := opus.Decode(data)
		if err != nil {
			continue
		}
		for _, s := range pcm {
			out = append(out, float32(s)/32768.0)
		}
	}
	if len(out) == 0 {
		return nil, ErrUnsupportedFormat
	}
	return out, nil
}
```

Verify against the vendored shirei `audio` package that `NewStreamVoice`, `Write`, `Close`, and `Release` have the signatures assumed here (`audio/stream.go`: `NewStreamVoice(bufSamples int) *StreamVoice`, `Write(samples []float32) (int, error)`, `Close() error`, `Release()`).

- [ ] **Step 4: Run the player tests**

Run: `go test ./pkg/media/playback/ -v`
Expected: PASS. If `TestPlayFilesPlaysAndFinishes` hangs, the fake device's `fill` loop is not draining; ensure it runs continuously until cleanup.

- [ ] **Step 5: Drop oto**

```bash
go mod tidy
```

Run: `rg -n 'oto/v3|ebitengine/oto' go.mod go.sum pkg/`
Expected: no matches. (`github.com/ebitengine/purego` remains as a shirei indirect.)

- [ ] **Step 6: Verify the GUI consumer still builds and tests**

Run: `go build ./... && go vet ./...`
Expected: clean.

Run: `go test ./pkg/gui/ -run 'Audio|PlayTurn|PlaySegment' -v`
Expected: PASS (re-run once if the known temp-dir flake appears).

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
refactor(audio): play through shirei's purego mixer instead of oto

oto links a cgo C++ backend on Linux, blocking CGO-free builds. The
clip pipeline already yields 48 kHz mono PCM, so decode straight into
shirei's mono float32 mixer and keep the player's public surface, with
the device boundary injectable so tests need no sound card.

💘 Generated with Crush

Assisted-by: Crush:deepseek-v4.1-flash
EOF
)"
```

---

### Task 3: Confirm the remaining cgo source is Wails alone

**Files:** none (verification only).

**Interfaces:**
- Consumes: Tasks 1-2.
- Produces: documented evidence of the remaining cgo blocker.

- [ ] **Step 1: Confirm the default build no longer links oto or sherpa**

Run: `CGO_ENABLED=0 go build ./pkg/media/playback ./pkg/provider/all`
Expected: clean.

Run: `go list -deps -f '{{.ImportPath}}' ./cmd/localrpg | rg -i 'oto|sherpa'`
Expected: no matches (default build).

- [ ] **Step 2: Confirm the remaining failure is Wails**

Run: `CGO_ENABLED=0 go build ./... 2>&1 | head`
Expected: the only failure mentions the Wails `pkg/application` Linux cgo files. If any other package appears, stop and report.

- [ ] **Step 3: Full gate**

Run: `mise run test`
Expected: PASS (re-run if the `pkg/gui` flake triggers).

- [ ] **Step 4: No commit** (verification task).

---

## Self-Review

**Spec coverage (Phase 2, §4.6):**

| Spec item | Task |
| --- | --- |
| build-tag `ttssherpa` | Task 1 |
| swap `oto` for shirei audio | Task 2 |
| CGO-free gate (deferred to teardown) | Task 3 documents that only Wails remains; the gate itself lands in Phase 8 |

**Placeholder scan:** No TBDs. Task 2 Step 3 instructs the implementer to confirm the shirei `audio` signatures against the source; that is a factual verification, not a placeholder.

**Type consistency:** `startDevice`, `clipVoice`, `decodeSamples`, and the `Player` methods are defined once and used consistently in the tests. `ErrUnavailable`/`ErrUnsupportedFormat` retain their existing names and meanings.

**Known deferrals (not gaps):** the root-boot switch, screen ports, theatre work, and Wails removal are later plans. Mono output is intentionally accepted.

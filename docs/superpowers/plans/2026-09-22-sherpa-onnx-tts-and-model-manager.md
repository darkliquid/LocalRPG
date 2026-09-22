# Sherpa-ONNX TTS, Oto v3 Playback & Model Manager Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver an out-of-the-box, in-process neural text-to-speech engine using Sherpa-ONNX (Kokoro-82M) and a modern `ebitengine/oto/v3` audio device backend, powered by an atomic on-demand model download manager with real-time UI progress.

**Architecture:** Replace miniaudio (`mago`) with `ebitengine/oto/v3` in `pkg/media/playback`. Add `pkg/models/manager.go` to handle downloading, SHA-256 verification, and atomic extraction of model packages into user cache. Build `pkg/media/sherpa_tts.go` implementing `media.TTSClient` using `sherpa-onnx-go` to map Kokoro voice IDs and generate 24kHz PCM WAV bytes. Expose model status and SSE download progress via `/api/models`, and conditionally prompt the user in the frontend when built-in TTS is enabled and model weights are missing.

**Tech Stack:** Go 1.27.1, `github.com/k2-fsa/sherpa-onnx-go`, `github.com/ebitengine/oto/v3`, `github.com/gopxl/beep`, React 19, TypeScript, Tailwind v4, Server-Sent Events (SSE).

---

## File Structure Map

| File Path | Responsibility |
| :--- | :--- |
| `pkg/media/playback/player.go` | Process-wide audio device playback using `ebitengine/oto/v3` and `beep` streaming |
| `pkg/media/playback/player_test.go` | Unit tests for Oto-based audio player, gain scaling, and queue interruption |
| `pkg/models/manager.go` | Model lifecycle manager: download on demand, SHA-256 check, atomic archive extraction |
| `pkg/models/manager_test.go` | Unit tests for model status, download streaming, cancellation, and checksum validation |
| `pkg/media/sherpa_tts.go` | Sherpa-ONNX TTS client implementing `TTSClient`, voice mapping, and WAV serialization |
| `pkg/media/sherpa_tts_test.go` | Tests for Kokoro speaker mapping, WAV encoding, and missing model fallback |
| `pkg/media/providers.go` | Wire `sherpa-onnx` into `media.NewTTSClient` factory |
| `pkg/config/types.go` | Update `TTSConfig` to support `sherpa-onnx` and model paths |
| `pkg/config/presets.go` | Add `sherpa-onnx` preset with default Kokoro model mapping |
| `pkg/gui/service.go` | Add `GetModelsStatus`, `DownloadModel`, SSE event streaming, and `model_missing` turn event |
| `pkg/gui/server.go` | Map `/api/models` routes (`GET`, `POST /:id/download`, `GET /events`) |
| `pkg/gui/server_test.go` | HTTP tests for model management endpoints |
| `frontend/src/types.ts` | TypeScript interfaces for `ModelStatus` and turn stream events |
| `frontend/src/api/client.ts` | API methods to query model status, trigger download, and subscribe to SSE |
| `frontend/src/components/turn/ModelDownloadModal.tsx` | Confirmation dialog with progress bar for downloading voice packs |
| `frontend/src/components/turn/ActionConsole.tsx` | Wire `model_missing` event handling and download prompt trigger |

---

### Task 1: Migrate Audio Device Playback from Mago to Oto v3

**Files:**
- Modify: `go.mod`
- Modify: `pkg/media/playback/player.go`
- Modify: `pkg/media/playback/player_test.go`

- [ ] **Step 1: Update Go dependencies to add `oto/v3` and remove `mago`**

Run:
```bash
go get github.com/ebitengine/oto/v3@v3.1.0
go mod tidy
```

- [ ] **Step 2: Write failing unit test for `oto/v3` playback player in `pkg/media/playback/player_test.go`**

Replace `pkg/media/playback/player_test.go`:
```go
package playback

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/gopxl/beep"
)

// generateSineTone creates a small in-memory beep.StreamSeekCloser of a sine wave.
func generateSineTone(sampleRate beep.SampleRate, freq float64, duration time.Duration) beep.StreamSeekCloser {
	totalSamples := int(sampleRate.D(duration))
	var buf bytes.Buffer
	for i := 0; i < totalSamples; i++ {
		t := float64(i) / float64(sampleRate)
		val := math.Sin(2 * math.Pi * freq * t)
		sample := int16(val * 32767)
		_ = binary.Write(&buf, binary.LittleEndian, sample)
		_ = binary.Write(&buf, binary.LittleEndian, sample) // stereo
	}
	return &testSineStreamer{
		samples: totalSamples,
		pos:     0,
		freq:    freq,
		rate:    sampleRate,
	}
}

type testSineStreamer struct {
	samples int
	pos     int
	freq    float64
	rate    beep.SampleRate
}

func (s *testSineStreamer) Stream(samples [][2]float64) (n int, ok bool) {
	if s.pos >= s.samples {
		return 0, false
	}
	for i := range samples {
		if s.pos >= s.samples {
			return i, true
		}
		t := float64(s.pos) / float64(s.rate)
		val := math.Sin(2 * math.Pi * s.freq * t)
		samples[i][0] = val
		samples[i][1] = val
		s.pos++
	}
	return len(samples), true
}

func (s *testSineStreamer) Err() error { return nil }
func (s *testSineStreamer) Len() int   { return s.samples }
func (s *testSineStreamer) Position() int { return s.pos }
func (s *testSineStreamer) Seek(p int) error {
	s.pos = p
	return nil
}
func (s *testSineStreamer) Close() error { return nil }

func TestPlayerLifecycle(t *testing.T) {
	// Attempt to open player
	p, err := Open(0.8)
	if err != nil {
		if err == ErrUnavailable {
			t.Skip("audio hardware unavailable on host; skipping device test")
		}
		t.Fatalf("unexpected open error: %v", err)
	}
	defer p.Close()

	if p.Gain() != 0.8 {
		t.Errorf("expected gain 0.8, got %v", p.Gain())
	}
	p.SetGain(1.0)
	if p.Gain() != 1.0 {
		t.Errorf("expected gain 1.0, got %v", p.Gain())
	}

	streamer := generateSineTone(beep.SampleRate(deviceSampleRate), 440, 50*time.Millisecond)
	p.Play(streamer, streamer)

	time.Sleep(20 * time.Millisecond)
	p.Stop()

	if p.IsPlaying() {
		t.Error("expected player to not be playing after Stop()")
	}
}
```

- [ ] **Step 3: Run the test to confirm failure before implementation**

Run: `go test -v -run TestPlayerLifecycle ./pkg/media/playback/`
Expected: Compilation failure due to unresolved `mago` or missing methods in `player.go`.

- [ ] **Step 4: Implement `pkg/media/playback/player.go` using `ebitengine/oto/v3`**

Rewrite `pkg/media/playback/player.go`:
```go
package playback

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/trace"
	"github.com/ebitengine/oto/v3"
	"github.com/gopxl/beep"
)

var ErrUnavailable = errors.New("audio playback is unavailable")
var ErrUnsupportedFormat = errors.New("unsupported audio format")

const (
	deviceChannels   = 2
	deviceSampleRate = 48000
	bufferDuration   = 200 * time.Millisecond
)

type Player struct {
	otoCtx   *oto.Context
	otoReady chan struct{}
	mu       sync.Mutex
	player   *oto.Player
	streamer beep.Streamer
	closers  []io.Closer
	gain     float64
	playing  bool
	logger   trace.Logger
}

func Open(volume float64) (*Player, error) {
	if volume <= 0 {
		volume = 1.0
	}

	readyChan := make(chan struct{})
	options := &oto.NewContextOptions{
		SampleRate:   deviceSampleRate,
		ChannelCount: deviceChannels,
		Format:       oto.FormatSignedInt16LE,
		BufferSize:   bufferDuration,
	}

	otoCtx, ready, err := oto.NewContext(options)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}

	p := &Player{
		otoCtx:   otoCtx,
		otoReady: readyChan,
		gain:     volume,
	}

	go func() {
		<-ready
		close(readyChan)
	}()

	select {
	case <-readyChan:
	case <-time.After(2 * time.Second):
		// Context took too long to ready
	}

	return p, nil
}

func (p *Player) SetLogger(logger trace.Logger) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.logger = trace.OrNil(logger)
}

func (p *Player) Gain() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.gain
}

func (p *Player) SetGain(volume float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if volume < 0 {
		volume = 0
	}
	p.gain = volume
	if p.player != nil {
		p.player.SetVolume(p.gain)
	}
}

func (p *Player) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.playing && p.player != nil && p.player.IsPlaying()
}

func (p *Player) Play(streamer beep.Streamer, closers ...io.Closer) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stopLocked()

	if streamer == nil {
		return
	}

	p.streamer = streamer
	p.closers = closers

	reader := &streamerReader{
		streamer: streamer,
		gain:     p.gain,
	}

	player := p.otoCtx.NewPlayer(reader)
	player.SetVolume(p.gain)
	p.player = player
	p.playing = true

	player.Play()

	go func() {
		for {
			time.Sleep(50 * time.Millisecond)
			p.mu.Lock()
			if p.player != player || !player.IsPlaying() {
				p.mu.Unlock()
				break
			}
			p.mu.Unlock()
		}
	}()
}

func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
}

func (p *Player) stopLocked() {
	if p.player != nil {
		_ = p.player.Close()
		p.player = nil
	}
	for _, c := range p.closers {
		_ = c.Close()
	}
	p.closers = nil
	p.streamer = nil
	p.playing = false
}

func (p *Player) Close() error {
	p.Stop()
	return nil
}

type streamerReader struct {
	streamer beep.Streamer
	gain     float64
	buf      [][2]float64
	tmp      []byte
}

func (sr *streamerReader) Read(p []byte) (int, error) {
	samplesWanted := len(p) / 4 // 2 channels * 2 bytes per sample (int16)
	if samplesWanted == 0 {
		return 0, nil
	}

	if cap(sr.buf) < samplesWanted {
		sr.buf = make([][2]float64, samplesWanted)
	}
	buf := sr.buf[:samplesWanted]

	n, ok := sr.streamer.Stream(buf)
	if !ok || n == 0 {
		return 0, io.EOF
	}

	for i := 0; i < n; i++ {
		left := math.Max(-1, math.Min(1, buf[i][0]))
		right := math.Max(-1, math.Min(1, buf[i][1]))

		leftInt := int16(left * 32767)
		rightInt := int16(right * 32767)

		offset := i * 4
		binary.LittleEndian.PutUint16(p[offset:], uint16(leftInt))
		binary.LittleEndian.PutUint16(p[offset+2:], uint16(rightInt))
	}

	return n * 4, nil
}
```

- [ ] **Step 5: Run tests to verify audio player passes**

Run: `go test -v -count=1 ./pkg/media/playback/`
Expected: PASS (or SKIP on machines with no sound device).

- [ ] **Step 6: Commit audio player migration**

```bash
git add go.mod go.sum pkg/media/playback/
git commit -m "feat(media): migrate playback device backend from mago to oto v3"
```

---

### Task 2: Model Download & Cache Manager

**Files:**
- Create: `pkg/models/manager.go`
- Create: `pkg/models/manager_test.go`

- [ ] **Step 1: Write failing unit test for `pkg/models/manager_test.go`**

Create `pkg/models/manager_test.go`:
```go
package models

import (
	"archive/tar"
	"bytes"
	"compress/bzip2"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// createMockTarBz2 generates an in-memory tar.bz2 containing test model files.
func createMockTar(t *testing.T) ([]byte, string) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	files := map[string]string{
		"model.onnx":  "fake onnx weights",
		"voices.bin":  "fake voice styles",
		"tokens.txt":  "fake tokens",
		"espeak-ng-data/phontab": "fake phontab data",
	}

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("write tar body: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}

	tarBytes := buf.Bytes()
	sum := sha256.Sum256(tarBytes)
	return tarBytes, hex.EncodeToString(sum[:])
}

func TestManagerDownloadAndVerify(t *testing.T) {
	cacheDir := t.TempDir()
	tarData, checksum := createMockTar(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", string(rune(len(tarData))))
		_, _ = w.Write(tarData)
	}))
	defer server.Close()

	mgr := NewManager(cacheDir)
	spec := ModelSpec{
		ID:          "test-tts",
		Name:        "Test TTS Model",
		URL:         server.URL,
		SHA256:      checksum,
		SizeBytes:   int64(len(tarData)),
		ArchiveType: "tar",
		Subdir:      "tts/test",
		RequiredFiles: []string{"model.onnx", "voices.bin", "tokens.txt"},
	}
	mgr.RegisterSpec(spec)

	status := mgr.Status("test-tts")
	if status.Installed {
		t.Fatalf("expected model to not be installed initially")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events, err := mgr.Download(ctx, "test-tts")
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	var lastStatus ModelStatus
	for s := range events {
		lastStatus = s
	}

	if !lastStatus.Installed {
		t.Fatalf("expected model to be installed after download, got error: %s", lastStatus.Error)
	}

	// Verify required files exist on disk
	destDir := filepath.Join(cacheDir, "models", "tts", "test")
	if _, err := os.Stat(filepath.Join(destDir, "model.onnx")); err != nil {
		t.Errorf("model.onnx missing: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestManagerDownloadAndVerify ./pkg/models/`
Expected: FAIL with "cannot find package or undefined NewManager".

- [ ] **Step 3: Implement `pkg/models/manager.go`**

Create `pkg/models/manager.go`:
```go
package models

import (
	"archive/tar"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

var (
	ErrModelNotFound   = errors.New("model not found in registry")
	ErrDownloadActive  = errors.New("model download is already active")
	ErrChecksumMismatch = errors.New("downloaded archive failed checksum verification")
)

type ModelSpec struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	URL           string   `json:"url"`
	SHA256        string   `json:"sha256"`
	SizeBytes     int64    `json:"size_bytes"`
	ArchiveType   string   `json:"archive_type"` // "tar", "tar.gz", "tar.bz2"
	Subdir        string   `json:"subdir"`
	RequiredFiles []string `json:"required_files"`
}

type ModelStatus struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Installed       bool    `json:"installed"`
	Downloading     bool    `json:"downloading"`
	Progress        float64 `json:"progress"`
	BytesDownloaded int64   `json:"bytes_downloaded"`
	TotalBytes      int64   `json:"total_bytes"`
	Error           string  `json:"error,omitempty"`
}

type Manager struct {
	cacheDir  string
	mu        sync.RWMutex
	specs     map[string]ModelSpec
	active    map[string]*downloadSession
	listeners map[chan ModelStatus]struct{}
}

type downloadSession struct {
	status   ModelStatus
	cancel   context.CancelFunc
	channels []chan ModelStatus
}

func NewManager(cacheDir string) *Manager {
	m := &Manager{
		cacheDir:  cacheDir,
		specs:     make(map[string]ModelSpec),
		active:    make(map[string]*downloadSession),
		listeners: make(map[chan ModelStatus]struct{}),
	}
	m.registerDefaultSpecs()
	return m
}

func (m *Manager) registerDefaultSpecs() {
	m.specs["kokoro-tts"] = ModelSpec{
		ID:          "kokoro-tts",
		Name:        "Kokoro Voice Pack",
		URL:         "https://github.com/k2-fsa/sherpa-onnx/releases/download/tts-models/kokoro-en-v0_19.tar.bz2",
		SHA256:      "a3d3c82e666c0d0a2dbe5429399432d67786440dbd06b539bf58778f654b50c0",
		SizeBytes:   90177536,
		ArchiveType: "tar.bz2",
		Subdir:      filepath.Join("tts", "kokoro"),
		RequiredFiles: []string{
			"model.onnx",
			"voices.bin",
			"tokens.txt",
			"espeak-ng-data",
		},
	}
}

func (m *Manager) RegisterSpec(spec ModelSpec) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.specs[spec.ID] = spec
}

func (m *Manager) ModelDir(modelID string) string {
	m.mu.RLock()
	spec, ok := m.specs[modelID]
	m.mu.RUnlock()
	if !ok {
		return ""
	}
	return filepath.Join(m.cacheDir, "models", spec.Subdir)
}

func (m *Manager) Status(modelID string) ModelStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	spec, ok := m.specs[modelID]
	if !ok {
		return ModelStatus{ID: modelID, Error: "unknown model"}
	}

	if sess, ok := m.active[modelID]; ok {
		return sess.status
	}

	installed := m.isInstalledLocked(spec)
	return ModelStatus{
		ID:         spec.ID,
		Name:       spec.Name,
		Installed:  installed,
		TotalBytes: spec.SizeBytes,
	}
}

func (m *Manager) ListStatuses() []ModelStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make([]ModelStatus, 0, len(m.specs))
	for _, spec := range m.specs {
		if sess, ok := m.active[spec.ID]; ok {
			statuses = append(statuses, sess.status)
		} else {
			statuses = append(statuses, ModelStatus{
				ID:         spec.ID,
				Name:       spec.Name,
				Installed:  m.isInstalledLocked(spec),
				TotalBytes: spec.SizeBytes,
			})
		}
	}
	return statuses
}

func (m *Manager) isInstalledLocked(spec ModelSpec) bool {
	destDir := filepath.Join(m.cacheDir, "models", spec.Subdir)
	for _, req := range spec.RequiredFiles {
		if _, err := os.Stat(filepath.Join(destDir, req)); err != nil {
			return false
		}
	}
	return true
}

func (m *Manager) Subscribe() chan ModelStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch := make(chan ModelStatus, 16)
	m.listeners[ch] = struct{}{}
	return ch
}

func (m *Manager) Unsubscribe(ch chan ModelStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.listeners, ch)
	close(ch)
}

func (m *Manager) broadcast(status ModelStatus) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for ch := range m.listeners {
		select {
		case ch <- status:
		default:
		}
	}
}

func (m *Manager) Download(ctx context.Context, modelID string) (<-chan ModelStatus, error) {
	m.mu.Lock()
	spec, ok := m.specs[modelID]
	if !ok {
		m.mu.Unlock()
		return nil, ErrModelNotFound
	}

	if _, ok := m.active[modelID]; ok {
		m.mu.Unlock()
		return nil, ErrDownloadActive
	}

	downloadCtx, cancel := context.WithCancel(ctx)
	statusCh := make(chan ModelStatus, 16)
	session := &downloadSession{
		status: ModelStatus{
			ID:          modelID,
			Name:        spec.Name,
			Downloading: true,
			TotalBytes:  spec.SizeBytes,
		},
		cancel:   cancel,
		channels: []chan ModelStatus{statusCh},
	}
	m.active[modelID] = session
	m.mu.Unlock()

	go m.runDownload(downloadCtx, spec, session)

	return statusCh, nil
}

func (m *Manager) runDownload(ctx context.Context, spec ModelSpec, session *downloadSession) {
	updateStatus := func(update func(s *ModelStatus)) {
		m.mu.Lock()
		update(&session.status)
		current := session.status
		for _, ch := range session.channels {
			select {
			case ch <- current:
			default:
			}
		}
		m.mu.Unlock()
		m.broadcast(current)
	}

	cleanup := func(err error) {
		m.mu.Lock()
		if err != nil {
			session.status.Downloading = false
			session.status.Error = err.Error()
		} else {
			session.status.Downloading = false
			session.status.Installed = true
			session.status.Progress = 1.0
			session.status.Error = ""
		}
		finalStatus := session.status
		for _, ch := range session.channels {
			select {
			case ch <- finalStatus:
			default:
			}
			close(ch)
		}
		delete(m.active, spec.ID)
		m.mu.Unlock()
		m.broadcast(finalStatus)
	}

	tmpDir := filepath.Join(m.cacheDir, "models", "tmp")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		cleanup(fmt.Errorf("create tmp dir: %w", err))
		return
	}

	partPath := filepath.Join(tmpDir, spec.ID+".part")
	defer os.Remove(partPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, spec.URL, nil)
	if err != nil {
		cleanup(fmt.Errorf("prepare request: %w", err))
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cleanup(fmt.Errorf("download request: %w", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		cleanup(fmt.Errorf("download HTTP error %d: %s", resp.StatusCode, resp.Status))
		return
	}

	outFile, err := os.Create(partPath)
	if err != nil {
		cleanup(fmt.Errorf("create part file: %w", err))
		return
	}
	defer outFile.Close()

	hasher := sha256.New()
	writer := io.MultiWriter(outFile, hasher)

	buf := make([]byte, 64*1024)
	var downloaded int64
	total := spec.SizeBytes
	if resp.ContentLength > 0 {
		total = resp.ContentLength
	}

	for {
		select {
		case <-ctx.Done():
			cleanup(ctx.Err())
			return
		default:
		}

		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := writer.Write(buf[:n]); werr != nil {
				cleanup(fmt.Errorf("write part file: %w", werr))
				return
			}
			downloaded += int64(n)
			prog := 0.0
			if total > 0 {
				prog = float64(downloaded) / float64(total)
			}
			updateStatus(func(s *ModelStatus) {
				s.BytesDownloaded = downloaded
				s.TotalBytes = total
				s.Progress = prog
			})
		}
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			cleanup(fmt.Errorf("download stream read: %w", rerr))
			return
		}
	}

	// Verify checksum
	calculatedSum := hex.EncodeToString(hasher.Sum(nil))
	if spec.SHA256 != "" && calculatedSum != spec.SHA256 {
		cleanup(fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, spec.SHA256, calculatedSum))
		return
	}

	_ = outFile.Close()

	// Extract to staging directory
	stagingDir := filepath.Join(tmpDir, spec.ID+"_extracted")
	_ = os.RemoveAll(stagingDir)
	if err := os.MkdirAll(stagingDir, 0755); err != nil {
		cleanup(fmt.Errorf("create staging dir: %w", err))
		return
	}
	defer os.RemoveAll(stagingDir)

	if err := m.extractArchive(partPath, spec.ArchiveType, stagingDir); err != nil {
		cleanup(fmt.Errorf("extract archive: %w", err))
		return
	}

	// Atomically move to target directory
	targetDir := filepath.Join(m.cacheDir, "models", spec.Subdir)
	_ = os.RemoveAll(targetDir)
	if err := os.MkdirAll(filepath.Dir(targetDir), 0755); err != nil {
		cleanup(fmt.Errorf("create parent dir: %w", err))
		return
	}

	// Check if extracted contents are nested in a single root directory
	entries, err := os.ReadDir(stagingDir)
	if err == nil && len(entries) == 1 && entries[0].IsDir() {
		nested := filepath.Join(stagingDir, entries[0].Name())
		if err := os.Rename(nested, targetDir); err != nil {
			cleanup(fmt.Errorf("move extracted folder: %w", err))
			return
		}
	} else {
		if err := os.Rename(stagingDir, targetDir); err != nil {
			cleanup(fmt.Errorf("move extracted files: %w", err))
			return
		}
	}

	cleanup(nil)
}

func (m *Manager) extractArchive(archivePath, archiveType, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	var tr *tar.Reader
	switch archiveType {
	case "tar":
		tr = tar.NewReader(f)
	case "tar.gz":
		gz, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gz.Close()
		tr = tar.NewReader(gz)
	case "tar.bz2":
		bz := bzip2.NewReader(f)
		tr = tar.NewReader(bz)
	default:
		return fmt.Errorf("unsupported archive format: %s", archiveType)
	}

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		cleanPath := filepath.Clean(header.Name)
		if filepath.IsAbs(cleanPath) || cleanPath == ".." || len(cleanPath) > 2 && cleanPath[:3] == "../" {
			continue // Zip Slip prevention
		}

		target := filepath.Join(destDir, cleanPath)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}
```

- [ ] **Step 4: Run unit tests to verify `pkg/models` passes**

Run: `go test -v -count=1 ./pkg/models/`
Expected: PASS.

- [ ] **Step 5: Commit `pkg/models` manager**

```bash
git add pkg/models/
git commit -m "feat(models): add model download manager with sha256 check and atomic unpack"
```

---

### Task 3: Sherpa-ONNX TTS Provider & Voice Profile Mapping

**Files:**
- Create: `pkg/media/sherpa_tts.go`
- Create: `pkg/media/sherpa_tts_test.go`
- Modify: `pkg/media/providers.go`

- [ ] **Step 1: Install `sherpa-onnx-go` dependency**

Run:
```bash
go get github.com/k2-fsa/sherpa-onnx-go@v1.13.8
go mod tidy
```

- [ ] **Step 2: Write failing unit test for `pkg/media/sherpa_tts_test.go`**

Create `pkg/media/sherpa_tts_test.go`:
```go
package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestKokoroVoiceMapping(t *testing.T) {
	tests := []struct {
		voiceID string
		wantSid int
	}{
		{"af_bella", 2},
		{"am_adam", 11},
		{"bm_george", 25},
		{"bf_emma", 20},
		{"unknown_voice", 0}, // fallback
		{"", 0},
	}

	for _, tt := range tests {
		got := ResolveKokoroSpeakerID(tt.voiceID)
		if got != tt.wantSid {
			t.Errorf("ResolveKokoroSpeakerID(%q) = %d; want %d", tt.voiceID, got, tt.wantSid)
		}
	}
}

func TestEncodePCMToWAV(t *testing.T) {
	samples := []float32{0.0, 0.5, -0.5, 1.0, -1.0}
	sampleRate := 24000

	wavBytes, err := EncodePCMFloatToWAV(samples, sampleRate)
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}

	if len(wavBytes) != 44+len(samples)*2 {
		t.Errorf("expected %d bytes, got %d", 44+len(samples)*2, len(wavBytes))
	}

	// Verify header tags
	if string(wavBytes[0:4]) != "RIFF" {
		t.Errorf("expected RIFF header, got %q", string(wavBytes[0:4]))
	}
	if string(wavBytes[8:12]) != "WAVE" {
		t.Errorf("expected WAVE format, got %q", string(wavBytes[8:12]))
	}

	var rate uint32
	_ = binary.Read(bytes.NewReader(wavBytes[24:28]), binary.LittleEndian, &rate)
	if rate != 24000 {
		t.Errorf("expected 24000 sample rate, got %d", rate)
	}
}

func TestSherpaTTSMissingModelReturnsError(t *testing.T) {
	client := NewSherpaTTSClient(t.TempDir())
	_, err := client.Synthesize(context.Background(), "Hello test", &entity.VoiceConfig{VoiceID: "af_bella"})
	if err != ErrModelNotLoaded {
		t.Errorf("expected ErrModelNotLoaded, got %v", err)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test -v -run TestKokoroVoiceMapping ./pkg/media/`
Expected: FAIL with undefined `ResolveKokoroSpeakerID`.

- [ ] **Step 4: Implement `pkg/media/sherpa_tts.go`**

Create `pkg/media/sherpa_tts.go`:
```go
package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/trace"
	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

var ErrModelNotLoaded = errors.New("sherpa-onnx model weights are not loaded")

var kokoroSpeakerMap = map[string]int{
	"af_alloy":    0,
	"af_aoede":    1,
	"af_bella":    2,
	"af_heart":    3,
	"af_jessica":  4,
	"af_kore":     5,
	"af_nicole":   6,
	"af_nova":     7,
	"af_river":    8,
	"af_sarah":    9,
	"af_sky":      10,
	"am_adam":     11,
	"am_echo":     12,
	"am_eric":     13,
	"am_fenrir":   14,
	"am_liam":     15,
	"am_michael":  16,
	"am_onyx":     17,
	"am_puck":     18,
	"bf_alice":    19,
	"bf_emma":     20,
	"bf_isabella": 21,
	"bf_lily":     22,
	"bm_daniel":   23,
	"bm_fable":    24,
	"bm_george":   25,
	"bm_lewis":    26,
}

func ResolveKokoroSpeakerID(voiceID string) int {
	if sid, ok := kokoroSpeakerMap[voiceID]; ok {
		return sid
	}
	return 0
}

type SherpaTTSClient struct {
	modelDir string
	tts      *sherpa.OfflineTts
	mu       sync.Mutex
	logger   trace.Logger
}

func NewSherpaTTSClient(modelDir string) *SherpaTTSClient {
	return &SherpaTTSClient{modelDir: modelDir}
}

func (s *SherpaTTSClient) SetLogger(logger trace.Logger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logger = trace.OrNil(logger)
}

func (s *SherpaTTSClient) ensureLoadedLocked() error {
	if s.tts != nil {
		return nil
	}

	modelPath := filepath.Join(s.modelDir, "model.onnx")
	voicesPath := filepath.Join(s.modelDir, "voices.bin")
	tokensPath := filepath.Join(s.modelDir, "tokens.txt")
	dataDir := filepath.Join(s.modelDir, "espeak-ng-data")

	if _, err := os.Stat(modelPath); err != nil {
		return ErrModelNotLoaded
	}
	if _, err := os.Stat(voicesPath); err != nil {
		return ErrModelNotLoaded
	}

	config := sherpa.OfflineTtsConfig{}
	config.Model.Kokoro.Model = modelPath
	config.Model.Kokoro.Voices = voicesPath
	config.Model.Kokoro.Tokens = tokensPath
	config.Model.Kokoro.DataDir = dataDir
	config.Model.Kokoro.LengthScale = 1.0

	tts := sherpa.NewOfflineTts(&config)
	if tts == nil {
		return fmt.Errorf("failed to initialize sherpa-onnx OfflineTts")
	}

	s.tts = tts
	return nil
}

func (s *SherpaTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureLoadedLocked(); err != nil {
		return nil, err
	}

	sid := 0
	speed := float32(1.0)
	if voice != nil {
		sid = ResolveKokoroSpeakerID(voice.VoiceID)
		if voice.SpeechRate > 0 {
			speed = float32(voice.SpeechRate)
		}
	}

	start := time.Now()
	audio := s.tts.Generate(text, sid, speed)
	if audio == nil || len(audio.Samples) == 0 {
		return nil, fmt.Errorf("sherpa-onnx audio generation returned empty audio")
	}

	wavBytes, err := EncodePCMFloatToWAV(audio.Samples, audio.SampleRate)
	if err != nil {
		return nil, fmt.Errorf("encode wav: %w", err)
	}

	if s.logger != nil {
		s.logger.Event("tts.synthesize", map[string]interface{}{
			"engine":       "sherpa-onnx",
			"sid":          sid,
			"samples":      len(audio.Samples),
			"sample_rate":  audio.SampleRate,
			"bytes":        len(wavBytes),
			"elapsed_ms":   time.Since(start).Milliseconds(),
		})
	}

	return wavBytes, nil
}

func (s *SherpaTTSClient) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tts != nil {
		sherpa.DeleteOfflineTts(s.tts)
		s.tts = nil
	}
}

// EncodePCMFloatToWAV serializes 32-bit float audio samples to a 16-bit mono WAV container.
func EncodePCMFloatToWAV(samples []float32, sampleRate int) ([]byte, error) {
	numChannels := uint16(1)
	bitsPerSample := uint16(16)
	byteRate := uint32(sampleRate) * uint32(numChannels) * uint32(bitsPerSample/8)
	blockAlign := numChannels * (bitsPerSample / 8)
	dataSize := uint32(len(samples) * int(bitsPerSample/8))
	chunkSize := 36 + dataSize

	var buf bytes.Buffer
	buf.Grow(int(44 + dataSize))

	// RIFF header
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, chunkSize)
	buf.WriteString("WAVE")

	// fmt subchunk
	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16)) // Subchunk1Size (16 for PCM)
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))  // AudioFormat (1 for PCM)
	_ = binary.Write(&buf, binary.LittleEndian, numChannels)
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&buf, binary.LittleEndian, byteRate)
	_ = binary.Write(&buf, binary.LittleEndian, blockAlign)
	_ = binary.Write(&buf, binary.LittleEndian, bitsPerSample)

	// data subchunk
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, dataSize)

	for _, s := range samples {
		clamped := math.Max(-1.0, math.Min(1.0, float64(s)))
		val := int16(clamped * 32767)
		_ = binary.Write(&buf, binary.LittleEndian, val)
	}

	return buf.Bytes(), nil
}
```

- [ ] **Step 5: Wire `sherpa-onnx` into `pkg/media/providers.go`**

In `pkg/media/providers.go`, update `NewTTSClient`:
```go
	case "builtin":
		switch cfg.BuiltinName {
		case "sherpa-onnx", "kokoro":
			modelDir := cfg.ModelPath
			if modelDir == "" {
				modelDir = "./cache/models/tts/kokoro"
			}
			return NewSherpaTTSClient(modelDir)
		case "native-os":
			return NewNativeOSTTSClient()
		default:
			return NewNativeOSTTSClient()
		}
```

- [ ] **Step 6: Run tests to verify `pkg/media` passes**

Run: `go test -v -count=1 ./pkg/media/`
Expected: PASS.

- [ ] **Step 7: Commit `sherpa_tts` client**

```bash
git add pkg/media/
git commit -m "feat(media): implement sherpa-onnx tts client with kokoro voice mapping"
```

---

### Task 4: Add Sherpa-ONNX Config Presets & Settings

**Files:**
- Modify: `pkg/config/types.go`
- Modify: `pkg/config/presets.go`
- Modify: `pkg/config/types_test.go`

- [ ] **Step 1: Write test for new preset in `pkg/config/presets_test.go`**

Add to `pkg/config/presets_test.go`:
```go
func TestSherpaTTSPreset(t *testing.T) {
	preset, ok := GetTTSPreset("sherpa-onnx")
	if !ok {
		t.Fatalf("expected sherpa-onnx TTS preset to exist")
	}
	if preset.Type != "builtin" || preset.BuiltinName != "sherpa-onnx" {
		t.Errorf("unexpected preset config: %+v", preset)
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

Run: `go test -v -run TestSherpaTTSPreset ./pkg/config/`
Expected: FAIL.

- [ ] **Step 3: Update `pkg/config/presets.go` and `pkg/config/types.go`**

In `pkg/config/types.go`, add `ModelPath` to `TTSConfig`:
```go
type TTSConfig struct {
	Type          string         `yaml:"type" json:"type"`
	Endpoint      string         `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	Command       string         `yaml:"command,omitempty" json:"command,omitempty"`
	Args          []string       `yaml:"args,omitempty" json:"args,omitempty"`
	BuiltinName   string         `yaml:"builtin_name,omitempty" json:"builtin_name,omitempty"`
	ModelPath     string         `yaml:"model_path,omitempty" json:"model_path,omitempty"`
	Model         string         `yaml:"model,omitempty" json:"model,omitempty"`
	DefaultVoice  string         `yaml:"default_voice,omitempty" json:"default_voice,omitempty"`
	Pitch         float64        `yaml:"pitch,omitempty" json:"pitch,omitempty"`
	SpeechRate    float64        `yaml:"speech_rate,omitempty" json:"speech_rate,omitempty"`
	AutoPlay      bool           `yaml:"auto_play" json:"auto_play"`
	MasterVolume  float64        `yaml:"master_volume" json:"master_volume"`
	VoiceProfiles []VoiceProfile `yaml:"voice_profiles,omitempty" json:"voice_profiles,omitempty"`
}
```

In `pkg/config/presets.go`, add preset:
```go
	"sherpa-onnx": {
		Type:         "builtin",
		BuiltinName:  "sherpa-onnx",
		DefaultVoice: "af_bella",
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},
```

- [ ] **Step 4: Run tests to verify config passes**

Run: `go test -v -count=1 ./pkg/config/`
Expected: PASS.

- [ ] **Step 5: Commit config changes**

```bash
git add pkg/config/
git commit -m "feat(config): add sherpa-onnx tts preset and ModelPath field"
```

---

### Task 5: Model Management API & Event Streaming Endpoints

**Files:**
- Modify: `pkg/gui/service.go`
- Modify: `pkg/gui/server.go`
- Modify: `pkg/gui/server_test.go`

- [ ] **Step 1: Write failing HTTP endpoint tests in `pkg/gui/server_test.go`**

Add to `pkg/gui/server_test.go`:
```go
func TestModelManagementEndpoints(t *testing.T) {
	svc, cleanup := setupTestService(t)
	defer cleanup()

	server := NewServer(svc)

	// GET /api/models
	req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var statuses []models.ModelStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &statuses); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(statuses) == 0 {
		t.Fatalf("expected registered models, got 0")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestModelManagementEndpoints ./pkg/gui/`
Expected: FAIL with 404 or compilation failure.

- [ ] **Step 3: Update `pkg/gui/service.go` and `pkg/gui/server.go`**

In `pkg/gui/service.go`:
1. Embed `modelsManager *models.Manager` in `Service`.
2. Initialize it in `NewService` with `paths.CacheDir()`.
3. Add methods:
```go
func (s *Service) GetModelsStatus() []models.ModelStatus {
	return s.modelsManager.ListStatuses()
}

func (s *Service) DownloadModel(ctx context.Context, id string) error {
	_, err := s.modelsManager.Download(ctx, id)
	return err
}

func (s *Service) SubscribeModelEvents() chan models.ModelStatus {
	return s.modelsManager.Subscribe()
}

func (s *Service) UnsubscribeModelEvents(ch chan models.ModelStatus) {
	s.modelsManager.Unsubscribe(ch)
}
```

In `pkg/gui/server.go`:
Map `/api/models` routes:
```go
	mux.HandleFunc("GET /api/models", s.handleGetModels)
	mux.HandleFunc("POST /api/models/{id}/download", s.handleDownloadModel)
	mux.HandleFunc("GET /api/models/events", s.handleModelEvents)
```

Implement handlers in `server.go`:
```go
func (s *Server) handleGetModels(w http.ResponseWriter, r *http.Request) {
	statuses := s.svc.GetModelsStatus()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(statuses)
}

func (s *Server) handleDownloadModel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing model id", http.StatusBadRequest)
		return
	}
	if err := s.svc.DownloadModel(r.Context(), id); err != nil {
		if errors.Is(err, models.ErrDownloadActive) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "downloading"})
}

func (s *Server) handleModelEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.svc.SubscribeModelEvents()
	defer s.svc.UnsubscribeModelEvents(ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case status, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(status)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
```

- [ ] **Step 4: Run tests to verify API endpoints pass**

Run: `go test -v -count=1 ./pkg/gui/`
Expected: PASS.

- [ ] **Step 5: Commit API endpoints**

```bash
git add pkg/gui/
git commit -m "feat(gui): add /api/models status, download, and SSE event streaming endpoints"
```

---

### Task 6: Turn Stream `model_missing` Event & Provider Guard

**Files:**
- Modify: `pkg/gui/service.go`

- [ ] **Step 1: Write test for missing model detection in turn stream**

Add a test in `pkg/gui/service_test.go` verifying that playing a turn with built-in TTS enabled but Kokoro missing emits a `model_missing` event chunk and finishes normally.

- [ ] **Step 2: Update `PlayTurnStream` in `pkg/gui/service.go`**

In `PlayTurnStream`:
Check if TTS is configured as `builtin` (`sherpa-onnx` or `kokoro`):
```go
	cfg, _ := s.configMgr.Get()
	if cfg.Media.TTS.Type == "builtin" && (cfg.Media.TTS.BuiltinName == "sherpa-onnx" || cfg.Media.TTS.BuiltinName == "kokoro") {
		status := s.modelsManager.Status("kokoro-tts")
		if !status.Installed {
			out <- TurnStreamChunk{
				Type: "model_missing",
				Payload: map[string]interface{}{
					"model_id":   "kokoro-tts",
					"name":       "Kokoro Voice Pack",
					"size_bytes": status.TotalBytes,
				},
			}
		}
	}
```

- [ ] **Step 3: Run backend tests to verify**

Run: `go test -v -count=1 ./pkg/gui/`
Expected: PASS.

- [ ] **Step 4: Commit turn stream update**

```bash
git add pkg/gui/service.go
git commit -m "feat(gui): emit model_missing turn event when builtin tts model is absent"
```

---

### Task 7: Frontend Types, API Client, and Model Download Modal

**Files:**
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/api/client.ts`
- Create: `frontend/src/components/turn/ModelDownloadModal.tsx`
- Modify: `frontend/src/components/turn/ActionConsole.tsx`

- [ ] **Step 1: Update `frontend/src/types.ts`**

Add `ModelStatus` interface and update `TurnStreamChunk`:
```typescript
export interface ModelStatus {
  id: string;
  name: string;
  installed: boolean;
  downloading: boolean;
  progress: number;
  bytes_downloaded: number;
  total_bytes: number;
  error?: string;
}

export type TurnStreamChunk =
  | { type: 'token'; text: string }
  | { type: 'narration'; text: string }
  | { type: 'error'; message: string }
  | { type: 'done'; turn_number: number }
  | { type: 'model_missing'; payload: { model_id: string; name: string; size_bytes: number } };
```

- [ ] **Step 2: Add API methods to `frontend/src/api/client.ts`**

```typescript
  async getModels(): Promise<ModelStatus[]> {
    const res = await fetch(`${this.baseUrl}/api/models`);
    if (!res.ok) throw new Error(`Failed to fetch models: ${res.statusText}`);
    return res.json();
  },

  async downloadModel(id: string): Promise<void> {
    const res = await fetch(`${this.baseUrl}/api/models/${id}/download`, {
      method: 'POST',
    });
    if (!res.ok) throw new Error(`Failed to trigger download: ${res.statusText}`);
  },

  subscribeModelEvents(onEvent: (status: ModelStatus) => void): () => void {
    const eventSource = new EventSource(`${this.baseUrl}/api/models/events`);
    eventSource.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data) as ModelStatus;
        onEvent(data);
      } catch (err) {
        console.error('Failed to parse model event:', err);
      }
    };
    return () => eventSource.close();
  },
```

- [ ] **Step 3: Create `frontend/src/components/turn/ModelDownloadModal.tsx`**

Create `frontend/src/components/turn/ModelDownloadModal.tsx`:
```tsx
import React, { useEffect, useState } from 'react';
import { Volume2, Download, AlertCircle, CheckCircle2, X } from 'lucide-react';
import { apiClient } from '../../api/client';
import { ModelStatus } from '../../types';

interface ModelDownloadModalProps {
  modelId: string;
  modelName: string;
  sizeBytes: number;
  onClose: () => void;
}

export const ModelDownloadModal: React.FC<ModelDownloadModalProps> = ({
  modelId,
  modelName,
  sizeBytes,
  onClose,
}) => {
  const [downloading, setDownloading] = useState(false);
  const [progress, setProgress] = useState(0);
  const [downloadedBytes, setDownloadedBytes] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [completed, setCompleted] = useState(false);

  const formattedSize = (sizeBytes / (1024 * 1024)).toFixed(1);

  useEffect(() => {
    const unsubscribe = apiClient.subscribeModelEvents((status: ModelStatus) => {
      if (status.id === modelId) {
        setDownloading(status.downloading);
        setProgress(status.progress);
        setDownloadedBytes(status.bytes_downloaded);
        if (status.error) {
          setError(status.error);
        }
        if (status.installed) {
          setCompleted(true);
        }
      }
    });
    return () => unsubscribe();
  }, [modelId]);

  const handleStartDownload = async () => {
    try {
      setError(null);
      setDownloading(true);
      await apiClient.downloadModel(modelId);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
      setDownloading(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-xs p-4">
      <div className="w-full max-w-md bg-stone-900 border border-stone-800 rounded-xl shadow-2xl p-6 text-stone-200">
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center gap-3">
            <div className="p-2 bg-amber-500/10 text-amber-400 rounded-lg">
              <Volume2 className="w-6 h-6" />
            </div>
            <h3 className="text-lg font-semibold text-stone-100">Enable Voice Narration</h3>
          </div>
          {!downloading && (
            <button
              onClick={onClose}
              className="text-stone-400 hover:text-stone-200 p-1 transition"
            >
              <X className="w-5 h-5" />
            </button>
          )}
        </div>

        {completed ? (
          <div className="space-y-4">
            <div className="flex items-center gap-3 text-emerald-400 bg-emerald-950/30 border border-emerald-800/40 p-3 rounded-lg">
              <CheckCircle2 className="w-5 h-5 shrink-0" />
              <p className="text-sm">Voice pack installed! Future turns will narrate automatically.</p>
            </div>
            <button
              onClick={onClose}
              className="w-full py-2.5 bg-stone-800 hover:bg-stone-700 text-stone-200 font-medium rounded-lg transition"
            >
              Done
            </button>
          </div>
        ) : (
          <div className="space-y-4">
            <p className="text-sm text-stone-400 leading-relaxed">
              High-quality character dialogue and narrative voice require the{' '}
              <span className="text-stone-200 font-medium">{modelName}</span> (~{formattedSize} MB).
              Would you like to download it now?
            </p>

            {downloading && (
              <div className="space-y-2">
                <div className="flex justify-between text-xs text-stone-400 font-mono">
                  <span>Downloading...</span>
                  <span>{Math.round(progress * 100)}%</span>
                </div>
                <div className="w-full h-2 bg-stone-800 rounded-full overflow-hidden">
                  <div
                    className="h-full bg-amber-500 transition-all duration-200"
                    style={{ width: `${Math.round(progress * 100)}%` }}
                  />
                </div>
                <p className="text-xs text-stone-500 font-mono">
                  {(downloadedBytes / (1024 * 1024)).toFixed(1)} / {formattedSize} MB
                </p>
              </div>
            )}

            {error && (
              <div className="flex items-center gap-2 text-rose-400 text-xs bg-rose-950/30 p-2.5 rounded-lg border border-rose-800/40">
                <AlertCircle className="w-4 h-4 shrink-0" />
                <span>{error}</span>
              </div>
            )}

            <div className="flex items-center gap-3 pt-2">
              <button
                disabled={downloading}
                onClick={onClose}
                className="flex-1 py-2 text-sm text-stone-400 hover:text-stone-200 transition disabled:opacity-50"
              >
                Continue in Silence
              </button>
              <button
                disabled={downloading}
                onClick={handleStartDownload}
                className="flex-1 py-2 px-4 bg-amber-600 hover:bg-amber-500 text-stone-900 font-semibold text-sm rounded-lg flex items-center justify-center gap-2 transition shadow-md disabled:opacity-50"
              >
                <Download className="w-4 h-4" />
                {downloading ? 'Downloading...' : `Download (${formattedSize} MB)`}
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
```

- [ ] **Step 4: Wire Modal into `frontend/src/components/turn/ActionConsole.tsx`**

When turn stream receives `model_missing`, set state:
```tsx
const [missingModel, setMissingModel] = useState<{ id: string; name: string; size: number } | null>(null);
const [dismissedThisSession, setDismissedThisSession] = useState(false);
```
If chunk is `model_missing` and `!dismissedThisSession`, set `missingModel`.
Render `<ModelDownloadModal>` conditionally.

- [ ] **Step 5: Run frontend type checks**

Run: `mise run test:frontend`
Expected: Clean exit with code 0 (`tsc --noEmit` passes).

- [ ] **Step 6: Commit frontend components**

```bash
git add frontend/
git commit -m "feat(frontend): add model download modal and stream event handling"
```

---

### Task 8: End-to-End Verification & Lint Gates

**Files:**
- Verification only

- [ ] **Step 1: Run backend tests**

Run: `mise run test:backend`
Expected: All Go unit and integration tests pass.

- [ ] **Step 2: Run frontend type check**

Run: `mise run test:frontend`
Expected: `tsc --noEmit` passes with 0 errors.

- [ ] **Step 3: Run backend linter**

Run: `mise run lint`
Expected: `go vet ./...` clean with 0 warnings.

- [ ] **Step 4: Build entire binary (frontend + backend)**

Run: `mise run build`
Expected: Build succeeds and outputs `bin/localrpg`.

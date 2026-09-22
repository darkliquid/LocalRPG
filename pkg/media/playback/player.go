// Package playback plays narration and speech through the application's own
// audio device rather than through the browser.
//
// A browser refuses to start audio without a user gesture, so a web client can
// never narrate a turn automatically. The application runs on the same machine
// as the player, so it owns the device and is free of that restriction.
package playback

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/darkliquid/mago"
	"github.com/darkliquid/mago/audio"
	"github.com/hajimehoshi/go-mp3"
)

// ErrUnavailable means this process has no usable audio device, so a caller
// should fall back to client-side playback.
var ErrUnavailable = errors.New("audio playback is unavailable")

// ErrUnsupportedFormat means a clip is in a container this player cannot decode.
var ErrUnsupportedFormat = errors.New("unsupported audio format")

// pollInterval is how often playback checks whether the current clip has ended.
// The mago audio package mixes on a device callback and reports position, so
// completion is observed by polling that position.
const pollInterval = 50 * time.Millisecond

// Player owns the process-wide audio device and plays one clip queue at a time.
// A new queue replaces the current one, which is what makes a second turn or a
// manual replay interrupt rather than overlap.
type Player struct {
	mu       sync.Mutex
	engine   *audio.Engine
	volume   float64
	playing  bool
	cancel   chan struct{}
	done     chan struct{}
	closeErr error
}

// Open starts the application's audio device. It returns ErrUnavailable when the
// host has no audio backend, which is expected on a headless server.
func Open(volume float64) (*Player, error) {
	return openWith(volume, nil)
}

// OpenWithBackends starts the player against an explicit backend list. Tests use
// the null backend so the queue logic runs without audio hardware.
func OpenWithBackends(volume float64, backends []mago.Backend) (*Player, error) {
	if len(backends) == 0 {
		return nil, ErrUnavailable
	}
	return openWith(volume, backends)
}

func openWith(volume float64, backends []mago.Backend) (*Player, error) {
	config := audio.DefaultConfig()
	if len(backends) > 0 {
		config.Backends = backends
	}
	if volume <= 0 {
		volume = 1
	}

	engine, err := audio.Open(config)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return &Player{engine: engine, volume: volume}, nil
}

// Available reports whether a device is open.
func (p *Player) Available() bool {
	return p != nil && p.engine != nil
}

// Playing reports whether a queue is currently running.
func (p *Player) Playing() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.playing
}

// SetVolume sets the gain applied to every clip in the queue.
func (p *Player) SetVolume(volume float64) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if volume <= 0 {
		volume = 1
	}
	p.volume = volume
}

// PlayClips replaces the current queue with the given clips, which are decoded
// and played in order. It returns as soon as playback has started; stop waits for
// the queue to finish. A clip that cannot be decoded is skipped rather than
// silencing the rest of the turn.
func (p *Player) PlayClips(clips [][]byte) error {
	if !p.Available() {
		return ErrUnavailable
	}

	p.Stop()

	p.mu.Lock()
	p.cancel = make(chan struct{})
	p.done = make(chan struct{})
	cancel := p.cancel
	done := p.done
	p.playing = true
	volume := p.volume
	p.mu.Unlock()

	go func() {
		defer func() {
			p.mu.Lock()
			p.playing = false
			p.mu.Unlock()
			close(done)
		}()
		p.run(clips, volume, cancel)
	}()

	return nil
}

// PlayFiles plays clips read from disk, in order.
func (p *Player) PlayFiles(paths []string) error {
	clips := make([][]byte, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read clip %q: %w", path, err)
		}
		clips = append(clips, data)
	}
	return p.PlayClips(clips)
}

func (p *Player) run(clips [][]byte, volume float64, cancel chan struct{}) {
	for _, data := range clips {
		if isCancelled(cancel) {
			return
		}

		wav, err := ToWAV(data)
		if err != nil {
			continue
		}

		clip, err := p.engine.Load(bytes.NewReader(wav))
		if err != nil {
			continue
		}

		stream, err := p.engine.Play(clip, audio.PlayOptions{Volume: volume, Speed: 1})
		if err != nil {
			clip.Release()
			continue
		}

		p.await(stream, clip, cancel)

		stream.Close()
		clip.Release()
	}
}

// await blocks until the clip has played out or the queue is cancelled.
func (p *Player) await(stream *audio.Stream, clip *audio.Clip, cancel chan struct{}) {
	deadline := clip.Duration() + time.Second
	started := time.Now()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cancel:
			return
		case <-ticker.C:
			if stream.Position() >= clip.Duration() {
				return
			}
			// A stream that never advances must not hold the queue forever.
			if time.Since(started) > deadline {
				return
			}
		}
	}
}

// Stop cancels the current queue and waits briefly for it to unwind.
func (p *Player) Stop() {
	if !p.Available() {
		return
	}

	p.mu.Lock()
	cancel := p.cancel
	done := p.done
	playing := p.playing
	p.cancel = nil
	p.done = nil
	p.mu.Unlock()

	if !playing || cancel == nil {
		return
	}

	close(cancel)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

// Close stops playback and releases the device.
func (p *Player) Close() error {
	if !p.Available() {
		return nil
	}
	p.Stop()

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closeErr != nil {
		return p.closeErr
	}
	err := p.engine.Close()
	p.engine = nil
	p.closeErr = err
	return err
}

func isCancelled(cancel chan struct{}) bool {
	select {
	case <-cancel:
		return true
	default:
		return false
	}
}

// ToWAV converts a clip to the only container the mixer decodes. WAV passes
// through untouched; MP3 is decoded to 16-bit stereo PCM and wrapped in a WAV
// header, because providers return whatever their engine produces and the cache
// already holds MP3 from before this player existed.
func ToWAV(data []byte) ([]byte, error) {
	switch {
	case len(data) >= 4 && bytes.HasPrefix(data, []byte("RIFF")):
		return data, nil
	case isMP3(data):
		return mp3ToWAV(data)
	default:
		return nil, ErrUnsupportedFormat
	}
}

func isMP3(data []byte) bool {
	if bytes.HasPrefix(data, []byte("ID3")) {
		return true
	}
	return len(data) > 1 && data[0] == 0xFF && data[1]&0xE0 == 0xE0
}

func mp3ToWAV(data []byte) ([]byte, error) {
	decoder, err := mp3.NewDecoder(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode mp3: %w", err)
	}

	pcm, err := io.ReadAll(decoder)
	if err != nil {
		return nil, fmt.Errorf("read mp3 pcm: %w", err)
	}

	// go-mp3 always yields signed 16-bit little-endian stereo at the source rate.
	return wrapPCMAsWAV(pcm, 2, decoder.SampleRate(), 16), nil
}

func wrapPCMAsWAV(pcm []byte, channels, sampleRate, bitsPerSample int) []byte {
	blockAlign := channels * bitsPerSample / 8
	byteRate := sampleRate * blockAlign

	var buf bytes.Buffer
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36+len(pcm)))
	buf.WriteString("WAVE")

	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(blockAlign))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(bitsPerSample))

	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(pcm)))
	buf.Write(pcm)

	return buf.Bytes()
}

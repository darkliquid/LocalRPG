// Package playback plays narration and speech through the application's own
// audio device rather than through the browser.
//
// A browser refuses to start audio without a user gesture, so a web client can
// never narrate a turn automatically. The application runs on the same machine
// as the player, so it owns the device and is free of that restriction.
//
// Clips are decoded on demand by the audio device's callback and never
// materialised twice: the cache keeps its small MP3s, and mp3 or wav is decoded
// straight into the output buffer as it is consumed.
package playback

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"unsafe"

	"github.com/darkliquid/localrpg/pkg/trace"
	"github.com/darkliquid/mago"
	"github.com/gopxl/beep"
	beepmp3 "github.com/gopxl/beep/mp3"
	beepwav "github.com/gopxl/beep/wav"
)

// ErrUnavailable means this process has no usable audio device, so a caller
// should fall back to client-side playback.
var ErrUnavailable = errors.New("audio playback is unavailable")

// ErrUnsupportedFormat means a clip is in a container this player cannot decode.
var ErrUnsupportedFormat = errors.New("unsupported audio format")

const (
	// deviceChannels and deviceSampleRate are what the mixer is opened at. Sources
	// at any rate or channel count are resampled and folded into it on the fly.
	deviceChannels   = 2
	deviceSampleRate = 48000
	// periodFrames is the callback block size. Small enough to stay responsive,
	// large enough that the decoder is not woken excessively.
	periodFrames = 512
	// resampleQuality trades CPU for fidelity; 4 is beep's "on-the-fly, good".
	resampleQuality = 4
)

// Player owns the process-wide audio device and plays one clip queue at a time.
// A new queue replaces the current one, so a second turn or a manual replay
// interrupts rather than overlaps.
type Player struct {
	lib    *mago.Library
	ctx    *mago.Context
	device *mago.Device

	// mu guards the queue. The audio thread takes it only to read the current
	// streamer and gain, so swapping a queue never blocks a callback for long.
	mu       sync.Mutex
	streamer beep.Streamer
	closers  []io.Closer
	playing  bool
	gain     float64
	scratch  [][2]float64
	closeErr error
	logger   trace.Logger
	// generation identifies the current queue. Streamer values hold functions
	// and are not comparable, so a queue is identified by a counter instead.
	generation uint64
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
	if volume <= 0 {
		volume = 1
	}

	lib, err := mago.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}

	player := &Player{lib: lib, gain: volume}

	config := mago.DefaultPlaybackDeviceConfig()
	config.Format = mago.FormatF32
	config.Channels = deviceChannels
	config.SampleRate = deviceSampleRate
	config.PeriodSizeInFrames = periodFrames
	config.DataCallback = player.onData

	var ctx *mago.Context
	if len(backends) > 0 {
		ctx, err = lib.NewContext(backends...)
		if err != nil {
			_ = lib.Close()
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		player.ctx = ctx
	}

	device, err := lib.NewPlaybackDevice(ctx, config)
	if err != nil {
		_ = player.closeResources()
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	player.device = device

	if err := device.Start(); err != nil {
		_ = player.closeResources()
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}

	return player, nil
}

// SetLogger attaches a trace sink. A nil logger records nothing.
func (p *Player) SetLogger(logger trace.Logger) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.logger = trace.OrNil(logger)
}

// Available reports whether a device is open.
func (p *Player) Available() bool {
	return p != nil && p.device != nil
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
	p.gain = volume
}

// PlayFiles replaces the current queue with the given clips, which are decoded
// as they are consumed. It returns once playback has started. A clip that cannot
// be decoded is skipped rather than silencing the rest of the turn.
func (p *Player) PlayFiles(paths []string) error {
	if !p.Available() {
		return ErrUnavailable
	}

	streamers := make([]beep.Streamer, 0, len(paths))
	closers := make([]io.Closer, 0, len(paths))
	for _, path := range paths {
		streamer, closer, err := decodeFile(path)
		if err != nil {
			continue
		}
		streamers = append(streamers, streamer)
		closers = append(closers, closer)
	}

	if len(streamers) == 0 {
		return ErrUnsupportedFormat
	}

	var queue beep.Streamer
	if len(streamers) == 1 {
		queue = streamers[0]
	} else {
		queue = beep.Seq(streamers...)
	}

	p.mu.Lock()
	previous := p.closers
	p.generation++
	p.streamer = queue
	p.closers = closers
	p.playing = true
	logger := trace.OrNil(p.logger)
	gain := p.gain
	p.mu.Unlock()

	logger.Event("audio.play", map[string]interface{}{
		"clips":  len(streamers),
		"volume": gain,
	})

	go closeAll(previous)
	return nil
}

// Stop ends the current queue. The device keeps running and emits silence, which
// keeps a replay free of the device-open latency.
func (p *Player) Stop() {
	if !p.Available() {
		return
	}

	p.mu.Lock()
	closers := p.closers
	p.generation++
	p.closers = nil
	p.streamer = nil
	p.playing = false
	p.mu.Unlock()

	go closeAll(closers)
}

// Close stops playback and releases the device.
func (p *Player) Close() error {
	if p == nil {
		return nil
	}
	p.Stop()

	p.mu.Lock()
	if p.closeErr != nil {
		err := p.closeErr
		p.mu.Unlock()
		return err
	}
	p.closeErr = errors.New("closed")
	p.mu.Unlock()

	err := p.closeResources()
	if err == nil {
		err = nil
	}
	return err
}

func (p *Player) closeResources() error {
	var firstErr error
	if p.device != nil {
		if err := p.device.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		p.device = nil
	}
	if p.ctx != nil {
		if err := p.ctx.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		p.ctx = nil
	}
	if p.lib != nil {
		if err := p.lib.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		p.lib = nil
	}
	return firstErr
}

// onData is the audio thread. It pulls exactly one block from the current
// streamer, applies gain, and reports completion, which is what advances a queue
// without any buffering of its own.
func (p *Player) onData(device *mago.Device, output unsafe.Pointer, input unsafe.Pointer, frameCount uint32) {
	frames := int(frameCount)
	samples := unsafe.Slice((*float32)(output), frames*deviceChannels)

	p.mu.Lock()
	streamer := p.streamer
	gain := p.gain
	playing := p.playing
	generation := p.generation
	if !playing || streamer == nil {
		p.mu.Unlock()
		for i := range samples {
			samples[i] = 0
		}
		return
	}
	if cap(p.scratch) < frames {
		p.scratch = make([][2]float64, frames)
	}
	buf := p.scratch[:frames]
	p.mu.Unlock()

	n, ok := streamer.Stream(buf)

	for i := 0; i < n; i++ {
		samples[i*2] = float32(buf[i][0] * gain)
		samples[i*2+1] = float32(buf[i][1] * gain)
	}
	for i := n * deviceChannels; i < len(samples); i++ {
		samples[i] = 0
	}

	if !ok {
		p.mu.Lock()
		var closers []io.Closer
		if p.generation == generation {
			p.playing = false
			p.streamer = nil
			closers = p.closers
			p.closers = nil
		}
		p.mu.Unlock()

		// Closing files must not happen on the mixing thread.
		go closeAll(closers)
	}
}

// decodeFile opens a clip and returns a streamer that decodes it lazily. The
// returned closer owns the file and must outlive the streamer.
func decodeFile(path string) (beep.Streamer, io.Closer, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}

	format, isMP3, err := sniff(file)
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}

	var decoded beep.StreamSeekCloser
	if isMP3 {
		decoded, format, err = beepmp3.Decode(file)
	} else {
		decoded, format, err = beepwav.Decode(file)
	}
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}

	var streamer beep.Streamer = decoded
	if format.SampleRate != deviceSampleRate {
		streamer = beep.Resample(resampleQuality, format.SampleRate, deviceSampleRate, decoded)
	}
	return streamer, file, nil
}

// sniff identifies the container and rewinds the file for the decoder.
func sniff(file *os.File) (beep.Format, bool, error) {
	header := make([]byte, 4)
	n, err := io.ReadFull(file, header)
	if err != nil && n == 0 {
		return beep.Format{}, false, fmt.Errorf("read clip header: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return beep.Format{}, false, fmt.Errorf("rewind clip: %w", err)
	}

	head := header[:n]
	switch {
	case len(head) >= 4 && string(head) == "RIFF":
		return beep.Format{}, false, nil
	case len(head) >= 3 && string(head[:3]) == "ID3":
		return beep.Format{}, true, nil
	case len(head) >= 2 && head[0] == 0xFF && head[1]&0xE0 == 0xE0:
		return beep.Format{}, true, nil
	default:
		return beep.Format{}, false, ErrUnsupportedFormat
	}
}

func closeAll(closers []io.Closer) {
	for _, closer := range closers {
		if closer != nil {
			_ = closer.Close()
		}
	}
}

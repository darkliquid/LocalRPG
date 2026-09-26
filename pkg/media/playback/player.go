// Package playback plays narration and speech through the application's own
// audio device rather than through the browser.
//
// A browser refuses to start audio without a user gesture, so a web client can
// never narrate a turn automatically. The application runs on the same machine
// as the player, so it owns the device and is free of that restriction.
//
// Output goes through shirei's mono mixer, whose ALSA backend is pure Go. Clips
// are decoded to 48 kHz mono float32 and streamed into the mixer as they are
// consumed.
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

// ErrUnavailable means this process has no usable audio device, so a caller
// should fall back to client-side playback.
var ErrUnavailable = errors.New("audio playback is unavailable")

// ErrUnsupportedFormat means a clip is in a container this player cannot decode.
var ErrUnsupportedFormat = errors.New("unsupported audio format")

// deviceSampleRate matches the rate Opus decoding always yields.
const deviceSampleRate = opus.SampleRate

// startDevice is the platform audio boundary. Tests replace it with a draining
// sink so no sound card is required.
var startDevice = func(rate int, fill func([]float32)) error {
	return app.StartAudio(rate, app.AudioFillFn(fill))
}

// Player owns the process-wide audio device and plays one clip queue at a time.
// A new queue replaces the current one, so a second turn or a manual replay
// interrupts rather than overlaps.
type Player struct {
	mu         sync.Mutex
	mixer      *audio.Mixer
	voice      *clipVoice
	gain       float64
	playing    bool
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

// Open starts the application's audio device. It returns ErrUnavailable when the
// host has no audio backend, which is expected on a headless server.
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

// SetLogger attaches a trace sink. A nil logger records nothing.
func (p *Player) SetLogger(logger trace.Logger) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.logger = trace.OrNil(logger)
}

// Available reports whether a device is open. The device lives for the process,
// so unlike the previous backend a player cannot be re-opened after Close.
func (p *Player) Available() bool {
	return p != nil && p.mixer != nil && !p.closed
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
	if volume <= 0 {
		volume = 1
	}
	p.mu.Lock()
	p.gain = volume
	mixer := p.mixer
	p.mu.Unlock()
	if mixer != nil {
		mixer.SetVolume(float32(volume))
	}
}

// PlayFiles replaces the current queue with the given clips, which are decoded
// as they are consumed. It returns once playback has started. A clip that cannot
// be decoded is skipped rather than silencing the rest of the turn.
func (p *Player) PlayFiles(paths []string) error {
	if p == nil {
		return ErrUnavailable
	}

	p.mu.Lock()
	unavailable := p.closed || p.mixer == nil
	p.mu.Unlock()
	if unavailable {
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
	logger := trace.OrNil(p.logger)
	gain := p.gain
	mixer := p.mixer
	p.mu.Unlock()

	if previous != nil {
		previous.stream.Release()
	}

	logger.Event("audio.play", map[string]any{
		"clips":  len(paths),
		"volume": gain,
	})

	mixer.Add(voice)

	go func() {
		defer func() { _ = voice.stream.Close() }()
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

// Stop silences the current queue.
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
		for _, sample := range pcm {
			out = append(out, float32(sample)/32768.0)
		}
	}
	if len(out) == 0 {
		return nil, ErrUnsupportedFormat
	}
	return out, nil
}

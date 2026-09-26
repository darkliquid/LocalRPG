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
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/media/opus"
	"github.com/darkliquid/localrpg/pkg/trace"
	"github.com/ebitengine/oto/v3"
	"github.com/gopxl/beep"
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
	resampleQuality  = 4
	bufferDuration   = 100 * time.Millisecond
)

// Player owns the process-wide audio device and plays one clip queue at a time.
// A new queue replaces the current one, so a second turn or a manual replay
// interrupts rather than overlaps.
type Player struct {
	otoCtx   *oto.Context
	otoReady chan struct{}

	mu         sync.Mutex
	otoPlayer  *oto.Player
	streamer   beep.Streamer
	closers    []io.Closer
	gain       float64
	playing    bool
	logger     trace.Logger
	generation uint64
	closed     bool
}

// Open starts the application's audio device. It returns ErrUnavailable when the
// host has no audio backend, which is expected on a headless server.
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
		// Context took too long to become ready
	}

	return p, nil
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
	return p != nil && p.otoCtx != nil && !p.closed
}

// Playing reports whether a queue is currently running.
func (p *Player) Playing() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.playing && p.otoPlayer != nil
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
	if p.otoPlayer != nil {
		p.otoPlayer.SetVolume(p.gain)
	}
}

// PlayFiles replaces the current queue with the given clips, which are decoded
// up front. It returns once playback has started. A clip that cannot be decoded
// is skipped rather than silencing the rest of the turn.
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

	return p.playStreamer(queue, closers, len(streamers))
}

// PlayQueue starts playback and pulls clip paths from clips as each previous
// clip drains, so the first completed clip is heard while the rest are still
// synthesized. It returns once playback has started; closing the channel ends
// the queue.
func (p *Player) PlayQueue(ctx context.Context, clips <-chan string) error {
	if !p.Available() {
		return ErrUnavailable
	}
	queue := newQueueStreamer(ctx, clips)
	return p.playStreamer(queue, []io.Closer{queue}, 0)
}

// playStreamer installs a streamer as the current queue. It returns once
// playback has started. closers are released when the queue drains or is
// replaced.
func (p *Player) playStreamer(queue beep.Streamer, closers []io.Closer, clipCount int) error {
	p.mu.Lock()
	previousClosers := p.closers
	if p.otoPlayer != nil {
		_ = p.otoPlayer.Close()
		p.otoPlayer = nil
	}

	p.generation++
	gen := p.generation
	p.streamer = queue
	p.closers = closers
	p.playing = true

	reader := &streamerReader{
		streamer: queue,
	}

	otoPlayer := p.otoCtx.NewPlayer(reader)
	otoPlayer.SetVolume(p.gain)
	p.otoPlayer = otoPlayer

	logger := trace.OrNil(p.logger)
	gain := p.gain
	p.mu.Unlock()

	logger.Event("audio.play", map[string]interface{}{
		"clips":  clipCount,
		"volume": gain,
	})

	go closeAll(previousClosers)
	otoPlayer.Play()

	go func(player *oto.Player, gen uint64, closers []io.Closer, r *streamerReader) {
		for {
			time.Sleep(20 * time.Millisecond)
			p.mu.Lock()
			if p.generation != gen || p.otoPlayer != player {
				p.mu.Unlock()
				return
			}
			if !player.IsPlaying() || (r.isDrained() && player.BufferedSize() == 0) {
				p.playing = false
				p.streamer = nil
				p.closers = nil
				p.otoPlayer = nil
				p.mu.Unlock()
				_ = player.Close()
				closeAll(closers)
				return
			}
			p.mu.Unlock()
		}
	}(otoPlayer, gen, closers, reader)

	return nil
}

// Stop ends the current queue.
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
	if p.otoPlayer != nil {
		_ = p.otoPlayer.Close()
		p.otoPlayer = nil
	}
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
	defer p.mu.Unlock()
	p.closed = true
	p.otoCtx = nil
	return nil
}

type streamerReader struct {
	streamer beep.Streamer
	buf      [][2]float64
	drained  bool
	mu       sync.Mutex
}

func (sr *streamerReader) isDrained() bool {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	return sr.drained
}

func (sr *streamerReader) Read(p []byte) (int, error) {
	framesWanted := len(p) / (deviceChannels * 2) // 4 bytes per stereo 16-bit frame
	if framesWanted == 0 {
		return 0, nil
	}

	sr.mu.Lock()
	defer sr.mu.Unlock()

	if cap(sr.buf) < framesWanted {
		sr.buf = make([][2]float64, framesWanted)
	}
	buf := sr.buf[:framesWanted]

	n, ok := sr.streamer.Stream(buf)
	for i := 0; i < n; i++ {
		left := math.Max(-1.0, math.Min(1.0, buf[i][0]))
		right := math.Max(-1.0, math.Min(1.0, buf[i][1]))

		leftInt := int16(left * 32767)
		rightInt := int16(right * 32767)

		offset := i * 4
		binary.LittleEndian.PutUint16(p[offset:], uint16(leftInt))
		binary.LittleEndian.PutUint16(p[offset+2:], uint16(rightInt))
	}

	if !ok || n == 0 {
		sr.drained = true
		return n * 4, io.EOF
	}

	return n * 4, nil
}

// decodeFile reads a clip and returns a streamer over its decoded 48 kHz stereo
// samples. Every stored clip is Ogg/Opus, so there is one decode path.
func decodeFile(path string) (beep.Streamer, io.Closer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	pcm, _, _, err := opus.Decode(data)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrUnsupportedFormat, err)
	}
	return &opusStreamer{data: pcm}, nil, nil
}

// opusStreamer presents decoded mono 48 kHz PCM as the stereo frames beep expects.
type opusStreamer struct {
	data []int16
	pos  int
}

func (s *opusStreamer) Stream(samples [][2]float64) (int, bool) {
	n := 0
	for n < len(samples) && s.pos < len(s.data) {
		f := float64(s.data[s.pos]) / 32768
		samples[n] = [2]float64{f, f}
		s.pos++
		n++
	}
	return n, s.pos < len(s.data)
}

func (s *opusStreamer) Err() error { return nil }

func closeAll(closers []io.Closer) {
	for _, closer := range closers {
		if closer != nil {
			_ = closer.Close()
		}
	}
}

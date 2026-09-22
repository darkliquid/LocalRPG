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
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/trace"
	"github.com/ebitengine/oto/v3"
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
	return p.playing && p.otoPlayer != nil && p.otoPlayer.IsPlaying()
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
		onEOF: func() {
			p.mu.Lock()
			if p.generation == gen {
				p.playing = false
				activeClosers := p.closers
				p.closers = nil
				p.streamer = nil
				p.mu.Unlock()
				go closeAll(activeClosers)
				return
			}
			p.mu.Unlock()
		},
	}

	otoPlayer := p.otoCtx.NewPlayer(reader)
	otoPlayer.SetVolume(p.gain)
	p.otoPlayer = otoPlayer

	logger := trace.OrNil(p.logger)
	gain := p.gain
	p.mu.Unlock()

	logger.Event("audio.play", map[string]interface{}{
		"clips":  len(streamers),
		"volume": gain,
	})

	go closeAll(previousClosers)
	otoPlayer.Play()

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
	onEOF    func()
	buf      [][2]float64
	calledEOF bool
	mu       sync.Mutex
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
		if !sr.calledEOF && sr.onEOF != nil {
			sr.calledEOF = true
			sr.onEOF()
		}
		return n * 4, io.EOF
	}

	return n * 4, nil
}

// decodeFile opens a clip and returns a streamer that decodes it lazily. The
// returned closer owns the file and must outlive the streamer.
func decodeFile(path string) (beep.Streamer, io.Closer, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}

	_, isMP3, err := sniff(file)
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}

	var decoded beep.StreamSeekCloser
	var format beep.Format
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

package webm

import (
	"fmt"
	"io"
	"time"

	"github.com/at-wat/ebml-go/mkvcore"
	ebmlwebm "github.com/at-wat/ebml-go/webm"
)

// keyframeInterval is how often a video keyframe is forced, so a seek always
// lands on one and every cluster can begin with one.
const keyframeInterval = 5 * time.Second

// opusCodecDelay is the encoder lookahead declared for the Opus track, in
// nanoseconds: the 312-sample pre-skip at 48 kHz.
const opusCodecDelay = 6500000

// opusSeekPreRoll is the standard Opus seek pre-roll, 80 ms in nanoseconds.
const opusSeekPreRoll = 80000000

// Muxer writes VP8 video and Opus audio into one seekable WebM file. Video and
// audio are interleaved in non-decreasing timestamp order, which is what the
// container requires and what keeps either track's blocks from being dropped.
type Muxer struct {
	video ebmlwebm.BlockWriteCloser
	audio ebmlwebm.BlockWriteCloser

	track    *OpusTrack
	audioPos int
	started  bool

	lastKeyframe time.Duration
}

// NewMuxer builds the two tracks. Audio packets are written as the video catches
// up with them, so the caller only feeds video.
func NewMuxer(w io.WriteSeeker, width, height int, track *OpusTrack) (*Muxer, error) {
	if track == nil || len(track.Packets) == 0 {
		return nil, fmt.Errorf("webm: no audio to mux")
	}
	closer, ok := w.(io.WriteCloser)
	if !ok {
		return nil, fmt.Errorf("webm: writer must be an io.WriteCloser")
	}

	tracks := []ebmlwebm.TrackEntry{
		{
			TrackNumber: 1,
			TrackUID:    1,
			CodecID:     "V_VP8",
			TrackType:   1,
			Video:       &ebmlwebm.Video{PixelWidth: uint64(width), PixelHeight: uint64(height)},
		},
		{
			TrackNumber:  2,
			TrackUID:     2,
			CodecID:      "A_OPUS",
			CodecPrivate: track.Head,
			CodecDelay:   opusCodecDelay,
			SeekPreRoll:  opusSeekPreRoll,
			TrackType:    2,
			Audio:        &ebmlwebm.Audio{SamplingFrequency: 48000, Channels: track.Channels},
		},
	}

	writers, err := ebmlwebm.NewSimpleBlockWriter(closer, tracks,
		mkvcore.WithSeekHead(true),
		mkvcore.WithCues(8192),
		mkvcore.WithMaxKeyframeInterval(1, int64(keyframeInterval/time.Millisecond)),
	)
	if err != nil {
		return nil, fmt.Errorf("webm: open block writer: %w", err)
	}

	return &Muxer{video: writers[0], audio: writers[1], track: track}, nil
}

// WriteVideo adds one encoded frame. The first frame is always a keyframe, and a
// keyframe is forced every keyframeInterval so seeking stays cheap. Audio that
// belongs before this frame is written first.
func (m *Muxer) WriteVideo(data []byte, keyframe bool, t time.Duration) error {
	if err := m.flushAudio(t); err != nil {
		return err
	}

	if !m.started {
		keyframe = true
		m.started = true
	} else if t-m.lastKeyframe >= keyframeInterval {
		keyframe = true
	}
	if keyframe {
		m.lastKeyframe = t
	}

	if _, err := m.video.Write(keyframe, t.Milliseconds(), data); err != nil {
		return fmt.Errorf("webm: write video frame: %w", err)
	}
	return nil
}

// flushAudio writes every audio packet due at or before until.
func (m *Muxer) flushAudio(until time.Duration) error {
	for m.audioPos < len(m.track.Packets) && m.track.Packets[m.audioPos].Time <= until {
		packet := m.track.Packets[m.audioPos]
		if _, err := m.audio.Write(false, packet.Time.Milliseconds(), packet.Data); err != nil {
			return fmt.Errorf("webm: write audio packet: %w", err)
		}
		m.audioPos++
	}
	return nil
}

// Close flushes the trailing audio and writes the seek index. Both writers must
// be closed; the last one closes the file.
func (m *Muxer) Close() error {
	if err := m.flushAudio(time.Duration(1 << 62)); err != nil {
		return err
	}
	if err := m.video.Close(); err != nil {
		return fmt.Errorf("webm: close video track: %w", err)
	}
	if err := m.audio.Close(); err != nil {
		return fmt.Errorf("webm: close audio track: %w", err)
	}
	return nil
}

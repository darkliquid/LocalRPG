// Package webm muxes VP8 video and Opus audio into a WebM file, entirely in Go.
package webm

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/pion/opus/pkg/oggreader"

	"github.com/darkliquid/localrpg/pkg/media/opus"
)

// silenceMono is the canonical 20 ms mono Opus silence packet (RFC 6716): a
// CELT-only TOC followed by an empty frame.
var silenceMono = []byte{0xf8, 0xff, 0xfe}

// OpusPacket is one Opus packet and where it plays.
type OpusPacket struct {
	Data     []byte
	Time     time.Duration
	Duration time.Duration
}

// OpusTrack is one continuous Opus stream: the OpusHead that becomes the WebM
// track's CodecPrivate, plus every packet in presentation order.
type OpusTrack struct {
	Head     []byte
	Channels uint64
	Packets  []OpusPacket

	position time.Duration
}

// NewOpusTrack starts an empty track for a channel count. Its OpusHead is set
// immediately so a story with no clips at all still yields a decodable track.
func NewOpusTrack(channels uint64) *OpusTrack {
	if channels == 0 {
		channels = 1
	}
	return &OpusTrack{
		Channels: channels,
		Head:     defaultOpusHead(byte(channels)),
	}
}

// AppendClip demuxes one Ogg/Opus clip and appends its packets to the timeline.
// Speech is copied, never re-encoded: an Opus packet carries no timestamp of its
// own, so only the container decides when it plays.
func (t *OpusTrack) AppendClip(ogg []byte) error {
	reader, head, err := oggreader.NewWith(bytes.NewReader(ogg))
	if err != nil {
		return fmt.Errorf("webm: read clip headers: %w", err)
	}
	if uint64(head.Channels) != t.Channels {
		return fmt.Errorf("webm: clip has %d channels, track has %d", head.Channels, t.Channels)
	}

	for {
		packet, _, err := reader.ParseNextPacket()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("webm: read clip packet: %w", err)
		}
		if bytes.HasPrefix(packet, []byte("OpusHead")) || bytes.HasPrefix(packet, []byte("OpusTags")) {
			continue
		}
		duration := packetDuration(packet)
		t.Packets = append(t.Packets, OpusPacket{
			Data:     append([]byte(nil), packet...),
			Time:     t.position,
			Duration: duration,
		})
		t.position += duration
	}
}

// AppendSilence advances the timeline by d. A short gap is left to the
// container's timecodes, but a track with no packets yet is always padded, so a
// player never meets a track it cannot decode.
func (t *OpusTrack) AppendSilence(d time.Duration) error {
	if d <= 0 {
		return nil
	}
	if d < 2*time.Second && len(t.Packets) > 0 {
		t.position += d
		return nil
	}
	frame := packetDuration(silenceMono)
	if frame <= 0 {
		t.position += d
		return nil
	}
	for filled := time.Duration(0); filled < d; filled += frame {
		t.Packets = append(t.Packets, OpusPacket{
			Data:     silenceMono,
			Time:     t.position,
			Duration: frame,
		})
		t.position += frame
	}
	return nil
}

// Duration is the length of the timeline so far.
func (t *OpusTrack) Duration() time.Duration { return t.position }

// defaultOpusHead builds the RFC 7845 identification header for a channel count,
// which is the WebM track's CodecPrivate.
func defaultOpusHead(channels byte) []byte {
	buf := &bytes.Buffer{}
	buf.WriteString("OpusHead")
	buf.WriteByte(1)
	buf.WriteByte(channels)
	_ = binary.Write(buf, binary.LittleEndian, uint16(opus.PreSkip))
	_ = binary.Write(buf, binary.LittleEndian, uint32(opus.SampleRate))
	_ = binary.Write(buf, binary.LittleEndian, uint16(0)) // output gain
	buf.WriteByte(0)                                      // channel mapping family
	return buf.Bytes()
}

// packetDuration reads a packet's length from its own TOC byte (RFC 6716 §3.1):
// the config field gives the frame duration and the frame-count code gives how
// many frames it carries. No decoding.
func packetDuration(packet []byte) time.Duration {
	if len(packet) == 0 {
		return 0
	}
	toc := packet[0]
	config := (toc >> 3) & 0x1F
	frames := int(toc&0x03) + 1

	var frameMs float64
	switch {
	case config < 12:
		// SILK-only: 10, 20, 40 or 60 ms.
		frameMs = []float64{10, 20, 40, 60}[config%4]
	case config < 16:
		// Hybrid: 10 or 20 ms.
		frameMs = []float64{10, 20}[config%2]
	default:
		// CELT-only: 2.5, 5, 10 or 20 ms.
		frameMs = []float64{2.5, 5, 10, 20}[config%4]
	}
	return time.Duration(frameMs*float64(frames)*float64(time.Millisecond) + 0.5)
}

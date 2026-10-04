package opus

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/gopxl/beep"
	pionopus "github.com/pion/opus"
	"github.com/pion/opus/pkg/oggreader"
)

const (
	// SampleRate is the rate Opus encodes and decodes at.
	SampleRate = 48000
	// FrameSamples is one 20 ms frame at SampleRate.
	FrameSamples = 960
	// PreSkip is the encoder lookahead written into OpusHead and trimmed on
	// decode. pion's encoder does not expose libopus's lookahead, so the standard
	// 48 kHz value is used.
	PreSkip = 312
	// DefaultBitrate is the target used when the configuration names none.
	DefaultBitrate = 32000
	// MinBitrate and MaxBitrate bound the encoder's accepted target.
	MinBitrate = 6000
	MaxBitrate = 510000
	// maxDecodedFrame is the largest frame the decoder can emit (120 ms).
	maxDecodedFrame = 5760
)

// Encode resamples pcm (mono or interleaved s16 at sampleRate) to 48 kHz mono,
// encodes it with Opus, and returns a complete Ogg/Opus file.
func Encode(pcm []int16, sampleRate, channels, bitrate int) ([]byte, error) {
	if len(pcm) == 0 {
		return nil, errors.New("opus: no audio to encode")
	}
	if sampleRate <= 0 || sampleRate > math.MaxInt32 {
		return nil, fmt.Errorf("opus: invalid sample rate %d", sampleRate)
	}
	if bitrate < MinBitrate || bitrate > MaxBitrate {
		bitrate = DefaultBitrate
	}
	mono := toMono(pcm, channels)
	mono48 := resample48(mono, sampleRate)
	realSamples := len(mono48)

	enc, err := pionopus.NewEncoder(
		pionopus.WithSampleRate(SampleRate),
		pionopus.WithChannels(1),
		pionopus.WithBitrate(bitrate),
		pionopus.WithComplexity(5),
		pionopus.WithApplication(pionopus.ApplicationVoIP),
		pionopus.WithVBR(true),
	)
	if err != nil {
		return nil, fmt.Errorf("opus: create encoder: %w", err)
	}

	var out bytes.Buffer
	w, err := NewWriter(&out, 1, PreSkip, uint32(sampleRate), 0x4c525047) // "LRPG"
	if err != nil {
		return nil, err
	}

	frame := make([]byte, FrameSamples*2)
	packet := make([]byte, 4096)
	for off := 0; off < realSamples; off += FrameSamples {
		for i := 0; i < FrameSamples; i++ {
			var sample int16
			if off+i < realSamples {
				sample = mono48[off+i]
			}
			binary.LittleEndian.PutUint16(frame[i*2:], uint16(sample))
		}
		n, err := enc.Encode(frame, packet)
		if err != nil {
			return nil, fmt.Errorf("opus: encode frame: %w", err)
		}
		through := off + FrameSamples
		if through > realSamples {
			through = realSamples
		}
		last := off+FrameSamples >= realSamples
		if err := w.WritePacket(packet[:n], uint64(PreSkip+through), last); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Decode reads an Ogg/Opus file and returns 48 kHz mono s16 PCM with the
// pre-skip and trailing padding trimmed.
func Decode(data []byte) ([]int16, int, int, error) {
	reader, head, err := oggreader.NewWith(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("opus: read headers: %w", err)
	}
	channels := int(head.Channels)
	if channels < 1 {
		channels = 1
	}

	dec := pionopus.NewDecoder()
	if err := dec.Init(SampleRate, channels); err != nil {
		return nil, 0, 0, fmt.Errorf("opus: init decoder: %w", err)
	}

	frame := make([]int16, maxDecodedFrame*channels)
	var pcm []int16
	var lastGranule uint64
	for {
		packet, page, err := reader.ParseNextPacket()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, 0, 0, fmt.Errorf("opus: read packet: %w", err)
		}
		if bytes.HasPrefix(packet, []byte("OpusTags")) {
			continue
		}
		n, err := dec.DecodeToInt16(packet, frame)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("opus: decode packet: %w", err)
		}
		pcm = append(pcm, frame[:n*channels]...)
		lastGranule = page.GranulePosition
	}
	if len(pcm) == 0 {
		return nil, 0, 0, errors.New("opus: no audio packets")
	}

	if channels > 1 {
		pcm = toMono(pcm, channels)
	}
	skip := int(head.PreSkip)
	if skip < len(pcm) {
		pcm = pcm[skip:]
	} else {
		pcm = nil
	}
	if lastGranule > uint64(head.PreSkip) {
		if want := int(lastGranule) - int(head.PreSkip); want < len(pcm) {
			pcm = pcm[:want]
		}
	}
	return pcm, SampleRate, 1, nil
}

// toMono folds interleaved samples into one channel.
func toMono(pcm []int16, channels int) []int16 {
	if channels <= 1 {
		return pcm
	}
	mono := make([]int16, len(pcm)/channels)
	for i := range mono {
		var sum int
		for c := 0; c < channels; c++ {
			sum += int(pcm[i*channels+c])
		}
		mono[i] = int16(sum / channels)
	}
	return mono
}

// resample48 resamples mono s16 to 48 kHz using beep's resampler.
func resample48(mono []int16, sampleRate int) []int16 {
	if sampleRate == SampleRate || sampleRate <= 0 {
		return mono
	}
	src := &sliceStreamer{data: make([][2]float64, len(mono))}
	for i, v := range mono {
		f := float64(v) / 32768
		src.data[i] = [2]float64{f, f}
	}
	resampled := beep.Resample(4, beep.SampleRate(sampleRate), beep.SampleRate(SampleRate), src)

	buffer := make([][2]float64, 4096)
	out := make([]int16, 0, len(mono)*SampleRate/sampleRate)
	for {
		n, ok := resampled.Stream(buffer)
		for i := 0; i < n; i++ {
			out = append(out, floatToInt16(buffer[i][0]))
		}
		if !ok {
			break
		}
	}
	return out
}

func floatToInt16(v float64) int16 {
	switch {
	case v >= 1:
		return 32767
	case v <= -1:
		return -32768
	default:
		return int16(v * 32767)
	}
}

// sliceStreamer adapts a sample slice to beep's Streamer.
type sliceStreamer struct {
	data [][2]float64
	pos  int
}

func (s *sliceStreamer) Stream(samples [][2]float64) (int, bool) {
	n := copy(samples, s.data[s.pos:])
	s.pos += n
	return n, s.pos < len(s.data)
}

func (s *sliceStreamer) Err() error { return nil }

// Duration reads a clip's length from the final Ogg page's granule position. It
// replaces probing the file with ffprobe, which the export no longer needs: every
// clip is an Ogg/Opus stream this package wrote, so the granule is authoritative.
func Duration(data []byte) (time.Duration, error) {
	reader, head, err := oggreader.NewWith(bytes.NewReader(data))
	if err != nil {
		return 0, fmt.Errorf("opus: read headers: %w", err)
	}

	var lastGranule uint64
	for {
		_, page, err := reader.ParseNextPacket()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return 0, fmt.Errorf("opus: read packet: %w", err)
		}
		lastGranule = page.GranulePosition
	}
	if lastGranule == 0 {
		return 0, errors.New("opus: no audio packets")
	}

	skip := uint64(head.PreSkip)
	if lastGranule <= skip {
		return 0, nil
	}
	samples := lastGranule - skip
	return time.Duration(float64(samples) / float64(SampleRate) * float64(time.Second)), nil
}

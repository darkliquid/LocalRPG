package opus

import (
	"bytes"
	"math"
	"testing"

	"github.com/pion/opus/pkg/oggreader"
)

func tone(sampleRate int, seconds float64) []int16 {
	n := int(float64(sampleRate) * seconds)
	pcm := make([]int16, n)
	for i := range pcm {
		v := math.Sin(2 * math.Pi * 220 * float64(i) / float64(sampleRate))
		pcm[i] = int16(v * 12000)
	}
	return pcm
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	const rate = 24000
	source := tone(rate, 1.0)

	data, err := Encode(source, rate, 1, DefaultBitrate)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("OggS")) {
		t.Fatal("encoded audio is not an Ogg stream")
	}

	pcm, gotRate, channels, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if gotRate != SampleRate {
		t.Fatalf("rate = %d, want %d", gotRate, SampleRate)
	}
	if channels != 1 {
		t.Fatalf("channels = %d, want 1", channels)
	}
	// One second at 48 kHz, allow a frame of slack for encoder lookahead pacing.
	if diff := len(pcm) - SampleRate; diff > FrameSamples || diff < -FrameSamples {
		t.Fatalf("decoded %d samples, want about %d", len(pcm), SampleRate)
	}
}

func TestMuxerWritesAParseableOggStream(t *testing.T) {
	data, err := Encode(tone(48000, 0.5), 48000, 1, DefaultBitrate)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	reader, head, err := oggreader.NewWith(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("oggreader rejected the stream: %v", err)
	}
	if head.Channels != 1 {
		t.Fatalf("channels = %d, want 1", head.Channels)
	}
	if head.PreSkip != PreSkip {
		t.Fatalf("pre-skip = %d, want %d", head.PreSkip, PreSkip)
	}
	packets := 0
	for {
		if _, _, err := reader.ParseNextPacket(); err != nil {
			break
		}
		packets++
	}
	if packets == 0 {
		t.Fatal("expected audio packets after the headers")
	}
}

func TestEncodeRejectsEmptyAudio(t *testing.T) {
	if _, err := Encode(nil, 24000, 1, DefaultBitrate); err == nil {
		t.Fatal("expected an error for empty audio")
	}
}

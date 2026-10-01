package webm

import (
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/media/opus"
)

func clip(t *testing.T, seconds float64) []byte {
	t.Helper()
	pcm := make([]int16, int(float64(opus.SampleRate)*seconds))
	data, err := opus.Encode(pcm, opus.SampleRate, 1, opus.DefaultBitrate)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	return data
}

func TestOpusTrackLaysClipsOnOneTimeline(t *testing.T) {
	track := NewOpusTrack(1)
	if err := track.AppendClip(clip(t, 1.0)); err != nil {
		t.Fatalf("AppendClip: %v", err)
	}
	if err := track.AppendClip(clip(t, 0.5)); err != nil {
		t.Fatalf("AppendClip: %v", err)
	}
	if len(track.Head) < 8 || string(track.Head[:8]) != "OpusHead" {
		t.Fatalf("CodecPrivate is not an OpusHead: %q", track.Head)
	}
	if got := track.Duration(); got < 1400*time.Millisecond || got > 1600*time.Millisecond {
		t.Errorf("Duration = %v, want about 1.5s", got)
	}

	// Timestamps must be monotonic and start at zero.
	var last time.Duration
	for _, packet := range track.Packets {
		if packet.Time < last {
			t.Fatalf("packet time went backwards: %v after %v", packet.Time, last)
		}
		last = packet.Time
	}
	if track.Packets[0].Time != 0 {
		t.Errorf("first packet at %v, want 0", track.Packets[0].Time)
	}
}

func TestOpusTrackFillsAGapWithSilence(t *testing.T) {
	track := NewOpusTrack(1)
	if err := track.AppendClip(clip(t, 0.2)); err != nil {
		t.Fatal(err)
	}
	before := track.Duration()
	if err := track.AppendSilence(3 * time.Second); err != nil {
		t.Fatalf("AppendSilence: %v", err)
	}
	if got := track.Duration() - before; got < 2900*time.Millisecond || got > 3100*time.Millisecond {
		t.Errorf("silence added %v, want about 3s", got)
	}
	// A silence packet is the canonical mono frame.
	found := false
	for _, packet := range track.Packets {
		if len(packet.Data) == 3 && packet.Data[0] == 0xf8 {
			found = true
		}
	}
	if !found {
		t.Error("expected a canonical silence packet in the track")
	}
}

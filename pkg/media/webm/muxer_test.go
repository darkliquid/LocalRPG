package webm

import (
	"errors"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/at-wat/ebml-go"
	"github.com/at-wat/ebml-go/mkvcore"
)

// trackHeader is the part of a WebM file a test needs to inspect. Stopping at
// Tracks keeps the parse off the block stream, which needs a goroutine.
type trackHeader struct {
	Segment struct {
		Tracks struct {
			TrackEntry []mkvcore.TrackEntry
		} `ebml:"Tracks,stop"`
	}
}

func TestMuxerWritesASeekableWebM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.webm")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	track := NewOpusTrack(1)
	if err := track.AppendClip(clip(t, 0.5)); err != nil {
		t.Fatal(err)
	}

	muxer, err := NewMuxer(file, 64, 48, track)
	if err != nil {
		t.Fatalf("NewMuxer: %v", err)
	}

	encoder := NewEncoder(64, 48, 80)
	for i := 0; i < 5; i++ {
		data, err := encoder.Encode(solidFrame(64, 48, color.RGBA{byte(20 * i), 10, 10, 255}), i == 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := muxer.WriteVideo(data, i == 0, time.Duration(i)*100*time.Millisecond); err != nil {
			t.Fatalf("WriteVideo: %v", err)
		}
	}
	// The muxer owns the writer and closes it, so the file is not closed here.
	if err := muxer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	read, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()

	var header trackHeader
	if err := ebml.Unmarshal(read, &header); err != nil && !errors.Is(err, ebml.ErrReadStopped) {
		t.Fatalf("read back: %v", err)
	}

	entries := header.Segment.Tracks.TrackEntry
	if len(entries) != 2 {
		t.Fatalf("tracks = %d, want 2", len(entries))
	}
	codecs := map[string]bool{}
	for _, entry := range entries {
		codecs[entry.CodecID] = true
	}
	if !codecs["V_VP8"] || !codecs["A_OPUS"] {
		t.Fatalf("codec ids = %v", codecs)
	}
}

func TestMuxerRequiresAudio(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "out.webm"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	if _, err := NewMuxer(file, 64, 48, nil); err == nil {
		t.Fatal("expected an error for a missing audio track")
	}
}

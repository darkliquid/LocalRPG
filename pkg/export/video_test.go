package export

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/at-wat/ebml-go"
	"github.com/at-wat/ebml-go/mkvcore"
	"github.com/darkliquid/localrpg/pkg/scene"
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

func smallScript() *scene.Script {
	return &scene.Script{
		GameID:   "campaign-01",
		GameName: "Campaign One",
		Scenes: []scene.Scene{{
			LocationID:   "alden-tavern",
			LocationName: "Alden Tavern",
			Duration:     4 * time.Second,
			Beats: []scene.Beat{
				{Kind: scene.BeatSceneCard, Text: "Alden Tavern", Duration: 2 * time.Second},
				{Kind: scene.BeatNarration, Text: "Warm light.", Duration: 2 * time.Second},
			},
		}},
		TotalDuration: 4 * time.Second,
	}
}

func TestRenderVideoWritesAPlayableWebM(t *testing.T) {
	pipeline := NewVideoPipeline(".")
	pipeline.SetSize(64, 48)
	pipeline.SetFPS(5)

	out := filepath.Join(t.TempDir(), "replay.webm")
	if err := pipeline.RenderVideo(context.Background(), smallScript(), out); err != nil {
		t.Fatalf("RenderVideo: %v", err)
	}

	file, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	var header trackHeader
	if err := ebml.Unmarshal(file, &header); err != nil && !errors.Is(err, ebml.ErrReadStopped) {
		t.Fatalf("read back: %v", err)
	}
	if len(header.Segment.Tracks.TrackEntry) == 0 {
		t.Fatal("the file has no tracks")
	}
}

func TestRenderVideoLeavesNothingWhenCancelled(t *testing.T) {
	pipeline := NewVideoPipeline(".")
	pipeline.SetSize(64, 48)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	out := filepath.Join(t.TempDir(), "replay.webm")
	if err := pipeline.RenderVideo(ctx, smallScript(), out); err == nil {
		t.Fatal("expected a cancelled render to fail")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("a cancelled render left a file: %v", err)
	}
}

func TestRenderVideoReportsProgress(t *testing.T) {
	pipeline := NewVideoPipeline(".")
	pipeline.SetSize(64, 48)
	pipeline.SetFPS(5)

	var events []scene.Progress
	pipeline.SetProgress(func(p scene.Progress) { events = append(events, p) })

	out := filepath.Join(t.TempDir(), "replay.webm")
	if err := pipeline.RenderVideo(context.Background(), smallScript(), out); err != nil {
		t.Fatalf("RenderVideo: %v", err)
	}

	// The render must say it started and say when each beat finished, or a long
	// export looks like it has stalled.
	sawStart := false
	sawFinalBeat := false
	for _, event := range events {
		if event.Phase == "frames" && event.Total == 2 {
			if event.Done == 0 {
				sawStart = true
			}
			if event.Done == 2 {
				sawFinalBeat = true
			}
		}
	}
	if !sawStart {
		t.Error("expected an initial progress event")
	}
	if !sawFinalBeat {
		t.Error("expected progress to reach the last beat")
	}
	if events[len(events)-1].Phase != "done" {
		t.Errorf("last phase = %q, want done", events[len(events)-1].Phase)
	}
}

func TestRenderVideoRequiresScenes(t *testing.T) {
	out := filepath.Join(t.TempDir(), "x.webm")
	if err := NewVideoPipeline(".").RenderVideo(context.Background(), &scene.Script{}, out); err == nil {
		t.Fatal("expected an error for a script with no scenes")
	}
}

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
	if len(events) == 0 {
		t.Fatal("expected progress events")
	}

	// The first event must already carry the totals, so a bar can be exact from
	// the start rather than creeping towards an unknown end.
	first := events[0]
	if first.Phase != "frames" || first.Total == 0 || first.Done != 0 {
		t.Fatalf("first event = %+v, want an initial frames event with a total", first)
	}
	if first.ImageFrames != 0 || first.RepeatFrames != 0 {
		t.Fatalf("first event = %+v, want no frames counted yet", first)
	}

	last := events[len(events)-1]
	if last.Phase != "done" {
		t.Fatalf("last phase = %q, want done", last.Phase)
	}
	if last.Frames != first.Total {
		t.Errorf("finished %d frames, plan said %d", last.Frames, first.Total)
	}
	if last.ImageFrames+last.RepeatFrames != last.Frames {
		t.Errorf("image %d + repeat %d != frames %d", last.ImageFrames, last.RepeatFrames, last.Frames)
	}
	if last.RepeatFrames == 0 {
		t.Error("expected the still tail of each beat to be a repeat frame")
	}
}

func TestRenderVideoRequiresScenes(t *testing.T) {
	out := filepath.Join(t.TempDir(), "x.webm")
	if err := NewVideoPipeline(".").RenderVideo(context.Background(), &scene.Script{}, out); err == nil {
		t.Fatal("expected an error for a script with no scenes")
	}
}

func TestRenderPlanIncludesTheBuffers(t *testing.T) {
	pipeline := NewVideoPipeline(".")
	script := smallScript()
	track, err := pipeline.opusTrack(script)
	if err != nil {
		t.Fatalf("opusTrack: %v", err)
	}

	plain := NewRenderPlan(script, track, 5, true, 0, 0, 0)
	buffered := NewRenderPlan(script, track, 5, true, time.Second, time.Second, 500*time.Millisecond)

	beats := len(script.Beats())
	if got, want := buffered.Frames.Duration-plain.Frames.Duration, 2*time.Second+time.Duration(beats)*500*time.Millisecond; got != want {
		t.Errorf("buffers add %v, want %v", got, want)
	}
	if got, want := buffered.Frames.Total-plain.Frames.Total, beats+2; got != want {
		t.Errorf("buffers add %d frames, want %d", got, want)
	}
}

// TestOpusTrackMatchesThePicture pins the sync fix: the audio timeline must be as
// long as the picture. A beat is a clip plus a gap, so without holding the gap
// every spoken beat ends early and the whole track creeps ahead of the video.
func TestOpusTrackMatchesThePicture(t *testing.T) {
	pipeline := NewVideoPipeline(".")
	script := smallScript()
	track, err := pipeline.opusTrack(script)
	if err != nil {
		t.Fatalf("opusTrack: %v", err)
	}

	plan := NewRenderPlan(script, track, 5, true, pipeline.intro, pipeline.outro, pipeline.gap)
	if diff := plan.Length - track.Duration(); diff > 50*time.Millisecond || diff < -50*time.Millisecond {
		t.Errorf("picture runs %v but audio runs %v", plan.Length, track.Duration())
	}
}

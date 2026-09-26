package theater

import (
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/scene"
)

func testScript() *scene.Script {
	return &scene.Script{
		GameName: "Demo",
		Scenes: []scene.Scene{{
			LocationID: "hall", LocationName: "The Hall", ArtPath: "/tmp/hall.png",
			Beats: []scene.Beat{
				{Kind: scene.BeatSceneCard, Text: "The Hall", Duration: 2 * time.Second},
				{Kind: scene.BeatNarration, Text: "The door opens.", Duration: 3 * time.Second},
				{Kind: scene.BeatSpeech, Speaker: "Vance", Text: "Hello?", Duration: 2 * time.Second},
			},
		}},
	}
}

func TestBeatAtIndexesFlattenedBeats(t *testing.T) {
	s := testScript()
	sc, beat, ok := BeatAt(Frame{Script: s, SceneIdx: 0, BeatIdx: 2})
	if !ok {
		t.Fatal("expected a beat")
	}
	if beat.Speaker != "Vance" || sc.LocationName != "The Hall" {
		t.Fatalf("beat = %+v scene = %+v", beat, sc)
	}
}

func TestBeatProgressWalksFrames(t *testing.T) {
	s := testScript()
	// 2s + 3s + 2s = 7s at 15fps = 105 frames.
	f0 := BeatProgress(s, 0, 15)
	if f0.BeatIdx != 0 || f0.Progress != 0 {
		t.Fatalf("first frame = %+v", f0)
	}
	fLast := BeatProgress(s, 104, 15)
	if fLast.BeatIdx != 2 {
		t.Fatalf("last frame beat = %+v", fLast)
	}
	fMid := BeatProgress(s, 45, 15)
	if fMid.SceneIdx != 0 || fMid.Progress <= 0 || fMid.Progress >= 1 {
		t.Fatalf("mid frame = %+v", fMid)
	}
}

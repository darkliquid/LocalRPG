// Package theater renders the story theatre: the same view serves the live
// window and the offline video exporter, driven by an explicit time model.
package theater

import "github.com/darkliquid/localrpg/pkg/scene"

// Frame is one rendered moment: which beat is showing and how far through it
// playback has reached, plus the stage presentation the live window and the
// exporter share.
type Frame struct {
	Script   *scene.Script
	SceneIdx int
	BeatIdx  int
	Progress float64

	PlayerPortrait string
	NPCPortrait    string
	PlayerLabel    string
	NPCLabel       string
	PlayerActive   bool
	NPCActive      bool
}

// BeatAt returns the scene and beat a frame points at.
func BeatAt(f Frame) (scene.Scene, scene.Beat, bool) {
	if f.Script == nil || f.SceneIdx < 0 || f.SceneIdx >= len(f.Script.Scenes) {
		return scene.Scene{}, scene.Beat{}, false
	}
	sc := f.Script.Scenes[f.SceneIdx]
	if f.BeatIdx < 0 || f.BeatIdx >= len(sc.Beats) {
		return scene.Scene{}, scene.Beat{}, false
	}
	return sc, sc.Beats[f.BeatIdx], true
}

// BeatProgress maps an output frame index to the beat and progress it shows.
// Frames past the end clamp to the final beat.
func BeatProgress(script *scene.Script, frame, fps int) Frame {
	if script == nil {
		return Frame{}
	}
	position := 0
	last := Frame{Script: script}
	for si := range script.Scenes {
		for bi := range script.Scenes[si].Beats {
			beat := script.Scenes[si].Beats[bi]
			count := scene.FramesFor(beat.Duration, fps)
			if frame < position+count {
				progress := 1.0
				if count > 1 {
					progress = float64(frame-position) / float64(count-1)
				}
				return Frame{Script: script, SceneIdx: si, BeatIdx: bi, Progress: progress}
			}
			position += count
			last = Frame{Script: script, SceneIdx: si, BeatIdx: bi, Progress: 1}
		}
	}
	return last
}

// FrameCount is the total number of output frames for a script.
func FrameCount(script *scene.Script, fps int) int {
	if script == nil {
		return 0
	}
	total := 0
	for si := range script.Scenes {
		for bi := range script.Scenes[si].Beats {
			total += scene.FramesFor(script.Scenes[si].Beats[bi].Duration, fps)
		}
	}
	return total
}

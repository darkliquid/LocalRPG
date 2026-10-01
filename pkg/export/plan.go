package export

import (
	"time"

	"github.com/darkliquid/localrpg/pkg/media/webm"
	"github.com/darkliquid/localrpg/pkg/scene"
)

const (
	// introBuffer and outroBuffer are held before the first beat and after the
	// last, so a player does not open or close on a hard cut. They are part of the
	// length calculation and are zero until a renderer draws them.
	introBuffer = time.Duration(0)
	outroBuffer = time.Duration(0)
)

// RenderPlan is everything a progress bar needs before a render starts: how long
// the video is, how many frames it will produce, how many of those are new
// images, and how much audio it carries. Every total here is known up front, so a
// bar can be exact from the first frame.
type RenderPlan struct {
	// Length is the finished video's duration: every beat's span, plus the
	// intro and outro buffers.
	Length time.Duration

	// Frames counts the frames the render will produce, split into new images and
	// repeats of the frame before.
	Frames scene.FramePlan

	// AudioPackets and AudioBytes are the campaign's own clips, demuxed but not
	// re-encoded, so both are exact.
	AudioPackets int
	AudioBytes   int64
	AudioLength  time.Duration
}

// NewRenderPlan counts a script's frames and audio without rendering anything.
// The track must already be assembled, because its packet list is what makes the
// audio totals exact rather than a guess.
func NewRenderPlan(script *scene.Script, track *webm.OpusTrack, fps int, animate bool) RenderPlan {
	plan := RenderPlan{
		Frames: scene.PlanFrames(script, fps, animate),
	}
	plan.Length = plan.Frames.Duration + introBuffer + outroBuffer
	if track != nil {
		plan.AudioPackets = track.PacketCount()
		plan.AudioBytes = track.ByteCount()
		plan.AudioLength = track.Duration()
	}
	return plan
}

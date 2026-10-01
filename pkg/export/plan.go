package export

import (
	"time"

	"github.com/darkliquid/localrpg/pkg/media/webm"
	"github.com/darkliquid/localrpg/pkg/scene"
)

// RenderPlan is everything a progress bar needs before a render starts: how long
// the video is, how many frames it will produce, how many of those are new
// images, and how much audio it carries. Every total here is known up front, so a
// bar can be exact from the first frame.
type RenderPlan struct {
	// Length is the finished video's duration: every beat's span, plus the extra
	// gap after each beat and the intro and outro buffers.
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
// audio totals exact rather than a guess. The buffers are the ones the render will
// actually use, so the totals match what is drawn.
func NewRenderPlan(script *scene.Script, track *webm.OpusTrack, fps int, animate bool, intro, outro, gap time.Duration) RenderPlan {
	plan := RenderPlan{
		Frames: scene.PlanFrames(script, fps, animate),
	}

	beats := 0
	if script != nil {
		beats = len(script.Beats())
	}
	// The intro is a drawn frame; a beat's gap and the outro hold the picture
	// before them, so they repeat.
	if intro > 0 && beats > 0 {
		plan.Frames.Total++
		plan.Frames.Image++
		plan.Frames.Duration += intro
	}
	if gap > 0 {
		plan.Frames.Total += beats
		plan.Frames.Repeat += beats
		plan.Frames.Duration += gap * time.Duration(beats)
	}
	if outro > 0 && beats > 0 {
		plan.Frames.Total++
		plan.Frames.Repeat++
		plan.Frames.Duration += outro
	}

	plan.Length = plan.Frames.Duration
	if track != nil {
		plan.AudioPackets = track.PacketCount()
		plan.AudioBytes = track.ByteCount()
		plan.AudioLength = track.Duration()
	}
	return plan
}

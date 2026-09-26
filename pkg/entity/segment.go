package entity

// Segment kinds for a turn's ordered playback script.
const (
	SegmentNarration = "narration"
	SegmentSpeech    = "speech"
)

// TurnSegment is one spoken or narrated span of a turn, in playback order.
type TurnSegment struct {
	Kind      string `json:"kind"`
	Speaker   string `json:"speaker,omitempty"`
	SpeakerID string `json:"speaker_id,omitempty"`
	Text      string `json:"text"`
	// CheckRef names the CheckResult whose roll this segment narrates, so the
	// chronicle can render the dice inline rather than in a detached strip.
	CheckRef string `json:"check_ref,omitempty"`
	// Player marks the utterance as the protagonist's own line. It renders and
	// plays exactly like any other speech beat; the flag lets the chronicle skip
	// the duplicate action block that would otherwise print the same words.
	Player bool `json:"player,omitempty"`
}

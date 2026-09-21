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
}

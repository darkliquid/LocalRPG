package media

// KokoroModelV019 is the English Kokoro model pinned by pkg/models. Its speaker
// order comes from the sherpa-onnx release docs and must never be reused for a
// different model variant: v1.0 inserts am_santa at SID 19 and shifts every
// British voice by one.
const KokoroModelV019 = "kokoro-en-v0_19"

type KokoroSpeaker struct {
	Name string
	SID  int
}

var kokoroSpeakersByModel = map[string][]KokoroSpeaker{
	KokoroModelV019: {
		{"af", 0},
		{"af_bella", 1},
		{"af_nicole", 2},
		{"af_sarah", 3},
		{"af_sky", 4},
		{"am_adam", 5},
		{"am_michael", 6},
		{"bf_emma", 7},
		{"bf_isabella", 8},
		{"bm_george", 9},
		{"bm_lewis", 10},
	},
}

func KokoroSpeakersForModel(modelID string) []KokoroSpeaker {
	return kokoroSpeakersByModel[modelID]
}

// ResolveKokoroSpeakerID maps an authored voice ID to the numeric style id the
// model expects, falling back to the first speaker so an unknown voice still
// speaks rather than erroring the turn.
func ResolveKokoroSpeakerID(modelID, voiceID string) int {
	for _, s := range kokoroSpeakersByModel[modelID] {
		if s.Name == voiceID {
			return s.SID
		}
	}
	return 0
}

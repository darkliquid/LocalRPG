package media

// LiveGroupCaps clamps a capability set to what live audio can use: one speaker
// per request, because a multi-speaker request delays the first speaker's audio
// until the second speaker's text exists. The request limits are kept, so a
// single speaker's run is still split when it is too large for one request.
//
// Both the streaming fold and the turn's clip plan use it, so their groups agree
// and the streamed clips are the clips the turn records.
func LiveGroupCaps(caps TTSCapabilities) TTSCapabilities {
	caps = normalizeCaps(caps)
	caps.MaxSpeakers = 1
	return caps
}

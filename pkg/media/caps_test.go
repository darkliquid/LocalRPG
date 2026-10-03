package media

import "testing"

func TestLiveGroupCapsClampToSingleSpeaker(t *testing.T) {
	got := LiveGroupCaps(TTSCapabilities{MaxSpeakers: 3, MaxCharsPerRequest: 500, MaxTokensPerRequest: 200})
	if got.MaxSpeakers != 1 {
		t.Fatalf("MaxSpeakers = %d, want 1", got.MaxSpeakers)
	}
	if got.MaxCharsPerRequest != 500 || got.MaxTokensPerRequest != 200 {
		t.Fatalf("live caps dropped a request limit: %+v", got)
	}
}

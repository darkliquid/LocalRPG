package media

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestFilterVoiceProfiles(t *testing.T) {
	profiles := []config.VoiceProfile{
		{ID: "portable", VoiceID: "x"},
		{ID: "kokoro-one", Provider: "builtin:kokoro", VoiceID: "af_bella"},
		{ID: "eleven-one", Provider: "builtin:elevenlabs", VoiceID: "EXAVIT"},
	}

	kept := FilterVoiceProfiles(profiles, "builtin:kokoro")
	if len(kept) != 2 {
		t.Fatalf("kept %d profiles, want the portable one and the Kokoro one", len(kept))
	}
	if kept[0].ID != "portable" || kept[1].ID != "kokoro-one" {
		t.Errorf("kept = %+v", kept)
	}

	if kept := FilterVoiceProfiles(profiles, "builtin:elevenlabs"); len(kept) != 2 || kept[1].ID != "eleven-one" {
		t.Errorf("kept = %+v", kept)
	}

	// A disabled provider keeps only portable profiles, because nothing else can
	// be synthesised.
	if kept := FilterVoiceProfiles(profiles, "disabled"); len(kept) != 1 || kept[0].ID != "portable" {
		t.Errorf("kept = %+v", kept)
	}
}

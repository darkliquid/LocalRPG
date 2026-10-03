package media

import (
	"strings"
	"testing"
)

func TestStripAudioTagsRemovesDescriptiveStageDirections(t *testing.T) {
	text := "The shadow shifts. [a dry, resonant voice, echoing from the shaft] It speaks."
	got := StripAudioTags(text)
	if strings.Contains(got, "resonant") || strings.Contains(got, "[") {
		t.Fatalf("stage direction survived: %q", got)
	}
	if !strings.Contains(got, "The shadow shifts.") || !strings.Contains(got, "It speaks.") {
		t.Fatalf("prose was lost: %q", got)
	}
}

func TestStripUnsupportedTagsKeepsOnlyTheListed(t *testing.T) {
	text := "[serious] Keep walking. [a dry voice from the shaft] Go."
	got := stripUnsupportedTags(text, []string{"serious"})
	if !strings.Contains(got, "[serious]") {
		t.Fatalf("a supported tag was stripped: %q", got)
	}
	if strings.Contains(got, "shaft") {
		t.Fatalf("an unsupported tag survived: %q", got)
	}
}

func TestStripUnsupportedTagsLeavesTextWhenThereIsNoList(t *testing.T) {
	text := "[anything] goes."
	if got := stripUnsupportedTags(text, nil); got != text {
		t.Fatalf("got %q, want the text unchanged", got)
	}
}

func TestSpeakableHonoursDisabledAudioTags(t *testing.T) {
	pipeline := NewTTSPipeline(nil, NewContentCache(t.TempDir()))
	pipeline.SetSpeechCues(SpeechCueCapabilities{AudioTags: false})
	// The stage direction must not reach the provider when tags are off, even
	// though a tag-aware client would otherwise keep it.
	got := pipeline.speakable("[serious] Keep your head down.")
	if strings.Contains(got, "serious") {
		t.Fatalf("a tag survived with audio tags disabled: %q", got)
	}
	if !strings.Contains(got, "Keep your head down.") {
		t.Fatalf("the spoken words were lost: %q", got)
	}
}

func TestAudioTagsAreNotDeliveredUnlessOptedIn(t *testing.T) {
	pipeline := NewTTSPipeline(nil, NewContentCache(t.TempDir()))
	pipeline.SetSpeechCues(SpeechCueCapabilities{AudioTags: true, SupportedTags: []string{"serious"}})

	// Even a provider that declares support does not receive a tag until the
	// operator opts in, because a tag it does not honour is read aloud.
	if got := pipeline.speakable("[serious] Hold the line."); strings.Contains(got, "serious") {
		t.Fatalf("a tag was delivered by default: %q", got)
	}

	pipeline.SetAudioTagDelivery(true)
	if got := pipeline.speakable("[serious] Hold the line."); !strings.Contains(got, "[serious]") {
		t.Fatalf("a supported tag was stripped after opt-in: %q", got)
	}
	if got := pipeline.speakable("[a dry voice from the shaft] Go."); strings.Contains(got, "shaft") {
		t.Fatalf("an unsupported tag reached the provider after opt-in: %q", got)
	}
}

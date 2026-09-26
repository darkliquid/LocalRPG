package media

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestIsPCMAudio(t *testing.T) {
	cases := map[string]bool{
		"audio/L16;codec=pcm;rate=24000": true,
		"audio/pcm":                      true,
		"audio/wav":                      false,
		"audio/mpeg":                     false,
		"":                               false,
	}
	for mime, want := range cases {
		if got := IsPCMAudio(mime); got != want {
			t.Errorf("IsPCMAudio(%q) = %v, want %v", mime, got, want)
		}
	}
}

func TestPCMSampleRate(t *testing.T) {
	if got := PCMSampleRate("audio/L16;codec=pcm;rate=24000"); got != 24000 {
		t.Errorf("rate = %d, want 24000", got)
	}
	if got := PCMSampleRate("audio/L16"); got != DefaultPCMRate {
		t.Errorf("rate = %d, want the default %d", got, DefaultPCMRate)
	}
}

func TestWrapPCMAsWAV(t *testing.T) {
	pcm := []byte{1, 2, 3, 4}
	wrapped := WrapPCMAsWAV(pcm, 24000)
	if !bytes.HasPrefix(wrapped, []byte("RIFF")) {
		t.Fatal("expected a RIFF header")
	}
	if len(wrapped) != 44+len(pcm) {
		t.Fatalf("length = %d, want %d", len(wrapped), 44+len(pcm))
	}
	if AudioExtension(wrapped) != ".wav" {
		t.Fatalf("extension = %q, want .wav", AudioExtension(wrapped))
	}
	// An already-wrapped clip is returned unchanged.
	if again := WrapPCMAsWAV(wrapped, 24000); len(again) != len(wrapped) {
		t.Fatal("a clip with a container must not be double-wrapped")
	}
}

func TestClipHasValidHeader(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	if !clipHasValidHeader(write("a.opus", []byte("OggS____")), ".opus") {
		t.Error("an OggS header should be valid")
	}
	if clipHasValidHeader(write("b.opus", []byte{1, 2, 3, 4, 5, 6}), ".opus") {
		t.Error("a headerless clip should be rejected")
	}
	if clipHasValidHeader(write("c.wav", []byte("RIFF____WAVE")), ".wav") {
		t.Error("only Ogg/Opus is a stored clip format")
	}
}

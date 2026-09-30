package media

import (
	"bytes"
	"context"
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

func TestClipIsValidRequiresAWholeStream(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	opusClip := func() []byte {
		pipeline := NewTTSPipeline(&echoTTSClient{}, NewContentCache(t.TempDir()))
		path, err := pipeline.SynthesizeUtterance(context.Background(), "narrator", nil, "Keep your hood up.")
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}()

	if !clipIsValid(write("a.opus", opusClip)) {
		t.Error("a whole Opus clip should be valid")
	}

	// What an interrupted write leaves: the header, some audio, and no end-of-stream page.
	// A browser refuses it, so it must not count as a clip.
	truncated := opusClip[:len(opusClip)*2/3]
	if clipIsValid(write("b.opus", truncated)) {
		t.Error("a truncated Opus clip should be treated as broken")
	}

	if clipIsValid(write("c.opus", []byte("OggS____"))) {
		t.Error("Ogg that is not Opus should be rejected: the cache stores Opus")
	}
	if clipIsValid(write("d.opus", []byte{1, 2, 3, 4, 5, 6})) {
		t.Error("a headerless clip should be rejected")
	}
	if clipIsValid(write("e.wav", []byte("RIFF____WAVE"))) {
		t.Error("only Ogg/Opus is a stored clip format")
	}
}

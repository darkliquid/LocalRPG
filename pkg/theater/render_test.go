package theater

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestRenderFrameSize(t *testing.T) {
	s := testScript()
	s.Scenes[0].ArtPath = ""
	f := Frame{Script: s, SceneIdx: 0, BeatIdx: 1, Progress: 0.5}
	img := RenderFrame(f, 640, 360)
	if img.Bounds().Dx() != 640 || img.Bounds().Dy() != 360 {
		t.Fatalf("bounds = %v", img.Bounds())
	}
}

func TestWriteFramePNG(t *testing.T) {
	s := testScript()
	s.Scenes[0].ArtPath = ""
	path := filepath.Join(t.TempDir(), "frame.png")
	if err := WriteFramePNG(path, Frame{Script: s, SceneIdx: 0, BeatIdx: 0, Progress: 1}, 320, 180); err != nil {
		t.Fatalf("WriteFramePNG: %v", err)
	}
	fh, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer fh.Close()
	img, err := png.Decode(fh)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if img.Bounds().Dx() != 320 {
		t.Fatalf("decoded bounds = %v", img.Bounds())
	}
}

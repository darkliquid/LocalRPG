package desktop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunWithSeededStateRendersPNG(t *testing.T) {
	out := filepath.Join(t.TempDir(), "shell.png")
	st := &State{Loaded: true}
	if err := Run(Config{PNGPath: out, Width: 400, Height: 300, State: st}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// The file is written by RenderToPNG; existence is the observable contract.
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected PNG at %s: %v", out, err)
	}
}

func TestRunRequiresStateOrService(t *testing.T) {
	err := Run(Config{PNGPath: filepath.Join(t.TempDir(), "x.png"), Width: 100, Height: 100})
	if err == nil {
		t.Fatal("Run must fail when neither State nor Service is provided")
	}
}

package media

import (
	"bytes"
	"strings"
	"testing"
)

func TestFlattenSingleLayer(t *testing.T) {
	s := LayeredScene{Width: 10, Height: 10, Layers: []Layer{{Depth: 0, SVG: []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect/></svg>`)}}}
	flat := s.Flatten()
	if !bytes.Contains(flat, []byte("<svg")) || !bytes.Contains(flat, []byte("<rect/>")) {
		t.Fatalf("flatten = %q", flat)
	}
	if bytes.Contains(flat, []byte("</svg><svg")) {
		t.Fatalf("a flattened scene must not nest viewports: %q", flat)
	}
}

func TestFlattenOverlaysLayersInOrder(t *testing.T) {
	s := LayeredScene{
		Width:  10,
		Height: 10,
		Layers: []Layer{
			{Depth: 0, SVG: []byte(`<svg><a/></svg>`)},
			{Depth: 1, SVG: []byte(`<svg><b/></svg>`)},
		},
	}
	got := string(s.Flatten())
	if strings.Index(got, "<a/>") > strings.Index(got, "<b/>") {
		t.Fatalf("layers should flatten back to front, got %q", got)
	}
}

func TestGenerateLayeredScene(t *testing.T) {
	s := GenerateLayeredScene(SceneRequest{Genre: "fantasy", TimeOfDay: "day", Weather: "rain", Seed: "x"})
	if len(s.Layers) != 3 {
		t.Fatalf("layers = %d, want 3", len(s.Layers))
	}
	for i := 1; i < len(s.Layers); i++ {
		if s.Layers[i].Depth <= s.Layers[i-1].Depth {
			t.Fatalf("depths should increase back to front, got %v", s.Layers)
		}
	}
	for _, layer := range s.Layers {
		if !bytes.Contains(layer.SVG, []byte("<svg")) || !bytes.Contains(layer.SVG, []byte("</svg>")) {
			t.Fatalf("a layer must be a self-contained SVG document: %q", layer.SVG)
		}
	}
	if s.Width != 800 || s.Height != 600 {
		t.Fatalf("size = %dx%d", s.Width, s.Height)
	}
}

func TestLayeredSceneIsDeterministic(t *testing.T) {
	req := SceneRequest{Genre: "cyberpunk", Mood: "grim", TimeOfDay: "night", Weather: "fog", Seed: "y"}
	a := GenerateLayeredScene(req)
	b := GenerateLayeredScene(req)
	if len(a.Layers) != len(b.Layers) {
		t.Fatalf("layer count differs: %d and %d", len(a.Layers), len(b.Layers))
	}
	for i := range a.Layers {
		if !bytes.Equal(a.Layers[i].SVG, b.Layers[i].SVG) || a.Layers[i].Depth != b.Layers[i].Depth {
			t.Fatalf("layer %d is not deterministic", i)
		}
	}
}

// TestLayeredFlattenMatchesSingleImage guards that the flat path is the layers
// flattened, so the two outputs cannot drift.
func TestLayeredFlattenMatchesSingleImage(t *testing.T) {
	req := SceneRequest{Genre: "fantasy", Seed: "x"}
	flat := GenerateLayeredScene(req).Flatten()
	single := GenerateSceneSVG(req)
	if !bytes.Equal(flat, single) {
		t.Fatalf("flattened layers differ from the single image:\n%s\n%s", flat, single)
	}
}

// TestLayeredSceneContainsTheSameParts guards that the layers hold the scene's
// content rather than an empty shell.
func TestLayeredSceneContainsTheSameParts(t *testing.T) {
	req := SceneRequest{Genre: "fantasy", Weather: "rain", Seed: "x"}
	flat := string(GenerateLayeredScene(req).Flatten())
	if !strings.Contains(flat, "skyGrad") {
		t.Fatal("the background layer should carry the sky gradient")
	}
	if !strings.Contains(flat, "line") {
		t.Fatal("the foreground layer should carry the rain")
	}
}

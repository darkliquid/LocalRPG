package media

import (
	"bytes"
	"context"
	"testing"
)

func TestGenerateSceneIsDeterministicAndSeeded(t *testing.T) {
	req := SceneRequest{Genre: "fantasy", Mood: "grim", TimeOfDay: "night", Weather: "rain", Seed: "loc-1"}
	a := GenerateSceneSVG(req)
	b := GenerateSceneSVG(req)
	if !bytes.Equal(a, b) {
		t.Fatal("same request produced different SVG")
	}
	req.Seed = "loc-2"
	if bytes.Equal(a, GenerateSceneSVG(req)) {
		t.Fatal("a changed seed did not change the SVG")
	}
	if !bytes.Contains(a, []byte("<svg")) {
		t.Fatal("output is not SVG")
	}
}

func TestProceduralProviderImplementsSceneHint(t *testing.T) {
	var c ImageClient = NewProceduralArtClient()
	hp, ok := c.(SceneHintProvider)
	if !ok {
		t.Fatal("procedural provider must implement SceneHintProvider")
	}
	if _, err := hp.GenerateScene(context.Background(), SceneRequest{Genre: "horror", Seed: "x"}); err != nil {
		t.Fatal(err)
	}
}

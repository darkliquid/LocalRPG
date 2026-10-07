package engine

import (
	"strings"
	"testing"
)

func TestBuildScenePromptIncludesParts(t *testing.T) {
	got := BuildScenePrompt(ScenePromptContext{
		Action: "pick the lock", Entities: []string{"Kaelen", "Garrick"},
		Location: "The Drowned Hall", Style: "grim fantasy", Outcome: "miss",
	})
	for _, want := range []string{"pick the lock", "Kaelen", "The Drowned Hall", "grim fantasy", "ominous"} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt missing %q: %s", want, got)
		}
	}
}

func TestBuildScenePromptCaps(t *testing.T) {
	got := BuildScenePrompt(ScenePromptContext{Narration: strings.Repeat("x", 500)})
	if len(got) > 400 {
		t.Fatalf("prompt too long: %d", len(got))
	}
}

func TestBuildScenePromptCapsEntities(t *testing.T) {
	got := BuildScenePrompt(ScenePromptContext{Entities: []string{"ent1", "ent2", "ent3", "ent4", "ent5", "ent6"}})
	if strings.Contains(got, "ent5") || strings.Contains(got, "ent6") {
		t.Fatalf("entities should be capped at 4: %s", got)
	}
}

func TestBuildScenePromptIsDeterministic(t *testing.T) {
	ctx := ScenePromptContext{Narration: "The door gives way.", Action: "force it", Outcome: "weak"}
	if BuildScenePrompt(ctx) != BuildScenePrompt(ctx) {
		t.Fatal("the same context should produce the same prompt")
	}
}

func TestOutcomeToneWords(t *testing.T) {
	if outcomeToneWords("strong") == "" || outcomeToneWords("miss") == "" {
		t.Fatal("known outcomes should map to tone words")
	}
	if outcomeToneWords("") != "" || outcomeToneWords("nonsense") != "" {
		t.Fatal("an unknown outcome should add nothing")
	}
}

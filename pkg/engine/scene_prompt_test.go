package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestBuildScenePromptIncludesParts(t *testing.T) {
	got := BuildScenePrompt(ScenePromptContext{
		Action: "pick the lock", Cast: []SceneCastMember{
			{Name: "Kaelen", Appearance: "silver hair, blast goggles"},
			{Name: "Garrick"},
		},
		Location: "The Drowned Hall", Style: "grim fantasy", Outcome: "miss",
	})
	for _, want := range []string{"pick the lock", "Kaelen", "silver hair, blast goggles", "The Drowned Hall", "grim fantasy", "ominous"} {
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
	got := BuildScenePrompt(ScenePromptContext{Cast: []SceneCastMember{
		{Name: "ent1"}, {Name: "ent2"}, {Name: "ent3"}, {Name: "ent4"}, {Name: "ent5"}, {Name: "ent6"},
	}})
	if strings.Contains(got, "ent5") || strings.Contains(got, "ent6") {
		t.Fatalf("the cast should be capped at 4: %s", got)
	}
}

func TestSceneCastAppearanceFallsBack(t *testing.T) {
	withAppearance := &entity.Entity{Name: "Kaelen", Appearance: "silver hair", Body: "Tall."}
	if got := sceneCastAppearance(withAppearance); got != "silver hair" {
		t.Fatalf("appearance = %q, want the authored appearance", got)
	}

	withoutAppearance := &entity.Entity{Name: "Kaelen", Body: "Tall, scarred, and always armed."}
	if got := sceneCastAppearance(withoutAppearance); got == "" {
		t.Fatal("an empty appearance should fall back to the body")
	}
}

func TestBuildScenePromptIsDeterministic(t *testing.T) {
	ctx := ScenePromptContext{Narration: "The door gives way.", Action: "force it", Outcome: "weak"}
	// Comparing two named results rather than two inline calls: the test is about
	// the function being a function of its input, and naming them says so.
	first, second := BuildScenePrompt(ctx), BuildScenePrompt(ctx)
	if first != second {
		t.Fatalf("the same context should produce the same prompt\nfirst:  %q\nsecond: %q", first, second)
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

// TestSceneCastDescribesThePlayer guards the invariant a scene image depends on:
// the turn's own mentions put the player in the cast, so the protagonist's
// authored appearance reaches the prompt of every scene it appears in.
func TestSceneCastDescribesThePlayer(t *testing.T) {
	f := newResetFixture(t)

	turn := &Turn{
		Number:   1,
		Location: OpeningSceneEntityID,
		Entities: []entity.Mention{
			{ID: "sean", Kind: entity.MentionPlayer},
			{ID: OpeningSceneEntityID, Kind: entity.MentionLocation},
		},
	}

	cast := SceneCast(f.store, turn)
	if len(cast) == 0 || cast[0].Name != "Sean" {
		t.Fatalf("cast = %+v, want the player first", cast)
	}
	if cast[0].Appearance != "Weathered." {
		t.Fatalf("player appearance = %q, want the authored description", cast[0].Appearance)
	}

	prompt := BuildScenePrompt(ScenePromptContext{Cast: cast, Location: "Harbour Realm"})
	for _, want := range []string{"Sean", cast[0].Appearance} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q: %s", want, prompt)
		}
	}
}

func TestSceneCastLayersTheCurrentAppearance(t *testing.T) {
	f := newResetFixture(t)

	// Write circumstances to the protagonist's note the way a turn would, so the
	// cast has something to layer on the authored description.
	entitiesDir := filepath.Join(f.gameDir, "entities")
	notePath := filepath.Join(entitiesDir, "sean.md")
	note, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatal(err)
	}
	player, err := entity.ParseMarkdownEntity(note)
	if err != nil {
		t.Fatal(err)
	}
	player.CurrentAppearance = "Soaked through."
	out, err := player.SerializeMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath, out, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.NewSyncer(f.store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	turn := &Turn{
		Number:   1,
		Location: OpeningSceneEntityID,
		Entities: []entity.Mention{{ID: "sean", Kind: entity.MentionPlayer}},
	}
	cast := SceneCast(f.store, turn)
	if len(cast) == 0 {
		t.Fatal("expected the player in the cast")
	}
	if !strings.Contains(cast[0].Appearance, "Weathered.") {
		t.Errorf("cast appearance = %q, want the baseline", cast[0].Appearance)
	}
	if !strings.Contains(cast[0].Appearance, "Soaked through.") {
		t.Errorf("cast appearance = %q, want the current appearance layered on", cast[0].Appearance)
	}
}

func TestScenePromptCarriesThePlaceAsItIsNow(t *testing.T) {
	ctx := ScenePromptContext{
		Location:          "The Drowned Hall",
		Appearance:        "A vaulted stone hall.",
		CurrentAppearance: "The floor is flooded and the torches are out.",
		Style:             "oil painting",
	}
	got := BuildScenePrompt(ctx)
	if !strings.Contains(got, "The floor is flooded and the torches are out.") {
		t.Errorf("the moment should carry the place's current look: %s", got)
	}
	// The shared prefix is the scene's stable look, so it must not move with it.
	if strings.Contains(ScenePromptPrefix(ctx), "flooded") {
		t.Error("the current appearance belongs to the moment, not the shared prefix")
	}
}

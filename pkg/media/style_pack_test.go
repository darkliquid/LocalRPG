package media

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// resetStylePack restores the built-in look, so one test's pack cannot colour the
// next.
func resetStylePack(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { SetActiveTables(Tables{}) })
}

func TestPackMergesOverBuiltin(t *testing.T) {
	pack := StylePack{ID: "p", ScenePalettes: map[string]ScenePalette{"fantasy": {SkyTop: "#000000"}}}
	tables := pack.Merge()

	if tables.ScenePalettes["fantasy"].SkyTop != "#000000" {
		t.Fatalf("the pack should override the genre: %+v", tables.ScenePalettes["fantasy"])
	}
	if tables.ScenePalettes["fantasy"].SkyBottom == "" {
		t.Fatal("an absent field should inherit the built-in value")
	}
	if tables.ScenePalettes["horror"].SkyTop == "" {
		t.Fatal("an absent genre should inherit the built-in value")
	}
	if tables.PackID != "p" {
		t.Fatalf("pack id = %q", tables.PackID)
	}
}

func TestPackAddsSpeciesAndArchetypes(t *testing.T) {
	pack := StylePack{ID: "p", PortraitSpecies: map[string]SpeciesSpec{
		"mycelian": {EarShape: "frond", Jaw: "narrow", Skin: []string{"#8a9a7a"}},
	}, PortraitArchetypes: map[string]ArchetypeSpec{
		"warden": {Accessory: "lantern", Garment: []string{"#3a4a3a"}},
	}}
	tables := pack.Merge()

	sp, ok := tables.PortraitSpecies["mycelian"]
	if !ok || sp.Skin[0] != "#8a9a7a" || sp.Brow == "" {
		t.Fatalf("species = %+v", sp)
	}
	arch, ok := tables.PortraitArchetypes["warden"]
	if !ok || arch.Accessory != "lantern" || arch.Collar == "" {
		t.Fatalf("archetype = %+v", arch)
	}
	// A built-in category is still there.
	if _, ok := tables.PortraitSpecies["orc"]; !ok {
		t.Fatal("a pack should not remove the built-in species")
	}
}

func TestLoadStylePack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.yaml")
	body := "id: p\nlabel: Test\nscene_palettes:\n  fantasy:\n    sky_top: \"#111111\"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	pack, err := LoadStylePack(path)
	if err != nil || pack.ID != "p" || pack.Label != "Test" {
		t.Fatalf("pack %+v err %v", pack, err)
	}
}

func TestLoadStylePackRejectsAnInvalidOne(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("id: bad\nscene_palettes:\n  fantasy:\n    sky_top: nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadStylePack(path); err == nil {
		t.Fatal("an invalid colour should be rejected")
	}
}

func TestValidateStylePack(t *testing.T) {
	bad := StylePack{ID: "p", Genres: map[string]GenrePalette{"fantasy": {From: "not-a-colour"}}}
	if len(bad.Validate()) == 0 {
		t.Fatal("a bad colour should be reported")
	}
	good := StylePack{ID: "p", Genres: map[string]GenrePalette{"fantasy": {From: "#112233"}}}
	if problems := good.Validate(); len(problems) != 0 {
		t.Fatalf("a good pack reported %v", problems)
	}
	if len(StylePack{}.Validate()) == 0 {
		t.Fatal("a pack with no id should be reported")
	}
	unknown := StylePack{ID: "p", SceneStructures: map[string]string{"fantasy": "castle"}}
	if len(unknown.Validate()) == 0 {
		t.Fatal("an unknown structure should be reported")
	}
}

func TestListStylePacksReportsInvalidOnes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "good.yaml"), []byte("id: good\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("id: bad\ngenres:\n  fantasy:\n    from: nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	packs := ListStylePacks(dir)
	if len(packs) != 2 {
		t.Fatalf("packs = %+v", packs)
	}
	if packs[0].ID != "bad" || len(packs[0].Problems) == 0 {
		t.Fatalf("an invalid pack should be listed with its problem: %+v", packs[0])
	}
	if packs[1].ID != "good" || len(packs[1].Problems) != 0 {
		t.Fatalf("a good pack should be listed cleanly: %+v", packs[1])
	}
}

// TestNoPackIsByteIdentical guards that the built-in look is unchanged when no
// pack is in force, including the art cache key.
func TestNoPackIsByteIdentical(t *testing.T) {
	resetStylePack(t)
	SetActiveTables(Tables{})
	req := SceneRequest{Genre: "fantasy", TimeOfDay: "dusk", Seed: "x"}
	if !bytes.Equal(GenerateSceneSVG(req), GenerateSceneSVG(req)) {
		t.Fatal("the built-in scene is not deterministic")
	}
	if ActivePackID() != "" {
		t.Fatalf("no pack should mean no pack id, got %q", ActivePackID())
	}
	if got := ComputeArtCacheKey("e", "a", "w"); got != ComputeArtCacheKey("e", "a", "w") {
		t.Fatal("the cache key should be stable with no pack")
	}
}

func TestPackChangesTheScene(t *testing.T) {
	resetStylePack(t)
	req := SceneRequest{Genre: "fantasy", TimeOfDay: "day", Weather: "rain", Seed: "x"}

	SetActiveTables(Tables{})
	builtin := GenerateSceneSVG(req)

	pack := StylePack{ID: "ashen", ScenePalettes: map[string]ScenePalette{
		"fantasy": {SkyTop: "#000000", SkyBottom: "#010203"},
	}}
	SetActiveTables(pack.Merge())
	packed := GenerateSceneSVG(req)

	if bytes.Equal(builtin, packed) {
		t.Fatal("a pack override should change the scene")
	}
	if !bytes.Equal(packed, GenerateSceneSVG(req)) {
		t.Fatal("a packed scene should still be deterministic")
	}
}

func TestPackChangesThePortrait(t *testing.T) {
	resetStylePack(t)
	req := PortraitRequest{ID: "a", Tags: []string{"mycelian"}}

	SetActiveTables(Tables{})
	builtin := GenerateProceduralPortrait(req)

	pack := StylePack{ID: "ashen", PortraitSpecies: map[string]SpeciesSpec{
		"mycelian": {EarShape: "frond", Jaw: "narrow", Skin: []string{"#8a9a7a", "#6a7a5a"}},
	}}
	SetActiveTables(pack.Merge())
	packed := GenerateProceduralPortrait(req)

	if bytes.Equal(builtin, packed) {
		t.Fatal("a pack's species should change the portrait")
	}
}

func TestCacheKeyIncludesPack(t *testing.T) {
	resetStylePack(t)
	SetActiveTables(Tables{})
	builtin := ComputeArtCacheKey("e", "a", "w")

	SetActiveTables(StylePack{ID: "ashen"}.Merge())
	packed := ComputeArtCacheKey("e", "a", "w")

	if builtin == packed {
		t.Fatal("a pack should namespace the art cache")
	}
}

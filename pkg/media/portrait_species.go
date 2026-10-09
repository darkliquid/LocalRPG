package media

import (
	"math/rand"
	"strings"
)

// species is the silhouette a procedural portrait is drawn with: the ear, brow,
// and jaw shapes, and the skin hues the palette picks between. It is derived from
// the entity's tags, so an orc looks like an orc.
type species struct {
	ID       string
	EarShape string
	Brow     string
	Jaw      string
	Skin     []string
}

// speciesTable is the built-in species. A tag that matches none of them is a
// human, so every character still looks like someone.
var speciesTable = map[string]species{
	"human":     {ID: "human", EarShape: "round", Brow: "level", Jaw: "square", Skin: []string{"#e8c39e", "#d9a877", "#a9714b", "#6b4630"}},
	"elf":       {ID: "elf", EarShape: "pointed", Brow: "arched", Jaw: "narrow", Skin: []string{"#f2ddc4", "#dcc6a0", "#b99a72"}},
	"dwarf":     {ID: "dwarf", EarShape: "round", Brow: "heavy", Jaw: "broad", Skin: []string{"#e0b48c", "#c08c5c", "#8a5f3c"}},
	"orc":       {ID: "orc", EarShape: "pointed", Brow: "heavy", Jaw: "heavy", Skin: []string{"#8fa45f", "#6f8449", "#52643a"}},
	"halfling":  {ID: "halfling", EarShape: "pointed", Brow: "level", Jaw: "round", Skin: []string{"#efd0ab", "#d8ac7d", "#b98a5c"}},
	"beastfolk": {ID: "beastfolk", EarShape: "tufted", Brow: "heavy", Jaw: "muzzle", Skin: []string{"#c9a06a", "#8a6a44", "#6a5236"}},
	"undead":    {ID: "undead", EarShape: "round", Brow: "hollow", Jaw: "gaunt", Skin: []string{"#b9c2c0", "#8f9a98", "#6f7a78"}},
	"construct": {ID: "construct", EarShape: "plate", Brow: "ridge", Jaw: "angular", Skin: []string{"#b8b0a0", "#8a8478", "#6a665c"}},
}

// speciesTags maps a tag word to a species id, so "orcish" and "half-orc" resolve
// to the orc silhouette. Matching is case-insensitive and exact on a whole tag.
var speciesTags = map[string]string{
	"human": "human", "humans": "human", "mortal": "human", "humanoid": "human",
	"elf": "elf", "elves": "elf", "elven": "elf", "elvish": "elf", "drow": "elf",
	"dwarf": "dwarf", "dwarves": "dwarf", "dwarven": "dwarf", "dwarvish": "dwarf", "duergar": "dwarf",
	"orc": "orc", "orcs": "orc", "orcish": "orc", "ork": "orc", "half-orc": "orc",
	"goblin": "orc", "goblinoid": "orc", "hobgoblin": "orc", "troll": "orc", "ogre": "orc", "giant": "orc",
	"halfling": "halfling", "halflings": "halfling", "hobbit": "halfling", "gnome": "halfling", "kender": "halfling",
	"beastfolk": "beastfolk", "beast": "beastfolk", "beastman": "beastfolk", "catfolk": "beastfolk",
	"lizardfolk": "beastfolk", "dragonborn": "beastfolk", "minotaur": "beastfolk", "satyr": "beastfolk",
	"undead": "undead", "vampire": "undead", "zombie": "undead", "skeleton": "undead", "lich": "undead",
	"ghost": "undead", "wraith": "undead", "revenant": "undead", "spirit": "undead",
	"construct": "construct", "golem": "construct", "robot": "construct", "android": "construct",
	"automaton": "construct", "warforged": "construct", "cyborg": "construct", "drone": "construct",
}

// speciesFor resolves a species from the entity's tags, case-insensitively. An
// unmatched tag set is a human.
func speciesFor(tags []string) species {
	// The active style pack may add a species, which is selected by its own key as
	// a tag, so a pack's "mycelian" is matched before the built-in aliases.
	table := ActiveTables().PortraitSpecies
	for _, tag := range tags {
		key := strings.ToLower(strings.TrimSpace(tag))
		if sp, ok := table[key]; ok {
			return sp
		}
		if id, ok := speciesTags[key]; ok {
			return table[id]
		}
	}
	return table["human"]
}

// skinHue picks one of a species' skin tones deterministically.
func (s species) skinHue(rng *rand.Rand) string {
	if len(s.Skin) == 0 {
		return "#d9a877"
	}
	return s.Skin[rng.Intn(len(s.Skin))]
}

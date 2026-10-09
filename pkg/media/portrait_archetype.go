package media

import (
	"math/rand"
	"strings"
)

// archetype is what a portrait's subject does: the hair, collar, one accessory,
// and the garment hues the palette picks between. It is derived from the entity's
// tags.
type archetype struct {
	ID        string
	Hair      string
	Collar    string
	Accessory string
	Garment   []string
}

// archetypeTable is the built-in archetypes.
var archetypeTable = map[string]archetype{
	"warrior":  {ID: "warrior", Hair: "cropped", Collar: "gorget", Accessory: "sword", Garment: []string{"#6b7280", "#7f1d1d", "#4b5563"}},
	"mage":     {ID: "mage", Hair: "long", Collar: "high", Accessory: "staff", Garment: []string{"#4338ca", "#6d28d9", "#1e3a8a"}},
	"rogue":    {ID: "rogue", Hair: "shaggy", Collar: "hooded", Accessory: "dagger", Garment: []string{"#374151", "#1f2937", "#4c1d95"}},
	"scholar":  {ID: "scholar", Hair: "tidy", Collar: "flat", Accessory: "tome", Garment: []string{"#78350f", "#365314", "#1e3a8a"}},
	"priest":   {ID: "priest", Hair: "tonsured", Collar: "clerical", Accessory: "circlet", Garment: []string{"#e5e7eb", "#f5f5f4", "#c8a24a"}},
	"ranger":   {ID: "ranger", Hair: "braided", Collar: "cloak", Accessory: "bow", Garment: []string{"#3f6212", "#4d7c0f", "#78350f"}},
	"noble":    {ID: "noble", Hair: "styled", Collar: "ruff", Accessory: "signet", Garment: []string{"#7c2d12", "#4c1d95", "#0f766e"}},
	"labourer": {ID: "labourer", Hair: "wrapped", Collar: "open", Accessory: "satchel", Garment: []string{"#57534e", "#6b4f3a", "#44403c"}},
}

// archetypeTags maps a tag word to an archetype id.
var archetypeTags = map[string]string{
	"warrior": "warrior", "warriors": "warrior", "fighter": "warrior", "soldier": "warrior",
	"knight": "warrior", "mercenary": "warrior", "barbarian": "warrior", "gladiator": "warrior",
	"guard": "warrior", "captain": "warrior", "warlord": "warrior", "thug": "warrior",
	"mage": "mage", "wizard": "mage", "sorcerer": "mage", "sorceress": "mage", "warlock": "mage",
	"witch": "mage", "arcanist": "mage", "magus": "mage", "spellcaster": "mage", "necromancer": "mage",
	"rogue": "rogue", "thief": "rogue", "burglar": "rogue", "assassin": "rogue", "scoundrel": "rogue",
	"smuggler": "rogue", "cutpurse": "rogue", "spy": "rogue", "bandit": "rogue",
	"scholar": "scholar", "sage": "scholar", "academic": "scholar", "professor": "scholar",
	"scientist": "scholar", "engineer": "scholar", "researcher": "scholar", "scribe": "scholar",
	"priest": "priest", "priestess": "priest", "cleric": "priest", "monk": "priest", "paladin": "priest",
	"bishop": "priest", "healer": "priest", "shaman": "priest", "druid": "priest",
	"ranger": "ranger", "hunter": "ranger", "scout": "ranger", "tracker": "ranger", "trapper": "ranger",
	"woodsman": "ranger", "explorer": "ranger", "hermit": "ranger",
	"noble": "noble", "lord": "noble", "lady": "noble", "king": "noble", "queen": "noble",
	"prince": "noble", "princess": "noble", "aristocrat": "noble", "merchant": "noble", "diplomat": "noble",
	"labourer": "labourer", "laborer": "labourer", "worker": "labourer", "farmer": "labourer",
	"smith": "labourer", "blacksmith": "labourer", "servant": "labourer", "cook": "labourer", "sailor": "labourer",
}

// archetypeOrder is the deterministic order a seeded archetype is drawn from, so
// a character with no archetype tag still looks like someone.
var archetypeOrder = []string{"warrior", "mage", "rogue", "scholar", "priest", "ranger", "noble", "labourer"}

// archetypeFor resolves an archetype from the entity's tags, case-insensitively.
// An unmatched tag set picks one deterministically from the seed.
func archetypeFor(tags []string, rng *rand.Rand) archetype {
	table := ActiveTables().PortraitArchetypes
	for _, tag := range tags {
		key := strings.ToLower(strings.TrimSpace(tag))
		if arch, ok := table[key]; ok {
			return arch
		}
		if id, ok := archetypeTags[key]; ok {
			return table[id]
		}
	}
	if rng == nil {
		return table[archetypeOrder[0]]
	}
	return table[archetypeOrder[rng.Intn(len(archetypeOrder))]]
}

// garmentHue picks one of an archetype's garment colours deterministically.
func (a archetype) garmentHue(rng *rand.Rand) string {
	if len(a.Garment) == 0 {
		return "#4b5563"
	}
	return a.Garment[rng.Intn(len(a.Garment))]
}

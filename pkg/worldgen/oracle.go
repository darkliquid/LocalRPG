package worldgen

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// OracleGenerator is a deterministic, offline Generator. It fills the same
// structured shapes from templates derived from the prompt, so world generation
// works with no provider configured. It is a labelled fallback, not a substitute
// for a model: the shapes are honest, the prose is generic.
type OracleGenerator struct{}

// NewOracleGenerator returns the deterministic fallback generator.
func NewOracleGenerator() *OracleGenerator { return &OracleGenerator{} }

// Oracle reports whether a generator is the deterministic fallback, so a caller
// can label the result as such.
func Oracle(gen Generator) bool {
	_, ok := gen.(*OracleGenerator)
	return ok
}

// GenerateJSON answers a pipeline step from a template, choosing the shape by the
// schema it was handed.
func (o *OracleGenerator) GenerateJSON(_ context.Context, prompt, schema string) ([]byte, error) {
	switch {
	case strings.Contains(schema, `"locations"`):
		return json.Marshal(o.places(prompt))
	case strings.Contains(schema, `"characters"`):
		return json.Marshal(o.characters(prompt))
	case strings.Contains(schema, `"proposals"`):
		return json.Marshal(o.proposals(prompt))
	case strings.Contains(schema, `"body"`):
		return json.Marshal(o.linked(prompt))
	case strings.Contains(schema, `"art_style"`):
		return json.Marshal(o.outline(prompt))
	default:
		return json.Marshal(o.batch(prompt))
	}
}

var (
	oraclePremiseRe = regexp.MustCompile(`(?m)^Premise: (.*)$`)
	oracleNameRe    = regexp.MustCompile(`(?m)^Preferred name: (.*)$`)
	oracleLocRe     = regexp.MustCompile(`Produce (\d+) location`)
	oracleFacRe     = regexp.MustCompile(`Produce (\d+) faction`)
	oracleCharRe    = regexp.MustCompile(`Produce (\d+) character`)
	oracleEntityRe  = regexp.MustCompile(`(?m)^- id=(\S+) name=(.*?) type=(\S+)$`)
	oracleBatchRe   = regexp.MustCompile(`Produce (\d+) entities`)
	oracleKindRe    = regexp.MustCompile(`(?m)^Kinds: ([A-Za-z_ -]+)$`)
)

type oracleOutline struct {
	Name        string   `json:"name"`
	Genre       string   `json:"genre"`
	Premise     string   `json:"premise"`
	Description string   `json:"description"`
	Themes      []string `json:"themes"`
	Tone        string   `json:"tone"`
	ArtStyle    string   `json:"art_style"`
	Lore        string   `json:"lore"`
}

func (o *OracleGenerator) outline(prompt string) oracleOutline {
	premise := oracleField(oraclePremiseRe, prompt)
	name := oracleField(oracleNameRe, prompt)
	if name == "" {
		name = oracleTitle(premise)
	}
	if name == "" {
		name = "Untitled World"
	}
	description := premise
	if description == "" {
		description = name + " is a world awaiting a description."
	}
	return oracleOutline{
		Name:        name,
		Premise:     premise,
		Description: description,
		Themes:      []string{"discovery", "consequence"},
		Tone:        "grounded, with room for wonder",
		ArtStyle:    "ink and wash",
		Lore:        buildLore(name, description, "grounded, with room for wonder", []string{"discovery", "consequence"}),
	}
}

func (o *OracleGenerator) places(prompt string) map[string]any {
	locations := oraclePick(oracleLocationNames, oracleCount(oracleLocRe, prompt, 3))
	factions := oraclePick(oracleFactionNames, oracleCount(oracleFacRe, prompt, 2))
	return map[string]any{
		"locations": oraclePlaces(locations, "A place of note in this world."),
		"factions":  oraclePlaces(factions, "A faction with its own aims."),
	}
}

func (o *OracleGenerator) characters(prompt string) map[string]any {
	names := oraclePick(oracleCharacterNames, oracleCount(oracleCharRe, prompt, 3))
	characters := make([]map[string]any, 0, len(names))
	for i, name := range names {
		characters = append(characters, map[string]any{
			"name":        name,
			"role":        oracleRoles[i%len(oracleRoles)],
			"description": name + " is " + oracleRoles[i%len(oracleRoles)] + " in this world.",
		})
	}
	return map[string]any{"characters": characters}
}

func (o *OracleGenerator) batch(prompt string) map[string]any {
	count := oracleCount(oracleBatchRe, prompt, 3)
	kind := "character"
	if m := oracleKindRe.FindStringSubmatch(prompt); len(m) > 1 {
		kind = strings.TrimSpace(strings.Split(m[1], ",")[0])
	}
	pool := oracleCharacterNames
	switch kind {
	case "location":
		pool = oracleLocationNames
	case "faction":
		pool = oracleFactionNames
	default:
		kind = "character"
	}
	names := oraclePick(pool, count)
	out := make([]map[string]any, 0, len(names))
	for _, name := range names {
		out = append(out, map[string]any{
			"name":        name,
			"type":        kind,
			"description": name + " is a new " + kind + " in this world.",
		})
	}
	return map[string]any{"entities": out}
}

func (o *OracleGenerator) linked(prompt string) map[string]any {
	matches := oracleEntityRe.FindAllStringSubmatch(prompt, -1)
	type named struct{ id, name, kind string }
	entities := make([]named, 0, len(matches))
	for _, m := range matches {
		entities = append(entities, named{id: m[1], name: m[2], kind: m[3]})
	}
	out := make([]map[string]string, 0, len(entities))
	for i, e := range entities {
		body := fmt.Sprintf("%s is a %s of this world.", e.name, e.kind)
		for offset := 1; offset <= 2 && len(entities) > 1; offset++ {
			other := entities[(i+offset)%len(entities)]
			if other.id == e.id {
				continue
			}
			body += fmt.Sprintf(" It is bound to [[%s]].", other.name)
		}
		out = append(out, map[string]string{"id": e.id, "body": body})
	}
	return map[string]any{"entities": out}
}

func (o *OracleGenerator) proposals(prompt string) map[string]any {
	return map[string]any{"proposals": []map[string]string{
		{
			"kind":  "lore",
			"title": "A Deeper History",
			"body":  "Long before the present day, this world was shaped by a conflict whose outcome is still felt.",
			"reason": "the lore has no history section",
		},
		{
			"kind":   "hook",
			"title":  "A Debt Comes Due",
			"body":   "Someone arrives with a claim that cannot be ignored.",
			"reason": "gives the world an immediate story seed",
		},
	}}
}

func oraclePlaces(names []string, description string) []map[string]any {
	out := make([]map[string]any, 0, len(names))
	for _, name := range names {
		out = append(out, map[string]any{"name": name, "description": name + ": " + description})
	}
	return out
}

func oracleField(re *regexp.Regexp, prompt string) string {
	if m := re.FindStringSubmatch(prompt); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func oracleCount(re *regexp.Regexp, prompt string, fallback int) int {
	if m := re.FindStringSubmatch(prompt); len(m) > 1 {
		if n, err := strconv.Atoi(m[1]); err == nil && n >= 0 {
			return n
		}
	}
	return fallback
}

func oraclePick(pool []string, n int) []string {
	if n <= 0 {
		return nil
	}
	if n > len(pool) {
		n = len(pool)
	}
	return append([]string(nil), pool[:n]...)
}

// oracleTitle turns a premise into a world name, dropping a leading article.
func oracleTitle(premise string) string {
	words := strings.Fields(strings.TrimSpace(premise))
	if len(words) == 0 {
		return ""
	}
	if len(words) > 1 {
		switch strings.ToLower(strings.Trim(words[0], ",.")) {
		case "a", "an", "the":
			words = words[1:]
		}
	}
	if len(words) > 4 {
		words = words[:4]
	}
	titled := make([]string, 0, len(words))
	for _, w := range words {
		titled = append(titled, strings.ToUpper(w[:1])+w[1:])
	}
	return strings.Join(titled, " ")
}

// The template pools the oracle draws from. They are fixed so a generation is
// reproducible, and generic because the fallback has no model to ask.
var (
	oracleLocationNames = []string{
		"Saltmarch", "The Ashen Reach", "Hollowmere", "Old Kestrel Gate", "The Sunken Ward",
		"Blackfen", "Cinderhall", "Tidewatch Point", "Greywater", "The Long Road",
	}
	oracleFactionNames = []string{
		"The Tidewatch", "House Vane", "The Ashen Circle", "The Free Company", "The Order of the Lamp",
		"The Salt Guild", "The Hollow Choir", "The Iron Pact", "The Quiet Hand", "The Broken Crown",
	}
	oracleCharacterNames = []string{
		"Maren Vale", "Corin Ash", "Sable Quinn", "Orin Thale", "Wren Halloway",
		"Bram Kestrel", "Isolde Vance", "Tobias Grey", "Nessa Cole", "Aldric Rook",
	}
	oracleRoles = []string{"a wanderer", "a keeper of records", "a sellsword", "a healer", "a smuggler"}
)

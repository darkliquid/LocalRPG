package media

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// StylePack is a user-supplied override of the procedural look: the genre
// palettes, the scene palettes and structure preferences, and extra portrait
// categories. Every section is optional, so a small pack is useful and an absent
// key inherits the built-in value.
type StylePack struct {
	ID                 string                   `yaml:"id" json:"id"`
	Label              string                   `yaml:"label,omitempty" json:"label,omitempty"`
	Genres             map[string]GenrePalette  `yaml:"genres,omitempty" json:"genres,omitempty"`
	ScenePalettes      map[string]ScenePalette  `yaml:"scene_palettes,omitempty" json:"scene_palettes,omitempty"`
	SceneStructures    map[string]string        `yaml:"scene_structures,omitempty" json:"scene_structures,omitempty"`
	PortraitSpecies    map[string]SpeciesSpec   `yaml:"portrait_species,omitempty" json:"portrait_species,omitempty"`
	PortraitArchetypes map[string]ArchetypeSpec `yaml:"portrait_archetypes,omitempty" json:"portrait_archetypes,omitempty"`
}

// GenrePalette is a genre's colours as a pack declares them. It mirrors
// scene.GenrePalette, which pkg/media cannot import without an import cycle, so a
// pack's genre palette is surfaced for the chrome rather than drawn by the export.
type GenrePalette struct {
	From   string `yaml:"from" json:"from"`
	To     string `yaml:"to" json:"to"`
	Accent string `yaml:"accent" json:"accent"`
}

// ScenePalette is a scene palette as a pack declares it. A zero field keeps the
// built-in value, so a pack may override one colour.
type ScenePalette struct {
	SkyTop    string `yaml:"sky_top,omitempty" json:"sky_top,omitempty"`
	SkyBottom string `yaml:"sky_bottom,omitempty" json:"sky_bottom,omitempty"`
	Ground    string `yaml:"ground,omitempty" json:"ground,omitempty"`
	Ridge     string `yaml:"ridge,omitempty" json:"ridge,omitempty"`
	Fog       string `yaml:"fog,omitempty" json:"fog,omitempty"`
	Celestial string `yaml:"celestial,omitempty" json:"celestial,omitempty"`
	AccentA   string `yaml:"accent_a,omitempty" json:"accent_a,omitempty"`
	AccentB   string `yaml:"accent_b,omitempty" json:"accent_b,omitempty"`
}

// SpeciesSpec is a portrait species as a pack declares it. The key is the tag that
// selects it.
type SpeciesSpec struct {
	EarShape string   `yaml:"ear_shape,omitempty" json:"ear_shape,omitempty"`
	Brow     string   `yaml:"brow,omitempty" json:"brow,omitempty"`
	Jaw      string   `yaml:"jaw,omitempty" json:"jaw,omitempty"`
	Skin     []string `yaml:"skin,omitempty" json:"skin,omitempty"`
}

// ArchetypeSpec is a portrait archetype as a pack declares it.
type ArchetypeSpec struct {
	Hair      string   `yaml:"hair,omitempty" json:"hair,omitempty"`
	Collar    string   `yaml:"collar,omitempty" json:"collar,omitempty"`
	Accessory string   `yaml:"accessory,omitempty" json:"accessory,omitempty"`
	Garment   []string `yaml:"garment,omitempty" json:"garment,omitempty"`
}

// Tables are the procedural look in force: the built-in tables with any active
// pack merged over them. PackID names the pack, and is empty for the built-in
// look.
type Tables struct {
	Genres             map[string]GenrePalette
	ScenePalettes      map[string]palette
	SceneStructures    map[string]string
	PortraitSpecies    map[string]species
	PortraitArchetypes map[string]archetype
	PackID             string
}

// builtinTables is the look this binary ships, which every pack merges over.
func builtinTables() Tables {
	return Tables{
		Genres:             map[string]GenrePalette{},
		ScenePalettes:      cloneScenePalettes(basePalettes),
		SceneStructures:    cloneStrings(genreStructures),
		PortraitSpecies:    cloneSpecies(speciesTable),
		PortraitArchetypes: cloneArchetypes(archetypeTable),
	}
}

// Merge returns the built-in tables with the pack's entries overlaid by key. An
// absent key inherits, so a pack that names one genre leaves the rest alone.
func (p StylePack) Merge() Tables {
	tables := builtinTables()
	tables.PackID = strings.TrimSpace(p.ID)
	for genre, palette := range p.Genres {
		tables.Genres[strings.ToLower(strings.TrimSpace(genre))] = palette
	}
	for genre, override := range p.ScenePalettes {
		key := strings.ToLower(strings.TrimSpace(genre))
		base := tables.ScenePalettes[key]
		if base == (palette{}) {
			base = basePalettes["fantasy"]
		}
		tables.ScenePalettes[key] = override.merge(base)
	}
	for genre, structure := range p.SceneStructures {
		name := strings.ToLower(strings.TrimSpace(structure))
		if _, ok := structures[name]; !ok {
			continue
		}
		tables.SceneStructures[strings.ToLower(strings.TrimSpace(genre))] = name
	}
	for id, spec := range p.PortraitSpecies {
		key := strings.ToLower(strings.TrimSpace(id))
		base := tables.PortraitSpecies[key]
		if base.ID == "" {
			base = species{ID: key, EarShape: "round", Brow: "level", Jaw: "square"}
		}
		base.EarShape = firstNonEmpty(spec.EarShape, base.EarShape)
		base.Brow = firstNonEmpty(spec.Brow, base.Brow)
		base.Jaw = firstNonEmpty(spec.Jaw, base.Jaw)
		if len(spec.Skin) > 0 {
			base.Skin = spec.Skin
		}
		if len(base.Skin) == 0 {
			base.Skin = speciesTable["human"].Skin
		}
		tables.PortraitSpecies[key] = base
	}
	for id, spec := range p.PortraitArchetypes {
		key := strings.ToLower(strings.TrimSpace(id))
		base := tables.PortraitArchetypes[key]
		if base.ID == "" {
			base = archetype{ID: key, Hair: "cropped", Collar: "open", Accessory: "satchel"}
		}
		base.Hair = firstNonEmpty(spec.Hair, base.Hair)
		base.Collar = firstNonEmpty(spec.Collar, base.Collar)
		base.Accessory = firstNonEmpty(spec.Accessory, base.Accessory)
		if len(spec.Garment) > 0 {
			base.Garment = spec.Garment
		}
		if len(base.Garment) == 0 {
			base.Garment = archetypeTable["labourer"].Garment
		}
		tables.PortraitArchetypes[key] = base
	}
	return tables
}

// merge overlays a pack's scene palette over a built-in one, field by field, so a
// pack may set one colour and keep the rest.
func (s ScenePalette) merge(base palette) palette {
	return palette{
		SkyTop:    firstNonEmpty(s.SkyTop, base.SkyTop),
		SkyBottom: firstNonEmpty(s.SkyBottom, base.SkyBottom),
		Ground:    firstNonEmpty(s.Ground, base.Ground),
		Ridge:     firstNonEmpty(s.Ridge, base.Ridge),
		Fog:       firstNonEmpty(s.Fog, base.Fog),
		Celestial: firstNonEmpty(s.Celestial, base.Celestial),
		AccentA:   firstNonEmpty(s.AccentA, base.AccentA),
		AccentB:   firstNonEmpty(s.AccentB, base.AccentB),
	}
}

// StylePackPath names a pack's file inside a styles directory.
func StylePackPath(dir, id string) string {
	return filepath.Join(dir, strings.TrimSpace(id)+".yaml")
}

// LoadStylePack reads and validates a pack from a file. A pack with no id is
// rejected, because the id names it in the configuration and in the art cache.
func LoadStylePack(path string) (StylePack, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return StylePack{}, fmt.Errorf("read style pack: %w", err)
	}
	var pack StylePack
	if err := yaml.Unmarshal(data, &pack); err != nil {
		return StylePack{}, fmt.Errorf("parse style pack: %w", err)
	}
	pack.ID = strings.TrimSpace(pack.ID)
	if pack.ID == "" {
		return StylePack{}, fmt.Errorf("style pack %s has no id", filepath.Base(path))
	}
	if problems := pack.Validate(); len(problems) > 0 {
		return StylePack{}, fmt.Errorf("style pack %s is invalid: %s", pack.ID, strings.Join(problems, "; "))
	}
	return pack, nil
}

// ListStylePacks returns the packs under a directory, sorted by id. A pack that
// does not load is reported with its problem rather than skipped, so the settings
// can explain it.
func ListStylePacks(dir string) []StylePackStatus {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	statuses := make([]StylePackStatus, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		pack, err := LoadStylePack(path)
		if err != nil {
			statuses = append(statuses, StylePackStatus{
				ID:       strings.TrimSuffix(entry.Name(), ".yaml"),
				Path:     path,
				Problems: []string{err.Error()},
			})
			continue
		}
		statuses = append(statuses, StylePackStatus{ID: pack.ID, Label: pack.Label, Path: path})
	}
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].ID < statuses[j].ID })
	return statuses
}

// StylePackStatus is one pack as a listing reports it.
type StylePackStatus struct {
	ID       string   `json:"id"`
	Label    string   `json:"label,omitempty"`
	Path     string   `json:"path"`
	Problems []string `json:"problems,omitempty"`
}

var colourPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// Validate reports a pack's problems, so a malformed pack is explained rather
// than silently ignored. An empty result means the pack is usable.
func (p StylePack) Validate() []string {
	var problems []string
	if strings.TrimSpace(p.ID) == "" {
		problems = append(problems, "id is required")
	}
	for genre, palette := range p.Genres {
		for _, colour := range []string{palette.From, palette.To, palette.Accent} {
			if colour != "" && !colourPattern.MatchString(colour) {
				problems = append(problems, fmt.Sprintf("genres.%s: %q is not a #rrggbb colour", genre, colour))
			}
		}
	}
	for genre, palette := range p.ScenePalettes {
		if _, known := basePalettes[strings.ToLower(strings.TrimSpace(genre))]; !known {
			problems = append(problems, fmt.Sprintf("scene_palettes.%s: unknown genre", genre))
		}
		for name, colour := range palette.fields() {
			if colour != "" && !colourPattern.MatchString(colour) {
				problems = append(problems, fmt.Sprintf("scene_palettes.%s.%s: %q is not a #rrggbb colour", genre, name, colour))
			}
		}
	}
	for genre, structure := range p.SceneStructures {
		if _, known := structures[strings.ToLower(strings.TrimSpace(structure))]; !known {
			problems = append(problems, fmt.Sprintf("scene_structures.%s: %q is not a known structure", genre, structure))
		}
	}
	for id, spec := range p.PortraitSpecies {
		if spec.EarShape != "" && !oneOf(spec.EarShape, earShapes) {
			problems = append(problems, fmt.Sprintf("portrait_species.%s.ear_shape: %q is not a known shape", id, spec.EarShape))
		}
		if spec.Brow != "" && !oneOf(spec.Brow, browShapes) {
			problems = append(problems, fmt.Sprintf("portrait_species.%s.brow: %q is not a known shape", id, spec.Brow))
		}
		if spec.Jaw != "" && !oneOf(spec.Jaw, jawShapes) {
			problems = append(problems, fmt.Sprintf("portrait_species.%s.jaw: %q is not a known shape", id, spec.Jaw))
		}
		for _, colour := range spec.Skin {
			if !colourPattern.MatchString(colour) {
				problems = append(problems, fmt.Sprintf("portrait_species.%s.skin: %q is not a #rrggbb colour", id, colour))
			}
		}
	}
	for id, spec := range p.PortraitArchetypes {
		if spec.Hair != "" && !oneOf(spec.Hair, hairStyles) {
			problems = append(problems, fmt.Sprintf("portrait_archetypes.%s.hair: %q is not a known style", id, spec.Hair))
		}
		if spec.Collar != "" && !oneOf(spec.Collar, collarStyles) {
			problems = append(problems, fmt.Sprintf("portrait_archetypes.%s.collar: %q is not a known style", id, spec.Collar))
		}
		if spec.Accessory != "" && !oneOf(spec.Accessory, accessories) {
			problems = append(problems, fmt.Sprintf("portrait_archetypes.%s.accessory: %q is not a known accessory", id, spec.Accessory))
		}
		for _, colour := range spec.Garment {
			if !colourPattern.MatchString(colour) {
				problems = append(problems, fmt.Sprintf("portrait_archetypes.%s.garment: %q is not a #rrggbb colour", id, colour))
			}
		}
	}
	return problems
}

// fields names a scene palette's colours, so validation can report which one is
// malformed.
func (s ScenePalette) fields() map[string]string {
	return map[string]string{
		"sky_top": s.SkyTop, "sky_bottom": s.SkyBottom, "ground": s.Ground, "ridge": s.Ridge,
		"fog": s.Fog, "celestial": s.Celestial, "accent_a": s.AccentA, "accent_b": s.AccentB,
	}
}

// The shapes a pack may name, so a typo is reported rather than drawn as nothing.
var (
	earShapes    = []string{"round", "pointed", "tufted", "plate"}
	browShapes   = []string{"level", "arched", "heavy", "hollow", "ridge"}
	jawShapes    = []string{"square", "broad", "narrow", "round", "muzzle", "gaunt", "angular"}
	hairStyles   = []string{"cropped", "long", "shaggy", "tidy", "tonsured", "braided", "styled", "wrapped", "hooded"}
	collarStyles = []string{"gorget", "high", "hooded", "flat", "clerical", "cloak", "ruff", "open"}
	accessories  = []string{"sword", "staff", "dagger", "tome", "circlet", "bow", "signet", "satchel"}
)

func oneOf(value string, allowed []string) bool {
	for _, candidate := range allowed {
		if strings.EqualFold(strings.TrimSpace(value), candidate) {
			return true
		}
	}
	return false
}

func firstNonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func cloneScenePalettes(in map[string]palette) map[string]palette {
	out := make(map[string]palette, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneStrings(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneSpecies(in map[string]species) map[string]species {
	out := make(map[string]species, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneArchetypes(in map[string]archetype) map[string]archetype {
	out := make(map[string]archetype, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

// activeTables holds the look in force. It is package-level rather than threaded
// through every call because the generator is reached from the image pipeline, the
// GUI, and the export with no configuration in hand; the process sets it once.
var (
	activeMu     sync.RWMutex
	activeTables = builtinTables()
)

// SetActiveTables puts a pack's merged tables in force. The zero value restores
// the built-in look.
func SetActiveTables(tables Tables) {
	activeMu.Lock()
	defer activeMu.Unlock()
	if tables.ScenePalettes == nil {
		activeTables = builtinTables()
		return
	}
	activeTables = tables
}

// ActiveTables returns the look in force.
func ActiveTables() Tables {
	activeMu.RLock()
	defer activeMu.RUnlock()
	return activeTables
}

// ActivePackID names the pack in force, or "" for the built-in look.
func ActivePackID() string { return ActiveTables().PackID }

package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/pathutil"
	"golang.org/x/mod/semver"
	"gopkg.in/yaml.v3"
)

// SystemManifest is a system's system.yaml: what it is called, the requirements it
// places on a world, the mechanics it declares, and the character fields it asks for.
type SystemManifest struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description,omitempty"`
	// CharacterCreation describes the prompts a player answers when starting a
	// campaign with this system. An empty spec falls back to the engine default.
	CharacterCreation CharacterCreationSpec `yaml:"character_creation,omitempty"`
	// Mechanics is the optional declarative mechanics schema. A nil value means
	// the system is schema-agnostic and mechanics.js owns everything.
	Mechanics *MechanicsSpec `yaml:"mechanics,omitempty"`
}

// CharacterCreationField is one prompt in a system's character creation.
type CharacterCreationField struct {
	ID          string   `yaml:"id" json:"id"`
	Label       string   `yaml:"label" json:"label"`
	Prompt      string   `yaml:"prompt,omitempty" json:"prompt,omitempty"`
	Kind        string   `yaml:"kind,omitempty" json:"kind,omitempty"` // text | long | number | select | voice
	Required    bool     `yaml:"required,omitempty" json:"required,omitempty"`
	Generatable bool     `yaml:"generatable,omitempty" json:"generatable,omitempty"`
	Options     []string `yaml:"options,omitempty" json:"options,omitempty"`
	Default     string   `yaml:"default,omitempty" json:"default,omitempty"`
}

// CharacterCreationSpec is a system's set of character creation prompts.
type CharacterCreationSpec struct {
	Preamble string                   `yaml:"preamble,omitempty" json:"preamble,omitempty"`
	Fields   []CharacterCreationField `yaml:"fields,omitempty" json:"fields,omitempty"`
}

// ContentRequirement names a content dependency and semver constraint.
type ContentRequirement struct {
	Type    string `yaml:"type"`              // e.g. "system"
	ID      string `yaml:"id"`                // e.g. "narrative_2d6"
	Version string `yaml:"version,omitempty"` // semver constraint, e.g. ">=1.0.0 <2.0.0"
}

// Satisfies checks whether targetVersion satisfies the requirement's version constraint.
// An empty constraint accepts any version.
func (r ContentRequirement) Satisfies(targetVersion string) bool {
	constraint := strings.TrimSpace(r.Version)
	if constraint == "" {
		return true
	}
	targetCanon := CanonicalSemver(strings.TrimSpace(targetVersion))
	if !semver.IsValid(targetCanon) {
		return false
	}

	clauses := strings.Fields(constraint)
	for _, clause := range clauses {
		op := "=="
		vStr := clause
		if strings.HasPrefix(clause, ">=") {
			op = ">="
			vStr = clause[2:]
		} else if strings.HasPrefix(clause, "<=") {
			op = "<="
			vStr = clause[2:]
		} else if strings.HasPrefix(clause, ">") {
			op = ">"
			vStr = clause[1:]
		} else if strings.HasPrefix(clause, "<") {
			op = "<"
			vStr = clause[1:]
		} else if strings.HasPrefix(clause, "==") {
			op = "=="
			vStr = clause[2:]
		} else if strings.HasPrefix(clause, "=") {
			op = "=="
			vStr = clause[1:]
		}

		clauseCanon := CanonicalSemver(strings.TrimSpace(vStr))
		if !semver.IsValid(clauseCanon) {
			return false
		}

		cmp := semver.Compare(targetCanon, clauseCanon)
		switch op {
		case ">=":
			if cmp < 0 {
				return false
			}
		case "<=":
			if cmp > 0 {
				return false
			}
		case ">":
			if cmp <= 0 {
				return false
			}
		case "<":
			if cmp >= 0 {
				return false
			}
		case "==":
			if cmp != 0 {
				return false
			}
		default:
			return false
		}
	}

	return true
}

// WorldManifest is a world's world.yaml: its name, genre and art style, the systems
// it will play with, and where a campaign starts.
type WorldManifest struct {
	ID            string               `yaml:"id"`
	Name          string               `yaml:"name"`
	Version       string               `yaml:"version,omitempty"`
	Description   string               `yaml:"description,omitempty"`
	Genre         string               `yaml:"genre,omitempty"`
	DefaultSystem string               `yaml:"default_system,omitempty"`
	Requires      []ContentRequirement `yaml:"requires,omitempty"`
	ArtStyle      string               `yaml:"art_style,omitempty"`
	Tags          []string             `yaml:"tags,omitempty"`
}

// GameManifest is a campaign's game.yaml: the system and world it was created from,
// its player, and the location it opens at.
type GameManifest struct {
	ID         string         `yaml:"id"`
	Name       string         `yaml:"name"`
	SystemID   string         `yaml:"system"`
	WorldID    string         `yaml:"world"`
	Player     string         `yaml:"player"`
	PlayerName string         `yaml:"player_name,omitempty"`
	Settings   map[string]any `yaml:"settings,omitempty"`
}

// SaveGameManifest writes a campaign manifest. It creates no directories, so a
// caller that is replacing a manifest leaves nothing half-built behind.
func SaveGameManifest(path string, manifest *GameManifest) error {
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal game manifest: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write game manifest: %w", err)
	}
	return nil
}

// PathResolver turns a category and an id into a directory, and is the only thing in
// the app that decides where content lives.
type PathResolver struct {
	BaseDir    string
	systemsDir string
	worldsDir  string
	gamesDir   string
	cacheDir   string
}

// NewPathResolver resolves every category under baseDir, which is what a test or an
// export that works in one directory wants.
func NewPathResolver(baseDir string) *PathResolver {
	return &PathResolver{
		BaseDir:    baseDir,
		systemsDir: filepath.Join(baseDir, "systems"),
		worldsDir:  filepath.Join(baseDir, "worlds"),
		gamesDir:   filepath.Join(baseDir, "games"),
		cacheDir:   filepath.Join(baseDir, "cache"),
	}
}

// NewCustomPathResolver resolves each category to its own directory, which is what a
// resolved configuration provides.
func NewCustomPathResolver(systemsDir, worldsDir, gamesDir, cacheDir string) *PathResolver {
	return &PathResolver{
		BaseDir:    "",
		systemsDir: systemsDir,
		worldsDir:  worldsDir,
		gamesDir:   gamesDir,
		cacheDir:   cacheDir,
	}
}

// SetPaths replaces all four directories in place, which is what a settings save does
// rather than rebuilding the resolver.
func (p *PathResolver) SetPaths(systemsDir, worldsDir, gamesDir, cacheDir string) {
	p.systemsDir = systemsDir
	p.worldsDir = worldsDir
	p.gamesDir = gamesDir
	p.cacheDir = cacheDir
}

// SystemsDir is the directory the systems live in.
func (p *PathResolver) SystemsDir() string {
	if p.systemsDir != "" {
		return p.systemsDir
	}
	return filepath.Join(p.BaseDir, "systems")
}

// SystemDir is one system's directory.
func (p *PathResolver) SystemDir(id string) string {
	return filepath.Join(p.SystemsDir(), pathutil.SanitizeID(id))
}

// WorldsDir is the directory the worlds live in.
func (p *PathResolver) WorldsDir() string {
	if p.worldsDir != "" {
		return p.worldsDir
	}
	return filepath.Join(p.BaseDir, "worlds")
}

// WorldDir is one world's directory.
func (p *PathResolver) WorldDir(id string) string {
	return filepath.Join(p.WorldsDir(), pathutil.SanitizeID(id))
}

// GamesDir is the directory the campaigns live in.
func (p *PathResolver) GamesDir() string {
	if p.gamesDir != "" {
		return p.gamesDir
	}
	return filepath.Join(p.BaseDir, "games")
}

// GameDir is one campaign's directory.
func (p *PathResolver) GameDir(id string) string {
	return filepath.Join(p.GamesDir(), pathutil.SanitizeID(id))
}

// GameDBPath returns the canonical SQLite index for a campaign.
func (p *PathResolver) GameDBPath(gameID string) string {
	return filepath.Join(p.GameDir(gameID), "cache", "index.db")
}

// CacheDir is the directory derived data lives in, which is safe to delete.
func (p *PathResolver) CacheDir() string {
	if p.cacheDir != "" {
		return p.cacheDir
	}
	return filepath.Join(p.BaseDir, "cache")
}

// CanonicalSemver prepends 'v' if missing for semver package compatibility.
func CanonicalSemver(v string) string {
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(v, "v") {
		return "v" + v
	}
	return v
}

// ValidateSemver checks if v is valid semantic versioning (accepts with or without leading 'v').
// An empty string is considered valid (unversioned).
func ValidateSemver(v string) error {
	if v == "" {
		return nil
	}
	if !semver.IsValid(CanonicalSemver(v)) {
		return fmt.Errorf("invalid semver: %q", v)
	}
	return nil
}

// Validate reports what a system manifest is missing, which is the fields a system
// cannot be loaded without.
func (s SystemManifest) Validate() error {
	if err := ValidateSemver(s.Version); err != nil {
		return fmt.Errorf("system %q: %w", s.ID, err)
	}
	return nil
}

// Validate reports what a world manifest is missing.
func (w WorldManifest) Validate() error {
	if err := ValidateSemver(w.Version); err != nil {
		return fmt.Errorf("world %q: %w", w.ID, err)
	}
	return nil
}

// LoadSystemManifest reads and parses one system.yaml, applying the defaults a file
// need not state.
func LoadSystemManifest(path string) (*SystemManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read system manifest: %w", err)
	}
	var manifest SystemManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("unmarshal system manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return &manifest, nil
}

// LoadWorldManifest reads and parses one world.yaml.
func LoadWorldManifest(path string) (*WorldManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read world manifest: %w", err)
	}
	var manifest WorldManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("unmarshal world manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return &manifest, nil
}

// LoadGameManifest reads and parses one game.yaml.
func LoadGameManifest(path string) (*GameManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read game manifest: %w", err)
	}
	var manifest GameManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("unmarshal game manifest: %w", err)
	}
	return &manifest, nil
}

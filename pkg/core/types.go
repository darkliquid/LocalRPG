package core

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

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

type WorldManifest struct {
	ID            string   `yaml:"id"`
	Name          string   `yaml:"name"`
	Description   string   `yaml:"description,omitempty"`
	Genre         string   `yaml:"genre,omitempty"`
	DefaultSystem string   `yaml:"default_system,omitempty"`
	ArtStyle      string   `yaml:"art_style,omitempty"`
	Tags          []string `yaml:"tags,omitempty"`
}

type GameManifest struct {
	ID         string                 `yaml:"id"`
	Name       string                 `yaml:"name"`
	SystemID   string                 `yaml:"system"`
	WorldID    string                 `yaml:"world"`
	Player     string                 `yaml:"player"`
	PlayerName string                 `yaml:"player_name,omitempty"`
	Settings   map[string]interface{} `yaml:"settings,omitempty"`
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

type PathResolver struct {
	BaseDir    string
	systemsDir string
	worldsDir  string
	gamesDir   string
	cacheDir   string
}

func NewPathResolver(baseDir string) *PathResolver {
	return &PathResolver{
		BaseDir:    baseDir,
		systemsDir: filepath.Join(baseDir, "systems"),
		worldsDir:  filepath.Join(baseDir, "worlds"),
		gamesDir:   filepath.Join(baseDir, "games"),
		cacheDir:   filepath.Join(baseDir, "cache"),
	}
}

func NewCustomPathResolver(systemsDir, worldsDir, gamesDir, cacheDir string) *PathResolver {
	return &PathResolver{
		BaseDir:    "",
		systemsDir: systemsDir,
		worldsDir:  worldsDir,
		gamesDir:   gamesDir,
		cacheDir:   cacheDir,
	}
}

func (p *PathResolver) SetPaths(systemsDir, worldsDir, gamesDir, cacheDir string) {
	p.systemsDir = systemsDir
	p.worldsDir = worldsDir
	p.gamesDir = gamesDir
	p.cacheDir = cacheDir
}

func (p *PathResolver) SystemsDir() string {
	if p.systemsDir != "" {
		return p.systemsDir
	}
	return filepath.Join(p.BaseDir, "systems")
}

func (p *PathResolver) SystemDir(id string) string {
	return filepath.Join(p.SystemsDir(), id)
}

func (p *PathResolver) WorldsDir() string {
	if p.worldsDir != "" {
		return p.worldsDir
	}
	return filepath.Join(p.BaseDir, "worlds")
}

func (p *PathResolver) WorldDir(id string) string {
	return filepath.Join(p.WorldsDir(), id)
}

func (p *PathResolver) GamesDir() string {
	if p.gamesDir != "" {
		return p.gamesDir
	}
	return filepath.Join(p.BaseDir, "games")
}

func (p *PathResolver) GameDir(id string) string {
	return filepath.Join(p.GamesDir(), id)
}

// GameDBPath returns the canonical SQLite index for a campaign.
func (p *PathResolver) GameDBPath(gameID string) string {
	return filepath.Join(p.GameDir(gameID), "cache", "index.db")
}

func (p *PathResolver) CacheDir() string {
	if p.cacheDir != "" {
		return p.cacheDir
	}
	return filepath.Join(p.BaseDir, "cache")
}

func LoadSystemManifest(path string) (*SystemManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read system manifest: %w", err)
	}
	var manifest SystemManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("unmarshal system manifest: %w", err)
	}
	return &manifest, nil
}

func LoadWorldManifest(path string) (*WorldManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read world manifest: %w", err)
	}
	var manifest WorldManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("unmarshal world manifest: %w", err)
	}
	return &manifest, nil
}

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

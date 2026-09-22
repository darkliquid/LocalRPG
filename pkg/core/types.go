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

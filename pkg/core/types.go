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
	ID       string                 `yaml:"id"`
	Name     string                 `yaml:"name"`
	SystemID string                 `yaml:"system"`
	WorldID  string                 `yaml:"world"`
	Player   string                 `yaml:"player"`
	Settings map[string]interface{} `yaml:"settings,omitempty"`
}

type PathResolver struct {
	BaseDir string
}

func NewPathResolver(baseDir string) *PathResolver {
	return &PathResolver{BaseDir: baseDir}
}

func (p *PathResolver) SystemsDir() string {
	return filepath.Join(p.BaseDir, "systems")
}

func (p *PathResolver) SystemDir(id string) string {
	return filepath.Join(p.SystemsDir(), id)
}

func (p *PathResolver) WorldsDir() string {
	return filepath.Join(p.BaseDir, "worlds")
}

func (p *PathResolver) WorldDir(id string) string {
	return filepath.Join(p.WorldsDir(), id)
}

func (p *PathResolver) GamesDir() string {
	return filepath.Join(p.BaseDir, "games")
}

func (p *PathResolver) GameDir(id string) string {
	return filepath.Join(p.GamesDir(), id)
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

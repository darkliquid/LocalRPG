package content

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Manifest describes a content package.
type Manifest struct {
	ID           string       `yaml:"id"`
	Name         string       `yaml:"name"`
	Version      string       `yaml:"version"`
	Type         string       `yaml:"type"` // "system" | "world"
	Description  string       `yaml:"description,omitempty"`
	Author       string       `yaml:"author,omitempty"`
	License      string       `yaml:"license,omitempty"`
	MinApp       string       `yaml:"min_app_version,omitempty"`
	Dependencies []Dependency `yaml:"dependencies,omitempty"`
	Files        []FileEntry  `yaml:"files"`
}

// Dependency is a required system or world and its version constraint.
type Dependency struct {
	Type    string `yaml:"type"` // "system" | "world"
	ID      string `yaml:"id"`
	Version string `yaml:"version,omitempty"` // semver constraint
}

// FileEntry records a file's path and checksum.
type FileEntry struct {
	Path   string `yaml:"path"`
	SHA256 string `yaml:"sha256"`
	Size   int64  `yaml:"size"`
}

// ManifestMeta holds metadata provided when packing a content tree.
type ManifestMeta struct {
	ID           string       `yaml:"id,omitempty"`
	Name         string       `yaml:"name,omitempty"`
	Version      string       `yaml:"version,omitempty"`
	Description  string       `yaml:"description,omitempty"`
	Author       string       `yaml:"author,omitempty"`
	License      string       `yaml:"license,omitempty"`
	MinApp       string       `yaml:"min_app_version,omitempty"`
	Dependencies []Dependency `yaml:"dependencies,omitempty"`
}

// ParseManifest unmarshals a YAML manifest from bytes.
func ParseManifest(data []byte) (Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("unmarshal manifest: %w", err)
	}
	return m, nil
}

// Marshal marshals the manifest to YAML bytes.
func (m Manifest) Marshal() ([]byte, error) {
	data, err := yaml.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}
	return data, nil
}

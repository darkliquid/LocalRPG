package content

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

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

// ContentDigest returns the stable digest of a manifest's files: the SHA-256 of
// the sorted "path\x00sha256\x00size\n" lines. It excludes package.sig.
func (m Manifest) ContentDigest() string {
	entries := make([]FileEntry, 0, len(m.Files))
	for _, f := range m.Files {
		if f.Path == "package.sig" {
			continue
		}
		entries = append(entries, f)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})

	h := sha256.New()
	for _, f := range entries {
		fmt.Fprintf(h, "%s\x00%s\x00%d\n", f.Path, f.SHA256, f.Size)
	}

	return hex.EncodeToString(h.Sum(nil))
}

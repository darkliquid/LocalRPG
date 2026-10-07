package content

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// ContentLock records the content versions and digests a campaign was played against.
type ContentLock struct {
	App     string      `yaml:"app,omitempty"`
	Entries []LockEntry `yaml:"entries"`
}

// LockEntry is one resolved content dependency and its behavioural digest.
type LockEntry struct {
	Type    string `yaml:"type"`    // "system" | "world"
	ID      string `yaml:"id"`
	Version string `yaml:"version"`
	SHA256  string `yaml:"sha256"`  // behavioural digest
}

// FindEntry returns the entry for the given type and ID, if present.
func (l ContentLock) FindEntry(typ, id string) (LockEntry, bool) {
	for _, e := range l.Entries {
		if e.Type == typ && e.ID == id {
			return e, true
		}
	}
	return LockEntry{}, false
}

// LoadLock reads and parses a content.lock.yaml from path.
func LoadLock(path string) (ContentLock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ContentLock{}, err
	}
	var lock ContentLock
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return ContentLock{}, fmt.Errorf("unmarshal lockfile %s: %w", path, err)
	}
	return lock, nil
}

// Save serializes and writes the lock to path.
func (l ContentLock) Save(path string) error {
	data, err := yaml.Marshal(l)
	if err != nil {
		return fmt.Errorf("marshal lockfile: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create lockfile dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write lockfile %s: %w", path, err)
	}
	return nil
}

// BehaviouralDigest returns a deterministic SHA256 checksum of the behavioural files
// in a system or world directory (excluding prose and documentation).
// For systems: system.yaml, mechanics.js.
// For worlds: world.yaml, and any system_overrides/*/hooks.js.
func BehaviouralDigest(dir string) (string, error) {
	var candidates []string

	// Check for system.yaml and mechanics.js
	sysYAML := filepath.Join(dir, "system.yaml")
	if info, err := os.Stat(sysYAML); err == nil && !info.IsDir() {
		candidates = append(candidates, "system.yaml")
		mechJS := filepath.Join(dir, "mechanics.js")
		if minfo, err := os.Stat(mechJS); err == nil && !minfo.IsDir() {
			candidates = append(candidates, "mechanics.js")
		}
	}

	// Check for world.yaml and system_overrides
	worldYAML := filepath.Join(dir, "world.yaml")
	if info, err := os.Stat(worldYAML); err == nil && !info.IsDir() {
		candidates = append(candidates, "world.yaml")

		overridesDir := filepath.Join(dir, "system_overrides")
		_ = filepath.WalkDir(overridesDir, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil || d == nil || d.IsDir() {
				return nil
			}
			if d.Name() == "hooks.js" {
				rel, err := filepath.Rel(dir, path)
				if err == nil {
					candidates = append(candidates, filepath.ToSlash(rel))
				}
			}
			return nil
		})
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no behavioural files found in %s", dir)
	}

	sort.Strings(candidates)

	h := sha256.New()
	for _, rel := range candidates {
		fullPath := filepath.Join(dir, filepath.FromSlash(rel))
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", rel, err)
		}
		fileSum := sha256.Sum256(data)
		fmt.Fprintf(h, "%s %x\n", rel, fileSum)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

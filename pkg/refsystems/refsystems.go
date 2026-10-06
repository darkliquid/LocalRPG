// Package refsystems ships complete, runnable systems as starting points. They
// are embedded so the studio, the docs, and the tests share one source.
package refsystems

import (
	"embed"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"gopkg.in/yaml.v3"
)

//go:embed systems
var files embed.FS

// ReferenceSystem is a complete, runnable system shipped as a starting point.
type ReferenceSystem struct {
	ID          string
	Name        string
	Version     string
	Description string
	RulesPrompt string
	Script      string
	Mechanics   *core.MechanicsSpec
}

// List returns every reference system, ordered by ID.
func List() []ReferenceSystem {
	entries, err := fs.ReadDir(files, "systems")
	if err != nil {
		return nil
	}
	out := make([]ReferenceSystem, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if sys, ok := load(e.Name()); ok {
			out = append(out, sys)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Get returns one reference system by id.
func Get(id string) (ReferenceSystem, bool) {
	for _, s := range List() {
		if s.ID == id {
			return s, true
		}
	}
	return ReferenceSystem{}, false
}

// ScenarioFile is one embedded scenario's file name and raw YAML.
type ScenarioFile struct {
	Name string
	Data []byte
}

// ScenarioFiles returns a system's embedded scenario files, ordered by name.
func ScenarioFiles(id string) []ScenarioFile {
	dir := path.Join("systems", id, "tests")
	entries, err := fs.ReadDir(files, dir)
	if err != nil {
		return nil
	}
	out := make([]ScenarioFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := files.ReadFile(path.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, ScenarioFile{Name: e.Name(), Data: data})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func load(dir string) (ReferenceSystem, bool) {
	manifest, err := files.ReadFile(path.Join("systems", dir, "system.yaml"))
	if err != nil {
		return ReferenceSystem{}, false
	}
	var m core.SystemManifest
	if err := yaml.Unmarshal(manifest, &m); err != nil {
		return ReferenceSystem{}, false
	}
	script, _ := files.ReadFile(path.Join("systems", dir, "mechanics.js"))
	rules, _ := files.ReadFile(path.Join("systems", dir, "prompts", "rules.md"))
	return ReferenceSystem{
		ID: m.ID, Name: m.Name, Version: m.Version, Description: m.Description,
		RulesPrompt: strings.TrimSpace(string(rules)),
		Script:      string(script),
		Mechanics:   m.Mechanics,
	}, true
}

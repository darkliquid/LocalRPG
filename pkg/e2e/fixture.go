package e2e

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/gui"
)

// Fixture is the application under test: a real gui.Service on a temp root,
// served over an httptest server with the embedded SPA. Every browser test
// starts here, so no test re-implements the server wiring.
type Fixture struct {
	Service *gui.Service
	Server  *httptest.Server
	Root    string
}

// NewFixture starts the app on a fresh temp root with the given config.yaml.
// An empty configYAML leaves the app on its defaults.
func NewFixture(t *testing.T, configYAML string) *Fixture {
	t.Helper()
	root := t.TempDir()
	if configYAML != "" {
		if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(configYAML), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	svc := gui.NewService(root)
	t.Cleanup(svc.Close)

	server := httptest.NewServer(gui.NewServer(svc, gui.AssetHandler()))
	t.Cleanup(server.Close)

	return &Fixture{Service: svc, Server: server, Root: root}
}

// Launch opens a browser on the fixture's server.
func (f *Fixture) Launch(t *testing.T) *Browser {
	t.Helper()
	return NewBrowser(t, f.Server.URL)
}

// WriteSystem writes a minimal system manifest.
func (f *Fixture) WriteSystem(t *testing.T, id, name string) {
	t.Helper()
	writeFile(t, filepath.Join(f.Service.GetResolver().SystemDir(id), "system.yaml"),
		fmt.Sprintf("id: %s\nname: %s\nversion: \"1.0\"\n", id, name))
}

// WriteWorld writes a minimal world manifest with an entities directory.
func (f *Fixture) WriteWorld(t *testing.T, id, name string, compatible []string) {
	t.Helper()
	dir := f.Service.GetResolver().WorldDir(id)
	var body strings.Builder
	fmt.Fprintf(&body, "id: %s\nname: %s\n", id, name)
	if len(compatible) > 0 {
		body.WriteString("compatible_systems:\n")
		for _, system := range compatible {
			fmt.Fprintf(&body, "  - %s\n", system)
		}
	}
	writeFile(t, filepath.Join(dir, "world.yaml"), body.String())
	if err := os.MkdirAll(filepath.Join(dir, "entities"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// InitGame creates a campaign and closes the init session.
func (f *Fixture) InitGame(t *testing.T, opts engine.InitOptions) {
	t.Helper()
	session, err := engine.InitGame(f.Service.GetResolver(), opts)
	if err != nil {
		t.Fatalf("InitGame: %v", err)
	}
	_ = session.Close()
}

// WriteEntities writes entity notes into a campaign.
func (f *Fixture) WriteEntities(t *testing.T, gameID string, files map[string]string) {
	t.Helper()
	dir := filepath.Join(f.Service.GetResolver().GameDir(gameID), "entities")
	for name, body := range files {
		writeFile(t, filepath.Join(dir, name), body)
	}
}

// WriteHistory replaces a campaign's history log with the given JSONL.
func (f *Fixture) WriteHistory(t *testing.T, gameID, lines string) {
	t.Helper()
	writeFile(t, filepath.Join(f.Service.GetResolver().GameDir(gameID), "history.jsonl"), lines)
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

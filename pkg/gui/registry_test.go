package gui

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/content"
	"github.com/darkliquid/localrpg/pkg/registry"
)

func TestRegistrySearchEndpoint(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name": "Community Registry",
			"packages": [
				{
					"type": "world",
					"id": "ember_peaks",
					"name": "Ember Peaks",
					"version": "1.0.0",
					"description": "Volcanic highlands.",
					"download": "https://example.org/ember.lrpgpack",
					"sha256": "abcdef"
				}
			]
		}`))
	}))
	defer s.Close()

	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, "config")
	t.Setenv("LOCALRPG_CONFIG_DIR", cfgDir)

	svc := NewService(tmpDir)
	cfg := svc.configMgr.Get()
	cfg.Registries.URLs = []string{s.URL}
	if err := svc.configMgr.Save(cfg); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/registry/search?q=ember", nil)
	rr := httptest.NewRecorder()
	svc.HandleRegistrySearch(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	var results []registry.PackageRef
	if err := json.Unmarshal(rr.Body.Bytes(), &results); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if len(results) != 1 || results[0].Package.ID != "ember_peaks" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestRegistryInstallEndpoint(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	worldYAML := "id: install_api_world\nname: API World\nversion: 1.0.0\n"
	if err := os.WriteFile(filepath.Join(srcDir, "world.yaml"), []byte(worldYAML), 0644); err != nil {
		t.Fatal(err)
	}

	var packBuf bytes.Buffer
	_, err := content.Pack(srcDir, "world", content.ManifestMeta{}, &packBuf)
	if err != nil {
		t.Fatal(err)
	}
	packBytes := packBuf.Bytes()
	h := sha256.Sum256(packBytes)
	actualSHA256 := hex.EncodeToString(h[:])

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(packBytes)
	}))
	defer server.Close()

	svc := NewService(tmpDir)

	body := RegistryInstallRequestDTO{
		Ref: registry.PackageRef{
			RegistryName: "TestReg",
			RegistryURL:  server.URL + "/index.json",
			Package: registry.Package{
				Type:     "world",
				ID:       "install_api_world",
				Name:     "API World",
				Version:  "1.0.0",
				Download: server.URL + "/pkg.lrpgpack",
				SHA256:   actualSHA256,
			},
		},
		OnConflict: "refuse",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/registry/install", bytes.NewReader(bodyBytes))
	rr := httptest.NewRecorder()
	svc.HandleRegistryInstall(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	var res ImportResultDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if res.ID != "install_api_world" {
		t.Fatalf("expected installed ID 'install_api_world', got %q", res.ID)
	}
}

func TestRegistryUpdatesEndpoint(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name": "Community Registry",
			"packages": [
				{
					"type": "world",
					"id": "my_world",
					"name": "My World",
					"version": "2.0.0",
					"description": "Updated world.",
					"download": "https://example.org/world2.lrpgpack",
					"sha256": "123456"
				}
			]
		}`))
	}))
	defer s.Close()

	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, "config")
	t.Setenv("LOCALRPG_CONFIG_DIR", cfgDir)

	worldDir := filepath.Join(tmpDir, "worlds", "my_world")
	if err := os.MkdirAll(worldDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte("id: my_world\nname: My World\nversion: 1.0.0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewService(tmpDir)
	cfg := svc.configMgr.Get()
	cfg.Registries.URLs = []string{s.URL}
	if err := svc.configMgr.Save(cfg); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/registry/updates", nil)
	rr := httptest.NewRecorder()
	svc.HandleRegistryUpdates(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	var results []registry.PackageRef
	if err := json.Unmarshal(rr.Body.Bytes(), &results); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if len(results) != 1 || results[0].Package.ID != "my_world" || results[0].Package.Version != "2.0.0" {
		t.Fatalf("unexpected updates results: %+v", results)
	}
}

func newRegistryIndexServer(t *testing.T, name string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"` + name + `","packages":[{"type":"world","id":"frontier","name":"Frontier","version":"1.0.0","download":"https://example.org/f.lrpgpack","sha256":"abc"}]}`))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestRegistrySourcesListsConfiguredURLs(t *testing.T) {
	server := newRegistryIndexServer(t, "Registry One")

	tmpDir := t.TempDir()
	t.Setenv("LOCALRPG_CONFIG_DIR", filepath.Join(tmpDir, "config"))

	svc := NewService(tmpDir)
	cfg := svc.configMgr.Get()
	cfg.Registries.URLs = []string{server.URL}
	if err := svc.configMgr.Save(cfg); err != nil {
		t.Fatal(err)
	}

	sources, err := svc.RegistrySources(context.Background())
	if err != nil {
		t.Fatalf("RegistrySources: %v", err)
	}
	if len(sources) != 1 || sources[0].URL != server.URL || sources[0].Name != "Registry One" || sources[0].PackageCount != 1 || sources[0].Error != "" {
		t.Fatalf("unexpected sources: %+v", sources)
	}
}

func TestAddRegistrySourcePersistsAndRejectsDuplicate(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("LOCALRPG_CONFIG_DIR", filepath.Join(tmpDir, "config"))
	svc := NewService(tmpDir)

	const url = "https://example.org/index.json"
	source, err := svc.AddRegistrySource(context.Background(), url)
	if err != nil {
		t.Fatalf("AddRegistrySource: %v", err)
	}
	if source.URL != url {
		t.Fatalf("source.URL = %q, want %q", source.URL, url)
	}

	if _, err := svc.AddRegistrySource(context.Background(), url); !errors.Is(err, ErrRegistrySourceExists) {
		t.Fatalf("duplicate err = %v, want ErrRegistrySourceExists", err)
	}
	if _, err := svc.AddRegistrySource(context.Background(), "ftp://example.org"); err == nil {
		t.Fatal("expected an invalid URL to be rejected")
	}

	cfg := svc.configMgr.Get()
	if len(cfg.Registries.URLs) != 1 || cfg.Registries.URLs[0] != url {
		t.Fatalf("persisted URLs = %v, want [%s]", cfg.Registries.URLs, url)
	}
}

func TestRemoveRegistrySourcePersistsAndReportsAbsent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("LOCALRPG_CONFIG_DIR", filepath.Join(tmpDir, "config"))
	svc := NewService(tmpDir)

	const kept = "https://example.org/one.json"
	const dropped = "https://example.org/two.json"
	for _, u := range []string{kept, dropped} {
		if _, err := svc.AddRegistrySource(context.Background(), u); err != nil {
			t.Fatalf("AddRegistrySource(%q): %v", u, err)
		}
	}

	if err := svc.RemoveRegistrySource(context.Background(), dropped); err != nil {
		t.Fatalf("RemoveRegistrySource: %v", err)
	}
	if err := svc.RemoveRegistrySource(context.Background(), dropped); !errors.Is(err, ErrRegistrySourceNotFound) {
		t.Fatalf("removing an absent source err = %v, want ErrRegistrySourceNotFound", err)
	}

	cfg := svc.configMgr.Get()
	if len(cfg.Registries.URLs) != 1 || cfg.Registries.URLs[0] != kept {
		t.Fatalf("persisted URLs = %v, want [%s]", cfg.Registries.URLs, kept)
	}
}

func TestRegistrySourcesRouteDispatchesByMethod(t *testing.T) {
	server := newRegistryIndexServer(t, "Route Registry")

	tmpDir := t.TempDir()
	t.Setenv("LOCALRPG_CONFIG_DIR", filepath.Join(tmpDir, "config"))
	svc := NewService(tmpDir)
	srv := NewServer(svc, http.NotFoundHandler())

	post := httptest.NewRequest(http.MethodPost, "/api/registry/sources", strings.NewReader(`{"url":"`+server.URL+`"}`))
	post.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, post)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/registry/sources", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var sources []RegistrySourceDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &sources); err != nil {
		t.Fatalf("unmarshal sources: %v", err)
	}
	if len(sources) != 1 || sources[0].URL != server.URL || sources[0].Name != "Route Registry" {
		t.Fatalf("unexpected sources: %+v", sources)
	}

	del := httptest.NewRequest(http.MethodDelete, "/api/registry/sources?url="+url.QueryEscape(server.URL), nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, del)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204: %s", rec.Code, rec.Body.String())
	}

	// A second DELETE for the same URL is a 404: the source is gone.
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/registry/sources?url="+url.QueryEscape(server.URL), nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("second DELETE status = %d, want 404", rec.Code)
	}
}


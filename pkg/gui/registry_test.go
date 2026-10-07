package gui

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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


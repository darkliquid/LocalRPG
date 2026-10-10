package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/content"
)

func TestRegistryAddListRemoveCLI(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("LOCALRPG_CONFIG_DIR", cfgDir)

	var stdout, stderr bytes.Buffer

	// 1. Initially empty
	code := runRegistryCommand([]string{"list"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("registry list failed: %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No registries configured") {
		t.Fatalf("expected 'No registries configured', got: %s", stdout.String())
	}

	// 2. Add registry URL
	testURL := "https://example.org/index.json"
	stdout.Reset()
	stderr.Reset()
	code = runRegistryCommand([]string{"add", testURL}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("registry add failed: %d, stderr: %s", code, stderr.String())
	}

	// 3. List contains URL
	stdout.Reset()
	stderr.Reset()
	code = runRegistryCommand([]string{"list"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("registry list failed: %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), testURL) {
		t.Fatalf("expected %s in list, got: %s", testURL, stdout.String())
	}

	// 4. Remove URL
	stdout.Reset()
	stderr.Reset()
	code = runRegistryCommand([]string{"remove", testURL}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("registry remove failed: %d, stderr: %s", code, stderr.String())
	}

	// 5. List is empty again
	stdout.Reset()
	stderr.Reset()
	code = runRegistryCommand([]string{"list"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("registry list failed: %d, stderr: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), testURL) {
		t.Fatalf("expected URL to be removed, got: %s", stdout.String())
	}
}

func TestRegistrySearchCLI(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name": "Community Registry",
			"packages": [
				{
					"type": "world",
					"id": "ashen_reach",
					"name": "Ashen Reach",
					"version": "1.2.0",
					"description": "A dying frontier.",
					"download": "https://example.org/ashen.lrpgpack",
					"sha256": "111111"
				}
			]
		}`))
	}))
	defer s.Close()

	cfgDir := t.TempDir()
	t.Setenv("LOCALRPG_CONFIG_DIR", cfgDir)

	// A registry command treats the current directory as the project root, so the
	// index cache and the content it reads land there. Run from a temp directory
	// so the test leaves nothing behind in the package directory.
	workDir := t.TempDir()
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldCwd) }()

	var stdout, stderr bytes.Buffer
	// Add test registry
	code := runRegistryCommand([]string{"add", s.URL}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("add failed: %d, stderr: %s", code, stderr.String())
	}

	// Search for "ash"
	stdout.Reset()
	stderr.Reset()
	code = runRegistryCommand([]string{"search", "ash"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("search failed: %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "ashen_reach") || !strings.Contains(stdout.String(), "Ashen Reach") {
		t.Fatalf("expected ashen_reach in search output, got: %s", stdout.String())
	}
}

func TestRegistryInstallCLI(t *testing.T) {
	// Create real package
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	worldYAML := "id: install_world\nname: Install World\nversion: 1.0.0\n"
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
		if strings.HasSuffix(r.URL.Path, "index.json") || r.URL.Path == "/" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"name": "Install Registry",
				"packages": [
					{
						"type": "world",
						"id": "install_world",
						"name": "Install World",
						"version": "1.0.0",
						"download": "pkg.lrpgpack",
						"sha256": "` + actualSHA256 + `"
					}
				]
			}`))
			return
		}
		_, _ = w.Write(packBytes)
	}))
	defer server.Close()

	workDir := t.TempDir()
	cfgDir := filepath.Join(workDir, "config")
	t.Setenv("LOCALRPG_CONFIG_DIR", cfgDir)

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldCwd) }()

	var stdout, stderr bytes.Buffer
	// Add registry
	code := runRegistryCommand([]string{"add", server.URL + "/index.json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("add failed: %d, stderr: %s", code, stderr.String())
	}

	// Install package
	stdout.Reset()
	stderr.Reset()
	code = runRegistryCommand([]string{"install", "install_world", "--yes"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("install failed: %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "install_world") {
		t.Fatalf("expected install_world in stdout, got: %s", stdout.String())
	}

	// Verify installed content exists on disk
	installedPath := filepath.Join(workDir, "worlds", "install_world", "world.yaml")
	if _, err := os.Stat(installedPath); err != nil {
		t.Fatalf("expected installed file %s to exist: %v", installedPath, err)
	}
}

func TestRegistryAddRejectsInvalidURL(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("LOCALRPG_CONFIG_DIR", cfgDir)

	var stdout, stderr bytes.Buffer
	if code := runRegistryCommand([]string{"add", "ftp://example.org"}, &stdout, &stderr); code == 0 {
		t.Fatal("registry add accepted an invalid URL")
	}

	stdout.Reset()
	stderr.Reset()
	if code := runRegistryCommand([]string{"list"}, &stdout, &stderr); code != 0 {
		t.Fatalf("registry list failed: %d", code)
	}
	if !strings.Contains(stdout.String(), "No registries configured") {
		t.Fatalf("an invalid URL must not be stored, got: %s", stdout.String())
	}
}

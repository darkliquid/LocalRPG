package registry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/content"
)

func TestClientCachesIndexes(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte(`{
			"name": "Community Registry",
			"packages": [
				{
					"type": "world",
					"id": "frontier",
					"name": "Frontier",
					"version": "1.0.0",
					"download": "https://example.org/frontier.lrpgpack",
					"sha256": "abcdef123456"
				}
			]
		}`))
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	cfg := config.RegistriesConfig{
		URLs: []string{server.URL},
	}

	client := NewClient(cfg, cacheDir)
	client.SetHTTPClient(server.Client())

	// 1. Initial fetch from server
	indexes, err := client.Indexes(context.Background())
	if err != nil {
		t.Fatalf("first Indexes() failed: %v", err)
	}
	if len(indexes) != 1 || indexes[0].Name != "Community Registry" {
		t.Fatalf("expected Community Registry index, got: %+v", indexes)
	}
	if requestCount.Load() != 1 {
		t.Fatalf("expected 1 request, got %d", requestCount.Load())
	}

	// 2. Shut down the server so it becomes unreachable
	server.Close()

	// 3. Second call should be served from cache
	cachedClient := NewClient(cfg, cacheDir)
	cachedIndexes, err := cachedClient.Indexes(context.Background())
	if err != nil {
		t.Fatalf("second Indexes() from cache failed: %v", err)
	}
	if len(cachedIndexes) != 1 || cachedIndexes[0].Name != "Community Registry" {
		t.Fatalf("expected Community Registry from cache, got: %+v", cachedIndexes)
	}
	if len(cachedIndexes[0].Packages) != 1 || cachedIndexes[0].Packages[0].ID != "frontier" {
		t.Fatalf("expected frontier package from cache, got: %+v", cachedIndexes[0].Packages)
	}
}

func TestClientSurvivesOneBadRegistry(t *testing.T) {
	goodServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name": "Good Registry",
			"packages": []
		}`))
	}))
	defer goodServer.Close()

	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer badServer.Close()

	cacheDir := t.TempDir()
	cfg := config.RegistriesConfig{
		URLs: []string{badServer.URL, goodServer.URL},
	}

	client := NewClient(cfg, cacheDir)
	client.SetHTTPClient(goodServer.Client())

	indexes, err := client.Indexes(context.Background())
	if err != nil {
		t.Fatalf("Indexes() failed when one registry is bad: %v", err)
	}
	if len(indexes) != 1 || indexes[0].Name != "Good Registry" {
		t.Fatalf("expected 1 good index, got: %+v", indexes)
	}
}

func TestClientEmptyRegistries(t *testing.T) {
	cacheDir := t.TempDir()
	cfg := config.RegistriesConfig{}

	client := NewClient(cfg, cacheDir)
	indexes, err := client.Indexes(context.Background())
	if err != nil {
		t.Fatalf("Indexes() failed on empty config: %v", err)
	}
	if len(indexes) != 0 {
		t.Fatalf("expected 0 indexes, got %d", len(indexes))
	}
}

func TestSearchMatchesAcrossIndexes(t *testing.T) {
	s1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name": "Community",
			"packages": [
				{
					"type": "world",
					"id": "ashen_reach",
					"name": "Ashen Reach",
					"version": "1.2.0",
					"description": "A dying frontier.",
					"author": "Bob",
					"download": "https://example.org/ashen.lrpgpack",
					"sha256": "111111"
				}
			]
		}`))
	}))
	defer s1.Close()

	s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name": "Official",
			"packages": [
				{
					"type": "system",
					"id": "dusk_realm",
					"name": "Dusk Realm",
					"version": "2.0.0",
					"description": "A shadowy kingdom.",
					"author": "Alice",
					"download": "https://example.org/dusk.lrpgpack",
					"sha256": "222222"
				}
			]
		}`))
	}))
	defer s2.Close()

	cacheDir := t.TempDir()
	cfg := config.RegistriesConfig{
		URLs: []string{s1.URL, s2.URL},
	}
	client := NewClient(cfg, cacheDir)

	ctx := context.Background()

	// 1. Query "ash" matches ashen_reach
	res, err := client.Search(ctx, "ash")
	if err != nil {
		t.Fatalf("Search('ash') failed: %v", err)
	}
	if len(res) != 1 || res[0].Package.ID != "ashen_reach" || res[0].RegistryName != "Community" {
		t.Fatalf("unexpected Search('ash') result: %+v", res)
	}

	// 2. Query "shadowy" matches description
	res, err = client.Search(ctx, "shadowy")
	if err != nil {
		t.Fatalf("Search('shadowy') failed: %v", err)
	}
	if len(res) != 1 || res[0].Package.ID != "dusk_realm" || res[0].RegistryName != "Official" {
		t.Fatalf("unexpected Search('shadowy') result: %+v", res)
	}

	// 3. Query "alice" matches author
	res, err = client.Search(ctx, "alice")
	if err != nil {
		t.Fatalf("Search('alice') failed: %v", err)
	}
	if len(res) != 1 || res[0].Package.ID != "dusk_realm" {
		t.Fatalf("unexpected Search('alice') result: %+v", res)
	}

	// 4. Empty query returns all packages
	res, err = client.Search(ctx, "")
	if err != nil {
		t.Fatalf("Search('') failed: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 packages for empty query, got %d", len(res))
	}

	// 5. Query with no match
	res, err = client.Search(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Search('nonexistent') failed: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("expected 0 packages, got %d", len(res))
	}
}

func TestInstallVerifiesChecksum(t *testing.T) {
	pkgBytes := []byte("some package payload")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(pkgBytes)
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	client := NewClient(config.RegistriesConfig{}, cacheDir)
	client.SetHTTPClient(server.Client())

	ref := PackageRef{
		RegistryName: "TestReg",
		RegistryURL:  server.URL + "/index.json",
		Package: Package{
			Type:     "world",
			ID:       "corrupt_pkg",
			Name:     "Corrupt Package",
			Version:  "1.0.0",
			Download: server.URL + "/pkg.lrpgpack",
			SHA256:   "expected_different_sha256",
		},
	}

	_, err := client.Install(context.Background(), ref, "refuse")
	if err == nil {
		t.Fatal("expected error on checksum mismatch, got nil")
	}
}

func TestInstallDelegatesToImport(t *testing.T) {
	// Create a real package
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	worldYAML := "id: my_world\nname: My World\nversion: 1.0.0\n"
	if err := os.WriteFile(filepath.Join(srcDir, "world.yaml"), []byte(worldYAML), 0644); err != nil {
		t.Fatal(err)
	}

	var packBuf bytes.Buffer
	m, err := content.Pack(srcDir, "world", content.ManifestMeta{}, &packBuf)
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

	cacheDir := t.TempDir()
	client := NewClient(config.RegistriesConfig{}, cacheDir)
	client.SetHTTPClient(server.Client())

	var delegateCalled bool
	var receivedConflict string
	client.SetInstaller(func(ctx context.Context, r io.Reader, onConflict string) (content.Manifest, error) {
		delegateCalled = true
		receivedConflict = onConflict
		readBytes, err := io.ReadAll(r)
		if err != nil {
			return content.Manifest{}, err
		}
		if len(readBytes) != len(packBytes) {
			t.Fatalf("delegate received %d bytes, want %d", len(readBytes), len(packBytes))
		}
		return m, nil
	})

	ref := PackageRef{
		RegistryName: "TestReg",
		RegistryURL:  server.URL + "/registry/index.json",
		Package: Package{
			Type:     "world",
			ID:       "my_world",
			Name:     "My World",
			Version:  "1.0.0",
			Download: "my_world.lrpgpack", // relative URL
			SHA256:   actualSHA256,
		},
	}

	installedM, err := client.Install(context.Background(), ref, "rename")
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if !delegateCalled {
		t.Fatal("expected installer delegate to be called")
	}
	if receivedConflict != "rename" {
		t.Fatalf("expected onConflict 'rename', got %q", receivedConflict)
	}
	if installedM.ID != "my_world" {
		t.Fatalf("expected installed ID 'my_world', got %q", installedM.ID)
	}

	// Now shut down server: second install should work from cache
	server.Close()
	delegateCalled = false
	_, err = client.Install(context.Background(), ref, "overwrite")
	if err != nil {
		t.Fatalf("offline Install from package cache failed: %v", err)
	}
	if !delegateCalled {
		t.Fatal("expected installer delegate to be called from cache")
	}
}

func TestUpdateFindsNewerVersions(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name": "Updates Registry",
			"packages": [
				{
					"type": "world",
					"id": "my_world",
					"name": "My World",
					"version": "1.5.0",
					"download": "https://example.org/w15.lrpgpack",
					"sha256": "123"
				},
				{
					"type": "world",
					"id": "current_world",
					"name": "Current World",
					"version": "1.0.0",
					"download": "https://example.org/cw.lrpgpack",
					"sha256": "456"
				}
			]
		}`))
	}))
	defer s.Close()

	cacheDir := t.TempDir()
	client := NewClient(config.RegistriesConfig{URLs: []string{s.URL}}, cacheDir)
	client.SetHTTPClient(s.Client())

	// Configure installed packages
	client.SetInstalledLister(func(ctx context.Context) ([]content.Manifest, error) {
		return []content.Manifest{
			{ID: "my_world", Type: "world", Version: "1.0.0"},
			{ID: "current_world", Type: "world", Version: "1.0.0"},
		}, nil
	})

	updates, err := client.Update(context.Background())
	if err != nil {
		t.Fatalf("Update() failed: %v", err)
	}

	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d (%+v)", len(updates), updates)
	}
	if updates[0].Package.ID != "my_world" || updates[0].Package.Version != "1.5.0" {
		t.Fatalf("unexpected update: %+v", updates[0])
	}
}

func TestGitRegistryScheme(t *testing.T) {
	// Create a local git repository fixture
	gitDir := t.TempDir()
	cmdInit := exec.Command("git", "init", gitDir)
	if out, err := cmdInit.CombinedOutput(); err != nil {
		t.Skipf("git init not available: %v (%s)", err, out)
	}

	// Configure author for commit
	_ = exec.Command("git", "-C", gitDir, "config", "user.name", "Tester").Run()
	_ = exec.Command("git", "-C", gitDir, "config", "user.email", "test@test.com").Run()

	indexJSON := `{
		"name": "Git Registry",
		"packages": [
			{
				"type": "world",
				"id": "git_world",
				"name": "Git World",
				"version": "1.0.0",
				"download": "https://example.org/git.lrpgpack",
				"sha256": "999"
			}
		]
	}`
	if err := os.WriteFile(filepath.Join(gitDir, "index.json"), []byte(indexJSON), 0644); err != nil {
		t.Fatal(err)
	}

	_ = exec.Command("git", "-C", gitDir, "add", "index.json").Run()
	if out, err := exec.Command("git", "-C", gitDir, "commit", "-m", "init").CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v (%s)", err, out)
	}

	cacheDir := t.TempDir()
	gitURL := "git+file://" + gitDir
	client := NewClient(config.RegistriesConfig{URLs: []string{gitURL}}, cacheDir)

	indexes, err := client.Indexes(context.Background())
	if err != nil {
		t.Fatalf("Indexes() with git+ scheme failed: %v", err)
	}
	if len(indexes) != 1 || indexes[0].Name != "Git Registry" {
		t.Fatalf("expected Git Registry index, got: %+v", indexes)
	}
	if len(indexes[0].Packages) != 1 || indexes[0].Packages[0].ID != "git_world" {
		t.Fatalf("expected git_world package, got: %+v", indexes[0].Packages)
	}
}

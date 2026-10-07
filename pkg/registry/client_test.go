package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
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

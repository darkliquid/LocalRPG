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

package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
)

// Client fetches, caches, and queries registry package indexes.
type Client struct {
	cfg        config.RegistriesConfig
	cacheDir   string
	httpClient *http.Client
	mu         sync.RWMutex
}

// NewClient constructs a Client for the given registries configuration and cache directory.
func NewClient(cfg config.RegistriesConfig, cacheDir string) *Client {
	return &Client{
		cfg:      cfg,
		cacheDir: cacheDir,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetHTTPClient overrides the HTTP client (useful for tests or proxies).
func (c *Client) SetHTTPClient(client *http.Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.httpClient = client
}

func (c *Client) getHTTPClient() *http.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.httpClient != nil {
		return c.httpClient
	}
	return http.DefaultClient
}

type cachedMeta struct {
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
}

func (c *Client) cacheKey(url string) string {
	h := sha256.Sum256([]byte(url))
	return hex.EncodeToString(h[:])
}

// Indexes returns the parsed indexes from all configured registries.
// It caches indexes on disk and falls back to the cache if a registry is unreachable.
func (c *Client) Indexes(ctx context.Context) ([]Index, error) {
	if len(c.cfg.URLs) == 0 {
		return []Index{}, nil
	}

	var results []Index
	var lastErr error

	for _, rawURL := range c.cfg.URLs {
		idx, err := c.fetchOrCachedIndex(ctx, rawURL)
		if err != nil {
			lastErr = err
			continue
		}
		results = append(results, idx)
	}

	// If no registries succeeded and we had configured URLs, return lastErr
	if len(results) == 0 && len(c.cfg.URLs) > 0 {
		return nil, fmt.Errorf("all registries failed: %w", lastErr)
	}

	return results, nil
}

func (c *Client) fetchOrCachedIndex(ctx context.Context, rawURL string) (Index, error) {
	key := c.cacheKey(rawURL)
	cachePath := filepath.Join(c.cacheDir, "registries", key+".json")
	metaPath := filepath.Join(c.cacheDir, "registries", key+".meta")

	// Try reading cached metadata
	var meta cachedMeta
	if metaBytes, err := os.ReadFile(metaPath); err == nil {
		_ = json.Unmarshal(metaBytes, &meta)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err == nil {
		if meta.ETag != "" {
			req.Header.Set("If-None-Match", meta.ETag)
		}
		if meta.LastModified != "" {
			req.Header.Set("If-Modified-Since", meta.LastModified)
		}

		client := c.getHTTPClient()
		resp, fetchErr := client.Do(req)
		if fetchErr == nil {
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusNotModified {
				// Cache is fresh
				if cachedData, err := os.ReadFile(cachePath); err == nil {
					return ParseIndex(cachedData)
				}
			} else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				bodyBytes, err := io.ReadAll(resp.Body)
				if err == nil {
					idx, err := ParseIndex(bodyBytes)
					if err == nil {
						// Update cache
						_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
						_ = os.WriteFile(cachePath, bodyBytes, 0644)
						newMeta := cachedMeta{
							ETag:         resp.Header.Get("ETag"),
							LastModified: resp.Header.Get("Last-Modified"),
						}
						newMetaBytes, _ := json.Marshal(newMeta)
						_ = os.WriteFile(metaPath, newMetaBytes, 0644)
						return idx, nil
					}
				}
			}
		}
	}

	// Fallback to cache if network fetch failed
	if cachedData, err := os.ReadFile(cachePath); err == nil {
		return ParseIndex(cachedData)
	}

	return Index{}, fmt.Errorf("fetch registry index %q: network failed and no cache available", rawURL)
}

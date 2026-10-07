package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/content"
	"github.com/darkliquid/localrpg/pkg/core"
)

// ContentInstaller delegates the actual content staging and installation.
type ContentInstaller func(ctx context.Context, r io.Reader, onConflict string) (content.Manifest, error)

// InstalledLister returns the manifests of installed content packages.
type InstalledLister func(ctx context.Context) ([]content.Manifest, error)

// Client fetches, caches, and queries registry package indexes.
type Client struct {
	cfg             config.RegistriesConfig
	cacheDir        string
	httpClient      *http.Client
	installer       ContentInstaller
	installedLister InstalledLister
	mu              sync.RWMutex
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

// Registry pairs a configured registry index with its name, URL, and fetch time.
type Registry struct {
	Name    string    `json:"name"`
	URL     string    `json:"url"`
	Index   Index     `json:"index"`
	Fetched time.Time `json:"fetched,omitempty"`
}

// Registries returns the configured registries with their parsed indexes.
func (c *Client) Registries(ctx context.Context) ([]Registry, error) {
	if len(c.cfg.URLs) == 0 {
		return []Registry{}, nil
	}

	var results []Registry
	var lastErr error

	for _, rawURL := range c.cfg.URLs {
		idx, err := c.fetchOrCachedIndex(ctx, rawURL)
		if err != nil {
			lastErr = err
			continue
		}
		results = append(results, Registry{
			Name:    idx.Name,
			URL:     rawURL,
			Index:   idx,
			Fetched: time.Now(),
		})
	}

	if len(results) == 0 && len(c.cfg.URLs) > 0 {
		return nil, fmt.Errorf("all registries failed: %w", lastErr)
	}

	return results, nil
}

// Indexes returns the parsed indexes from all configured registries.
// It caches indexes on disk and falls back to the cache if a registry is unreachable.
func (c *Client) Indexes(ctx context.Context) ([]Index, error) {
	regs, err := c.Registries(ctx)
	if err != nil {
		return nil, err
	}
	indexes := make([]Index, len(regs))
	for i, r := range regs {
		indexes[i] = r.Index
	}
	return indexes, nil
}

// Search performs a case-insensitive substring match across id, name, description, and author
// across every configured registry index, returning PackageRef items.
func (c *Client) Search(ctx context.Context, query string) ([]PackageRef, error) {
	regs, err := c.Registries(ctx)
	if err != nil {
		return nil, err
	}

	q := strings.ToLower(strings.TrimSpace(query))
	var matches []PackageRef

	for _, reg := range regs {
		for _, pkg := range reg.Index.Packages {
			if q == "" ||
				strings.Contains(strings.ToLower(pkg.ID), q) ||
				strings.Contains(strings.ToLower(pkg.Name), q) ||
				strings.Contains(strings.ToLower(pkg.Description), q) ||
				strings.Contains(strings.ToLower(pkg.Author), q) {
				matches = append(matches, PackageRef{
					RegistryName: reg.Name,
					RegistryURL:  reg.URL,
					Package:      pkg,
				})
			}
		}
	}

	return matches, nil
}

func (c *Client) fetchOrCachedIndex(ctx context.Context, rawURL string) (Index, error) {
	if strings.HasPrefix(rawURL, "git+") {
		return c.fetchOrCachedGitIndex(ctx, rawURL)
	}

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

// SetInstaller configures the installer delegate called by Install.
func (c *Client) SetInstaller(installer ContentInstaller) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.installer = installer
}

// Install downloads, verifies, and installs a package from a PackageRef.
func (c *Client) Install(ctx context.Context, ref PackageRef, onConflict string) (content.Manifest, error) {
	if ref.Package.Download == "" {
		return content.Manifest{}, errors.New("package download URL is empty")
	}
	if ref.Package.SHA256 == "" {
		return content.Manifest{}, errors.New("package sha256 checksum is empty")
	}

	packagesDir := filepath.Join(c.cacheDir, "packages")
	cachedPkgPath := filepath.Join(packagesDir, strings.ToLower(ref.Package.SHA256)+".lrpgpack")

	var pkgFile *os.File

	if fi, statErr := os.Stat(cachedPkgPath); statErr == nil && fi.Size() > 0 {
		f, openErr := os.Open(cachedPkgPath)
		if openErr == nil {
			h := sha256.New()
			if _, copyErr := io.Copy(h, f); copyErr == nil {
				computed := hex.EncodeToString(h.Sum(nil))
				if strings.EqualFold(computed, ref.Package.SHA256) {
					_, _ = f.Seek(0, io.SeekStart)
					pkgFile = f
				}
			}
			if pkgFile == nil {
				_ = f.Close()
				_ = os.Remove(cachedPkgPath)
			}
		}
	}

	if pkgFile == nil {
		downloadURL := ref.Package.Download
		if ref.RegistryURL != "" {
			if base, parseErr := url.Parse(ref.RegistryURL); parseErr == nil {
				if resolved, resolveErr := base.Parse(downloadURL); resolveErr == nil {
					downloadURL = resolved.String()
				}
			}
		}

		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
		if reqErr != nil {
			return content.Manifest{}, fmt.Errorf("create download request: %w", reqErr)
		}

		client := c.getHTTPClient()
		resp, doErr := client.Do(req)
		if doErr != nil {
			return content.Manifest{}, fmt.Errorf("download package: %w", doErr)
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return content.Manifest{}, fmt.Errorf("download package failed with status %d", resp.StatusCode)
		}

		if err := os.MkdirAll(packagesDir, 0755); err != nil {
			return content.Manifest{}, fmt.Errorf("create packages cache dir: %w", err)
		}

		tmpFile, err := os.CreateTemp(packagesDir, ".download-*")
		if err != nil {
			return content.Manifest{}, fmt.Errorf("create temp package file: %w", err)
		}
		tmpName := tmpFile.Name()
		defer func() {
			if tmpFile != nil {
				_ = tmpFile.Close()
				_ = os.Remove(tmpName)
			}
		}()

		hasher := sha256.New()
		tee := io.TeeReader(resp.Body, hasher)
		if _, err := io.Copy(tmpFile, tee); err != nil {
			return content.Manifest{}, fmt.Errorf("stream download: %w", err)
		}

		computedSHA := hex.EncodeToString(hasher.Sum(nil))
		if !strings.EqualFold(computedSHA, ref.Package.SHA256) {
			return content.Manifest{}, fmt.Errorf("checksum mismatch: got %s, want %s", computedSHA, ref.Package.SHA256)
		}

		_ = tmpFile.Close()
		tmpFile = nil

		if err := os.Rename(tmpName, cachedPkgPath); err != nil {
			return content.Manifest{}, fmt.Errorf("cache package file: %w", err)
		}

		f, openErr := os.Open(cachedPkgPath)
		if openErr != nil {
			return content.Manifest{}, fmt.Errorf("open cached package file: %w", openErr)
		}
		pkgFile = f
	}

	defer pkgFile.Close()

	c.mu.RLock()
	installer := c.installer
	c.mu.RUnlock()

	if installer != nil {
		return installer(ctx, pkgFile, onConflict)
	}

	stagingDir, err := os.MkdirTemp(c.cacheDir, ".default-install-*")
	if err != nil {
		return content.Manifest{}, fmt.Errorf("create staging dir: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(stagingDir)
	}()

	m, _, err := content.Unpack(pkgFile, stagingDir)
	if err != nil {
		return content.Manifest{}, fmt.Errorf("unpack package: %w", err)
	}

	return m, nil
}

func (c *Client) fetchOrCachedGitIndex(ctx context.Context, rawURL string) (Index, error) {
	gitURL := strings.TrimPrefix(rawURL, "git+")
	key := c.cacheKey(rawURL)
	targetDir := filepath.Join(c.cacheDir, "registries", "git-"+key)
	indexPath := filepath.Join(targetDir, "index.json")

	// Check if already cloned
	if fi, err := os.Stat(filepath.Join(targetDir, ".git")); err == nil && fi.IsDir() {
		fetchCmd := exec.CommandContext(ctx, "git", "-C", targetDir, "fetch", "--depth", "1", "origin")
		_ = fetchCmd.Run()
		resetCmd := exec.CommandContext(ctx, "git", "-C", targetDir, "reset", "--hard", "FETCH_HEAD")
		_ = resetCmd.Run()
	} else {
		_ = os.MkdirAll(filepath.Dir(targetDir), 0755)
		_ = os.RemoveAll(targetDir)
		cloneCmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", gitURL, targetDir)
		if out, err := cloneCmd.CombinedOutput(); err != nil {
			if data, readErr := os.ReadFile(indexPath); readErr == nil {
				return ParseIndex(data)
			}
			return Index{}, fmt.Errorf("git clone %s: %w (%s)", gitURL, err, string(out))
		}
	}

	data, err := os.ReadFile(indexPath)
	if err != nil {
		return Index{}, fmt.Errorf("read index.json in git registry %s: %w", gitURL, err)
	}

	return ParseIndex(data)
}

// SetInstalledLister configures the function that enumerates currently installed packages for Update.
func (c *Client) SetInstalledLister(lister InstalledLister) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.installedLister = lister
}

// Update checks installed content packages against configured registries
// and returns PackageRef entries that offer a newer semantic version.
func (c *Client) Update(ctx context.Context) ([]PackageRef, error) {
	c.mu.RLock()
	lister := c.installedLister
	c.mu.RUnlock()

	var installed []content.Manifest
	if lister != nil {
		var err error
		installed, err = lister(ctx)
		if err != nil {
			return nil, fmt.Errorf("list installed content: %w", err)
		}
	}

	if len(installed) == 0 {
		return []PackageRef{}, nil
	}

	allPackages, err := c.Search(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("search registries: %w", err)
	}

	var updates []PackageRef

	for _, inst := range installed {
		var bestCandidate *PackageRef

		for i := range allPackages {
			cand := &allPackages[i]
			if cand.Package.Type != inst.Type || cand.Package.ID != inst.ID {
				continue
			}

			vCand := core.CanonicalSemver(cand.Package.Version)
			vInst := core.CanonicalSemver(inst.Version)

			if semver.Compare(vCand, vInst) > 0 {
				if bestCandidate == nil || semver.Compare(vCand, core.CanonicalSemver(bestCandidate.Package.Version)) > 0 {
					bestCandidate = cand
				}
			}
		}

		if bestCandidate != nil {
			updates = append(updates, *bestCandidate)
		}
	}

	return updates, nil
}

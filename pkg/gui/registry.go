// The content registry and the package import/export path.

package gui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/content"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/pathutil"
	"github.com/darkliquid/localrpg/pkg/registry"
	"gopkg.in/yaml.v3"
)

// GenerateAssetPreview renders banner or icon image bytes in memory and returns
// them with a MIME type detected from the bytes.
func (s *Service) GenerateAssetPreview(ctx context.Context, req GenerateAssetPreviewRequestDTO) ([]byte, string, error) {
	imgBytes, failure := s.generateImage(ctx, req.Kind, buildAssetPrompt(req.Kind, req.Name, req.Description, req.ArtStyle, req.Genre), usageScope{token: req.UsageToken})
	if failure != nil {
		return nil, "", failure
	}
	return imgBytes, imageContentType(imgBytes), nil
}

// ExportContent packs a world or system directory as a .lrpgpack archive and writes it to w.
func (s *Service) ExportContent(ctx context.Context, typ, id string, w io.Writer) (content.Manifest, error) {
	if err := pathutil.ValidateID(id); err != nil {
		return content.Manifest{}, fmt.Errorf("invalid content ID: %w", err)
	}
	var dir string
	switch typ {
	case "world":
		dir = s.resolver.WorldDir(id)
	case "system":
		dir = s.resolver.SystemDir(id)
	default:
		return content.Manifest{}, fmt.Errorf("invalid content type %q: must be 'world' or 'system'", typ)
	}

	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return content.Manifest{}, fmt.Errorf("%w: content %s %q", ErrContentNotFound, typ, id)
	}

	return content.Pack(dir, typ, content.ManifestMeta{}, w)
}

// ErrContentConflict reports an attempt to import content that already exists under refuse mode.
var ErrContentConflict = errors.New("content already exists")

// ErrContentNotFound reports content that is not in the registry.
var ErrContentNotFound = errors.New("content not found")

// ImportContent unpacks a package archive from r into a staging directory, validates its
// structure, resolves conflicts according to onConflict ("refuse", "rename", "overwrite"),
// and atomically moves it into the content directory.
func (s *Service) ImportContent(ctx context.Context, r io.Reader, onConflict string) (ImportResultDTO, error) {
	return s.ImportContentWithOptions(ctx, r, ImportContentOptions{ConflictMode: onConflict})
}

// ImportContentWithOptions unpacks a package archive from r with type validation and
// conflict handling. The phases are its structure: the request is checked, the
// package is unpacked and verified, the staged content is validated, the conflict
// mode decides where it lands, and the install is a swap or a move.
func (s *Service) ImportContentWithOptions(ctx context.Context, r io.Reader, opts ImportContentOptions) (ImportResultDTO, error) {
	onConflict, err := normalizeConflictMode(opts.ConflictMode)
	if err != nil {
		return ImportResultDTO{}, err
	}

	stagingDir, err := os.MkdirTemp("", "lrpg-import-staging-*")
	if err != nil {
		return ImportResultDTO{}, fmt.Errorf("create staging directory: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(stagingDir)
	}()

	m, sigBytes, err := content.Unpack(r, stagingDir)
	if err != nil {
		return ImportResultDTO{}, fmt.Errorf("unpack content: %w", err)
	}
	if err := pathutil.ValidateID(m.ID); err != nil {
		return ImportResultDTO{}, fmt.Errorf("invalid package content ID %q: %w", m.ID, err)
	}
	if err := validateExpectedType(m, opts.ExpectedType); err != nil {
		return ImportResultDTO{}, err
	}
	if err := validatePackageFilename(m, opts.Filename); err != nil {
		return ImportResultDTO{}, err
	}

	trust, err := s.verifyPackage(m, sigBytes)
	if err != nil {
		return ImportResultDTO{Trust: trust}, err
	}

	wm, sm, err := loadStagedManifest(stagingDir, m)
	if err != nil {
		return ImportResultDTO{}, err
	}

	// Read from the staged directory, before the install renames it away.
	hasScript := stagedHasScript(stagingDir)

	targetDir, finalID, action, err := s.resolveContentConflict(m, wm, sm, stagingDir, onConflict)
	if err != nil {
		return ImportResultDTO{}, err
	}

	if err := installStagedContent(stagingDir, targetDir, action); err != nil {
		return ImportResultDTO{}, err
	}

	return importResult(m, finalID, action, hasScript, trust), nil
}

// normalizeConflictMode is the conflict mode an import runs under. It defaults to
// refuse and rejects anything that is not one of the three modes.
func normalizeConflictMode(mode string) (string, error) {
	if mode == "" {
		mode = "refuse"
	}
	if mode != "refuse" && mode != "rename" && mode != "overwrite" {
		return "", fmt.Errorf("invalid on_conflict mode %q: must be refuse, rename, or overwrite", mode)
	}
	return mode, nil
}

// validateExpectedType checks the package against the content type the caller asked
// for, when it asked for one.
func validateExpectedType(m content.Manifest, expectedType string) error {
	if expectedType == "" {
		return nil
	}
	expected := strings.ToLower(strings.TrimSpace(expectedType))
	if expected != "world" && expected != "system" {
		return fmt.Errorf("invalid expected content type %q: must be 'world' or 'system'", expected)
	}
	if m.Type != expected {
		return fmt.Errorf("package contains %s %q, cannot import as %s", m.Type, m.ID, expected)
	}
	return nil
}

// validatePackageFilename checks that a declared filename's extension agrees with
// what the package actually holds. The universal .lrpgpack is allowed for both.
func validatePackageFilename(m content.Manifest, filename string) error {
	if filename == "" {
		return nil
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".lrpgworld":
		if m.Type != "world" {
			return fmt.Errorf("package filename %q has extension .lrpgworld but package contains %s %q", filename, m.Type, m.ID)
		}
	case ".lrpgsystem":
		if m.Type != "system" {
			return fmt.Errorf("package filename %q has extension .lrpgsystem but package contains %s %q", filename, m.Type, m.ID)
		}
	case ".lrpgpack":
		// Universal package format, allowed for both
	}
	return nil
}

// verifyPackage checks a package's signature against the trusted publishers the
// configuration names. The trust it decided is returned either way, because an
// invalid package is reported with it.
func (s *Service) verifyPackage(m content.Manifest, sigBytes []byte) (content.Trust, error) {
	var trustedPublishers map[string]string
	if s.configMgr != nil {
		if cfg := s.configMgr.Get(); cfg != nil {
			trustedPublishers = cfg.Publishers
		}
	}

	trust, verifyErr := content.Verify(m, sigBytes, trustedPublishers)
	if verifyErr != nil || trust.State == "invalid" {
		return trust, fmt.Errorf("package signature is invalid: %w", verifyErr)
	}
	return trust, nil
}

// loadStagedManifest validates a staged package's own manifest and returns it, so
// the conflict mode can rewrite its id when the package installs under a new one.
func loadStagedManifest(stagingDir string, m content.Manifest) (*core.WorldManifest, *core.SystemManifest, error) {
	switch m.Type {
	case "world":
		wm, err := core.LoadWorldManifest(filepath.Join(stagingDir, "world.yaml"))
		if err != nil {
			return nil, nil, fmt.Errorf("invalid world package: missing or invalid world.yaml: %w", err)
		}
		if wm.ID != m.ID {
			return nil, nil, fmt.Errorf("world.yaml id %q does not match package id %q", wm.ID, m.ID)
		}
		return wm, nil, nil
	case "system":
		sm, err := core.LoadSystemManifest(filepath.Join(stagingDir, "system.yaml"))
		if err != nil {
			return nil, nil, fmt.Errorf("invalid system package: missing or invalid system.yaml: %w", err)
		}
		if sm.ID != m.ID {
			return nil, nil, fmt.Errorf("system.yaml id %q does not match package id %q", sm.ID, m.ID)
		}
		return nil, sm, nil
	default:
		return nil, nil, fmt.Errorf("invalid content type %q", m.Type)
	}
}

// stagedHasScript reports whether a staged package carries any executable script,
// which the result surfaces so the caller knows what it just installed.
func stagedHasScript(stagingDir string) bool {
	hasScript := false
	_ = filepath.WalkDir(stagingDir, func(_ string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".js") {
			hasScript = true
			return fs.SkipAll
		}
		return nil
	})
	return hasScript
}

// resolveContentConflict decides where a package lands. An id that is free installs
// as it is; otherwise the mode refuses the import, installs under the lowest free
// suffix (rewriting the staged manifest to match), or replaces what is there.
func (s *Service) resolveContentConflict(m content.Manifest, wm *core.WorldManifest, sm *core.SystemManifest, stagingDir, mode string) (dir, id, action string, err error) {
	dir = s.contentTargetDir(m.Type, m.ID)
	id = m.ID
	action = "installed"

	fi, statErr := os.Stat(dir)
	if statErr != nil || !fi.IsDir() {
		return dir, id, action, nil
	}

	switch mode {
	case "refuse":
		return "", "", "", fmt.Errorf("%w: content %s %q already exists", ErrContentConflict, m.Type, m.ID)
	case "rename":
		id = s.freeContentID(m.Type, m.ID)
		if err := rewriteStagedManifestID(stagingDir, m, wm, sm, id); err != nil {
			return "", "", "", err
		}
		dir = s.contentTargetDir(m.Type, id)
		action = "renamed"
	case "overwrite":
		action = "overwritten"
	}
	return dir, id, action, nil
}

// contentTargetDir is the directory a package of this kind and id installs into.
func (s *Service) contentTargetDir(kind, id string) string {
	cleanID := filepath.Base(pathutil.SanitizeID(id))
	if kind == "world" {
		return filepath.Join(s.resolver.WorldsDir(), cleanID)
	}
	return filepath.Join(s.resolver.SystemsDir(), cleanID)
}

// freeContentID is id with the lowest numeric suffix that is not already taken.
func (s *Service) freeContentID(kind, id string) string {
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s-%d", id, n)
		if _, err := os.Stat(s.contentTargetDir(kind, candidate)); os.IsNotExist(err) {
			return candidate
		}
	}
}

// rewriteStagedManifestID rewrites the staged manifest so the installed package
// names the id it will actually have.
func rewriteStagedManifestID(stagingDir string, m content.Manifest, wm *core.WorldManifest, sm *core.SystemManifest, id string) error {
	if m.Type == "world" {
		wm.ID = id
		data, err := yaml.Marshal(wm)
		if err != nil {
			return fmt.Errorf("marshal updated world manifest: %w", err)
		}
		if err := os.WriteFile(filepath.Join(stagingDir, "world.yaml"), data, 0644); err != nil {
			return fmt.Errorf("write updated world manifest: %w", err)
		}
		return nil
	}
	if m.Type == "system" {
		sm.ID = id
		data, err := yaml.Marshal(sm)
		if err != nil {
			return fmt.Errorf("marshal updated system manifest: %w", err)
		}
		if err := os.WriteFile(filepath.Join(stagingDir, "system.yaml"), data, 0644); err != nil {
			return fmt.Errorf("write updated system manifest: %w", err)
		}
	}
	return nil
}

// installStagedContent moves a staged package into place. The overwrite mode swaps
// through a backup directory, so a failure leaves the previous content where it was
// rather than deleting it.
func installStagedContent(stagingDir, targetDir, action string) error {
	// Install: ensure parent dir exists
	parentDir := filepath.Dir(targetDir)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("create content parent directory: %w", err)
	}

	if action == "overwritten" {
		// Temporary backup directory created directly under parentDir for atomic rollback.
		// lgtm[go/path-injection]
		backupDir, err := os.MkdirTemp(parentDir, ".backup-*")
		if err != nil {
			return fmt.Errorf("create backup directory: %w", err)
		}
		_ = os.RemoveAll(backupDir)
		if err := os.Rename(targetDir, backupDir); err != nil {
			return fmt.Errorf("backup existing content: %w", err)
		}
		swapSuccess := false
		defer func() {
			if !swapSuccess {
				_ = os.RemoveAll(targetDir)
				_ = os.Rename(backupDir, targetDir)
			} else {
				_ = os.RemoveAll(backupDir)
			}
		}()

		if err := os.Rename(stagingDir, targetDir); err != nil {
			return fmt.Errorf("install content into %q: %w", targetDir, err)
		}
		swapSuccess = true
		return nil
	}

	if err := os.Rename(stagingDir, targetDir); err != nil {
		return fmt.Errorf("install content into %q: %w", targetDir, err)
	}
	return nil
}

// importResult is what the caller is told about a package that installed.
func importResult(m content.Manifest, id, action string, hasScript bool, trust content.Trust) ImportResultDTO {
	return ImportResultDTO{
		ID:          id,
		Name:        m.Name,
		Version:     m.Version,
		Type:        m.Type,
		Author:      m.Author,
		License:     m.License,
		Description: m.Description,
		FileCount:   len(m.Files),
		HasScript:   hasScript,
		Action:      action,
		Trust:       trust,
	}
}

func (s *Service) registryClient() *registry.Client {
	var regCfg config.RegistriesConfig
	if s.configMgr != nil {
		if cfg := s.configMgr.Get(); cfg != nil {
			regCfg = cfg.Registries
		}
	}
	cacheDir := s.resolver.CacheDir()
	client := registry.NewClient(regCfg, cacheDir)

	client.SetInstaller(func(ctx context.Context, r io.Reader, conflictMode string) (content.Manifest, error) {
		res, err := s.ImportContent(ctx, r, conflictMode)
		if err != nil {
			return content.Manifest{}, err
		}
		return content.Manifest{
			ID:          res.ID,
			Name:        res.Name,
			Version:     res.Version,
			Type:        res.Type,
			Description: res.Description,
		}, nil
	})

	client.SetInstalledLister(func(ctx context.Context) ([]content.Manifest, error) {
		var manifests []content.Manifest

		// Scan worlds
		worldsDir := s.resolver.WorldsDir()
		if entries, err := os.ReadDir(worldsDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					worldPath := filepath.Join(s.resolver.WorldDir(entry.Name()), "world.yaml")
					if wm, err := core.LoadWorldManifest(worldPath); err == nil && wm != nil {
						manifests = append(manifests, content.Manifest{
							ID:      wm.ID,
							Name:    wm.Name,
							Version: wm.Version,
							Type:    "world",
						})
					}
				}
			}
		}

		// Scan systems
		systemsDir := s.resolver.SystemsDir()
		if entries, err := os.ReadDir(systemsDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					sysPath := filepath.Join(s.resolver.SystemDir(entry.Name()), "system.yaml")
					if sm, err := core.LoadSystemManifest(sysPath); err == nil && sm != nil {
						manifests = append(manifests, content.Manifest{
							ID:      sm.ID,
							Name:    sm.Name,
							Version: sm.Version,
							Type:    "system",
						})
					}
				}
			}
		}

		return manifests, nil
	})

	return client
}

// HandleRegistrySearch handles GET /api/registry/search?q=...
func (s *Service) HandleRegistrySearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query().Get("q")
	client := s.registryClient()
	results, err := client.Search(r.Context(), q)
	if err != nil {
		s.writeAPIError(w, err)
		return
	}
	if results == nil {
		results = []registry.PackageRef{}
	}
	writeJSON(w, results)
}

// HandleRegistryInstall handles POST /api/registry/install
func (s *Service) HandleRegistryInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req RegistryInstallRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.OnConflict == "" {
		req.OnConflict = "refuse"
	}

	client := s.registryClient()
	var lastResult ImportResultDTO
	client.SetInstaller(func(ctx context.Context, r io.Reader, conflictMode string) (content.Manifest, error) {
		res, err := s.ImportContent(ctx, r, conflictMode)
		if err != nil {
			return content.Manifest{}, err
		}
		lastResult = res
		return content.Manifest{
			ID:          res.ID,
			Name:        res.Name,
			Version:     res.Version,
			Type:        res.Type,
			Description: res.Description,
		}, nil
	})

	m, err := client.Install(r.Context(), req.Ref, req.OnConflict)
	if err != nil {
		if errors.Is(err, ErrContentConflict) {
			s.writeAPIError(w, err)
			return
		}
		s.writeAPIError(w, invalid(err))
		return
	}

	if lastResult.ID == "" {
		lastResult = ImportResultDTO{
			ID:          m.ID,
			Name:        m.Name,
			Version:     m.Version,
			Type:        m.Type,
			Description: m.Description,
			Action:      "installed",
		}
	}
	writeJSON(w, lastResult)
}

// HandleRegistryUpdates handles GET /api/registry/updates
func (s *Service) HandleRegistryUpdates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	client := s.registryClient()
	updates, err := client.Update(r.Context())
	if err != nil {
		s.writeAPIError(w, err)
		return
	}
	if updates == nil {
		updates = []registry.PackageRef{}
	}
	writeJSON(w, updates)
}

// RegistrySources reports every configured registry and its fetch state, so the
// registry view can show where packages come from and explain an empty source.
func (s *Service) RegistrySources(ctx context.Context) ([]RegistrySourceDTO, error) {
	statuses := s.registryClient().Sources(ctx)
	out := make([]RegistrySourceDTO, 0, len(statuses))
	for _, status := range statuses {
		out = append(out, RegistrySourceDTO{
			URL:          status.URL,
			Name:         status.Name,
			PackageCount: status.PackageCount,
			Error:        status.Error,
		})
	}
	return out, nil
}

// AddRegistrySource normalises a registry URL, refuses a duplicate, and persists
// it to the configuration.
func (s *Service) AddRegistrySource(_ context.Context, rawURL string) (RegistrySourceDTO, error) {
	normalized, err := registry.NormalizeSource(rawURL)
	if err != nil {
		return RegistrySourceDTO{}, err
	}
	cfg := s.configMgr.Get()
	if cfg == nil {
		return RegistrySourceDTO{}, fmt.Errorf("no configuration is loaded")
	}
	for _, existing := range cfg.Registries.URLs {
		if existing == normalized {
			return RegistrySourceDTO{}, fmt.Errorf("%w: %s", ErrRegistrySourceExists, normalized)
		}
	}
	cfg.Registries.URLs = append(append([]string{}, cfg.Registries.URLs...), normalized)
	if err := s.configMgr.Save(cfg); err != nil {
		return RegistrySourceDTO{}, fmt.Errorf("save config: %w", err)
	}
	return RegistrySourceDTO{URL: normalized}, nil
}

// RemoveRegistrySource deletes one configured registry URL from the configuration.
func (s *Service) RemoveRegistrySource(_ context.Context, rawURL string) error {
	normalized, err := registry.NormalizeSource(rawURL)
	if err != nil {
		return err
	}
	cfg := s.configMgr.Get()
	if cfg == nil {
		return fmt.Errorf("no configuration is loaded")
	}
	remaining := make([]string, 0, len(cfg.Registries.URLs))
	found := false
	for _, existing := range cfg.Registries.URLs {
		if existing == normalized {
			found = true
			continue
		}
		remaining = append(remaining, existing)
	}
	if !found {
		return fmt.Errorf("%w: %s", ErrRegistrySourceNotFound, normalized)
	}
	cfg.Registries.URLs = remaining
	if err := s.configMgr.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// HandleRegistrySources handles GET, POST, and DELETE on /api/registry/sources.
func (s *Service) HandleRegistrySources(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sources, err := s.RegistrySources(r.Context())
		if err != nil {
			s.writeAPIError(w, err)
			return
		}
		if sources == nil {
			sources = []RegistrySourceDTO{}
		}
		writeJSON(w, sources)

	case http.MethodPost:
		var req struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		source, err := s.AddRegistrySource(r.Context(), req.URL)
		if errors.Is(err, ErrRegistrySourceExists) {
			s.writeAPIError(w, err)
			return
		}
		if err != nil {
			s.writeAPIError(w, invalid(err))
			return
		}
		writeJSONStatus(w, http.StatusCreated, source)

	case http.MethodDelete:
		if err := s.RemoveRegistrySource(r.Context(), r.URL.Query().Get("url")); err != nil {
			if errors.Is(err, ErrRegistrySourceNotFound) {
				s.writeAPIError(w, err)
				return
			}
			s.writeAPIError(w, invalid(err))
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

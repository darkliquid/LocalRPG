# CodeQL Security and Path Sanitization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remediate all 122 GitHub CodeQL scan alerts (112 path injection issues, plus 10 non-path issues covering Zip Slip, integer conversion, allocation overflow, unsafe quoting, and DOM XSS) through centralized path validation, perimeter defense, and targeted fixes.

**Architecture:** A new [`pkg/pathutil`](file:///home/darkliquid/Projects/LocalRPG/pkg/pathutil) package provides single-component ID validation (`ValidateID`), safe kebab-case sanitization fallback (`SanitizeID`), path directory containment checking (`ResolveSafeChild`), and user-configured path cleaning (`ValidateUserPath`). The HTTP router ([`pkg/gui/server.go`](file:///home/darkliquid/Projects/LocalRPG/pkg/gui/server.go)) and service methods ([`pkg/gui/service.go`](file:///home/darkliquid/Projects/LocalRPG/pkg/gui/service.go)) validate parameters at the perimeter and return `400 Bad Request` on traversal. [`pkg/core/types.go`](file:///home/darkliquid/Projects/LocalRPG/pkg/core/types.go), engine timeline/portrait writers, storage pools, archive extraction, integer conversions, trace allocations, and frontend image bindings are hardened against abuse.

**Tech Stack:** Go 1.27.1, React 19, TypeScript, standard library (`path/filepath`, `strings`, `regexp`, `os`), modernc.org/sqlite.

---

### File Map

| File | Action | Responsibility |
| :--- | :--- | :--- |
| `pkg/pathutil/pathutil.go` | Create | Centralized ID validation, safe child resolution, path sanitization |
| `pkg/pathutil/pathutil_test.go` | Create | Unit tests for path validation, sanitization, containment, user paths |
| `pkg/core/types.go` | Modify | Sanitize IDs in `SystemDir`, `WorldDir`, `GameDir` against directory escape |
| `pkg/core/types_test.go` | Modify/Create | Verify `PathResolver` directory containment |
| `pkg/gui/server.go` | Modify | Validate route path parameters at HTTP boundary; return 400 on traversal |
| `pkg/gui/server_test.go` | Modify | Integration tests for HTTP route parameter path traversal rejections |
| `pkg/gui/service.go` | Modify | Validate IDs in game/world/system/entity/asset operations and `SaveSettings` |
| `pkg/gui/service_test.go` | Modify | Test service methods reject traversal IDs |
| `pkg/engine/game.go` | Modify | Validate IDs and sanitize template entity filenames in `InitGame` |
| `pkg/engine/timeline.go` | Modify | Sanitize entity IDs in `writeEntities` |
| `pkg/engine/portrait_worker.go` | Modify | Sanitize character IDs in portrait generation and cleanup |
| `pkg/engine/startlocation.go` | Modify | Sanitize opening scene entity filename |
| `pkg/storage/game.go` | Modify | Sanitize `gameID` in `OpenGameStore` and legacy retirement |
| `pkg/storage/sync.go` | Modify | Ensure `Sync` and `SyncFile` remain contained in entities directory |
| `pkg/export/web.go` | Modify | Validate output path with `pathutil.ValidateUserPath` |
| `pkg/export/video.go` | Modify | Validate output path with `pathutil.ValidateUserPath` |
| `pkg/gui/export.go` | Modify | Sanitize default export filenames and validate custom output paths |
| `pkg/provider/ttsfishaudio/client.go` | Modify | Validate reference audio file path with `pathutil.ValidateUserPath` |
| `pkg/models/manager.go` | Modify | Fix Zip Slip with `filepath.Rel` directory containment check |
| `pkg/models/manager_test.go` | Modify | Test archive extraction rejects traversal paths |
| `pkg/media/pcm.go` | Modify | Add bounds check on sample rate before `uint32` conversion |
| `pkg/media/opus/opus.go` | Modify | Add bounds check on sample rate before `uint32` conversion |
| `pkg/trace/trace.go` | Modify | Remove arithmetic from map allocation hint |
| `pkg/trace/stderr.go` | Modify | Remove arithmetic from map allocation hint |
| `pkg/provider/oracle/provider.go` | Modify | Sanitize quotes in player action interpolation |
| `frontend/src/utils/security.ts` | Create | Safe image preview URL validation |
| `frontend/src/components/WorldsStudio.tsx` | Modify | Wrap object preview URLs in `safeImagePreview` |
| `frontend/src/components/launcher/NewCampaignModal.tsx` | Modify | Wrap object preview URLs in `safeImagePreview` |

---

### Task 1: Create `pkg/pathutil` Package & Tests

**Files:**
- Create: `pkg/pathutil/pathutil.go`
- Create: `pkg/pathutil/pathutil_test.go`

- [ ] **Step 1: Write the failing unit tests for `pkg/pathutil`**

```go
package pathutil_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/pathutil"
)

func TestValidateID(t *testing.T) {
	valid := []string{
		"game-1",
		"aldon_harbour",
		"player",
		"guard-kael-12",
		"d20",
		"System_01",
	}
	for _, id := range valid {
		if err := pathutil.ValidateID(id); err != nil {
			t.Errorf("ValidateID(%q) unexpected error: %v", id, err)
		}
	}

	invalid := []string{
		"",
		"   ",
		"../etc/passwd",
		"..",
		".",
		"/root",
		"C:\\Windows",
		"foo/bar",
		"foo\\bar",
		"foo\x00bar",
		"entity with spaces",
		"name*with?wildcards",
		"hello:world",
	}
	for _, id := range invalid {
		if err := pathutil.ValidateID(id); err == nil {
			t.Errorf("ValidateID(%q) expected error, got nil", id)
		}
	}
}

func TestSanitizeID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "hello-world"},
		{"../etc/passwd", "etc-passwd"},
		{"foo/bar/baz", "foo-bar-baz"},
		{"--test--", "test"},
		{"", "unnamed"},
		{"   ", "unnamed"},
		{"Valid-ID_123", "valid-id_123"},
	}
	for _, tc := range tests {
		got := pathutil.SanitizeID(tc.input)
		if got != tc.expected {
			t.Errorf("SanitizeID(%q) = %q; want %q", tc.input, got, tc.expected)
		}
		if err := pathutil.ValidateID(got); err != nil {
			t.Errorf("SanitizeID output %q fails ValidateID: %v", got, err)
		}
	}
}

func TestResolveSafeChild(t *testing.T) {
	tmp := t.TempDir()

	validChild, err := pathutil.ResolveSafeChild(tmp, "sub/file.txt")
	if err != nil {
		t.Fatalf("ResolveSafeChild unexpected error: %v", err)
	}
	if !strings.HasPrefix(validChild, filepath.Clean(tmp)) {
		t.Errorf("ResolveSafeChild returned path %q outside base %q", validChild, tmp)
	}

	escapes := []string{
		"../outside.txt",
		"sub/../../outside.txt",
		"/etc/passwd",
		"..",
	}
	for _, rel := range escapes {
		if _, err := pathutil.ResolveSafeChild(tmp, rel); err == nil {
			t.Errorf("ResolveSafeChild(%q, %q) expected error, got nil", tmp, rel)
		}
	}
}

func TestValidateUserPath(t *testing.T) {
	tmp := t.TempDir()
	cleaned, err := pathutil.ValidateUserPath(filepath.Join(tmp, "custom", "export.html"))
	if err != nil {
		t.Fatalf("ValidateUserPath unexpected error: %v", err)
	}
	if cleaned == "" {
		t.Errorf("ValidateUserPath returned empty string")
	}

	invalid := []string{
		"",
		"   ",
		"path\x00with-null",
	}
	for _, p := range invalid {
		if _, err := pathutil.ValidateUserPath(p); err == nil {
			t.Errorf("ValidateUserPath(%q) expected error, got nil", p)
		}
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test -v ./pkg/pathutil/...`  
Expected: FAIL ("cannot find package" or "undefined pathutil")

- [ ] **Step 3: Implement `pkg/pathutil/pathutil.go`**

```go
package pathutil

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// ErrEmptyID is returned when an identifier is empty or whitespace.
	ErrEmptyID = errors.New("identifier cannot be empty")
	// ErrPathTraversal is returned when an identifier contains directory traversal tokens.
	ErrPathTraversal = errors.New("identifier cannot contain path separators or traversal sequences")
	// ErrInvalidID is returned when an identifier contains illegal characters.
	ErrInvalidID = errors.New("identifier contains invalid characters")

	validIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

// ValidateID ensures an identifier consists strictly of alphanumeric characters,
// dashes, or underscores, and contains no directory separators or traversal tokens.
func ValidateID(id string) error {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return ErrEmptyID
	}
	if strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") || strings.Contains(trimmed, "..") || strings.ContainsRune(trimmed, 0) {
		return ErrPathTraversal
	}
	if !validIDPattern.MatchString(trimmed) {
		return fmt.Errorf("%w: %q", ErrInvalidID, trimmed)
	}
	return nil
}

// SanitizeID normalizes an input string into a safe identifier,
// guaranteeing that path separators and traversal tokens are stripped.
func SanitizeID(id string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return "unnamed"
	}
	var buf strings.Builder
	precededBySeparator := true
	for _, r := range strings.ToLower(trimmed) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			buf.WriteRune(r)
			precededBySeparator = false
		case r == '_':
			buf.WriteRune('_')
			precededBySeparator = false
		case r == ' ' || r == '-' || r == '/' || r == '\\' || r == '.':
			if buf.Len() > 0 && !precededBySeparator {
				buf.WriteRune('-')
				precededBySeparator = true
			}
		}
	}
	res := strings.Trim(buf.String(), "-_")
	if res == "" {
		return "unnamed"
	}
	return res
}

// ResolveSafeChild resolves relativePath under baseDir and verifies that the
// resulting path remains strictly contained within baseDir.
func ResolveSafeChild(baseDir, relativePath string) (string, error) {
	cleanBase := filepath.Clean(baseDir)
	target := filepath.Clean(filepath.Join(cleanBase, relativePath))

	rel, err := filepath.Rel(cleanBase, target)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return "", fmt.Errorf("path %q escapes directory %q", relativePath, baseDir)
	}
	return target, nil
}

// ValidateUserPath normalizes an intentional user-provided path, rejecting null
// bytes and control characters while preserving intended directory locations.
func ValidateUserPath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("path cannot be empty")
	}
	if strings.ContainsRune(trimmed, 0) {
		return "", errors.New("path contains null byte")
	}
	return filepath.Clean(trimmed), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./pkg/pathutil/...`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/pathutil/pathutil.go pkg/pathutil/pathutil_test.go
git commit -m "feat(pathutil): add identifier validation and path containment helpers"
```

---

### Task 2: Harden Core `PathResolver` Against Directory Escape

**Files:**
- Modify: `pkg/core/types.go:68-93`
- Test: `pkg/core/types_test.go`

- [ ] **Step 1: Write test verifying `PathResolver` directory containment**

In `pkg/core/types_test.go` (create or append):
```go
package core_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestPathResolverSanitizesIDs(t *testing.T) {
	resolver := core.NewPathResolver("/app/base", "", "", "", "")

	malicious := "../../etc/passwd"
	gameDir := resolver.GameDir(malicious)
	if !strings.HasPrefix(gameDir, resolver.GamesDir()) {
		t.Errorf("GameDir(%q) = %q, escaped GamesDir %q", malicious, gameDir, resolver.GamesDir())
	}
	if filepath.Base(gameDir) == "passwd" || strings.Contains(gameDir, "..") {
		t.Errorf("GameDir(%q) contains traversal or escaped name: %q", malicious, gameDir)
	}

	worldDir := resolver.WorldDir(malicious)
	if !strings.HasPrefix(worldDir, resolver.WorldsDir()) || strings.Contains(worldDir, "..") {
		t.Errorf("WorldDir(%q) escaped WorldsDir: %q", malicious, worldDir)
	}

	sysDir := resolver.SystemDir(malicious)
	if !strings.HasPrefix(sysDir, resolver.SystemsDir()) || strings.Contains(sysDir, "..") {
		t.Errorf("SystemDir(%q) escaped SystemsDir: %q", malicious, sysDir)
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test -run TestPathResolverSanitizesIDs ./pkg/core/...`  
Expected: FAIL (paths escape `GamesDir()`)

- [ ] **Step 3: Update `pkg/core/types.go`**

In `pkg/core/types.go`:
Import `"github.com/darkliquid/localrpg/pkg/pathutil"` and update lines 68-93:
```go
func (p *PathResolver) SystemDir(id string) string {
	return filepath.Join(p.SystemsDir(), pathutil.SanitizeID(id))
}

func (p *PathResolver) WorldDir(id string) string {
	return filepath.Join(p.WorldsDir(), pathutil.SanitizeID(id))
}

func (p *PathResolver) GameDir(id string) string {
	return filepath.Join(p.GamesDir(), pathutil.SanitizeID(id))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestPathResolverSanitizesIDs ./pkg/core/...`  
Expected: PASS

- [ ] **Step 5: Run all core tests**

Run: `go test -v ./pkg/core/...`  
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/core/types.go pkg/core/types_test.go
git commit -m "fix(core): sanitize IDs in PathResolver to prevent directory escape"
```

---

### Task 3: Perimeter Parameter Validation in HTTP Route Handlers

**Files:**
- Modify: `pkg/gui/server.go:244-300`, `pkg/gui/server.go:915-990`, `pkg/gui/server.go:849-880`
- Test: `pkg/gui/server_test.go`

- [ ] **Step 1: Write integration tests for route traversal rejection**

In `pkg/gui/server_test.go`, add:
```go
func TestRoutePathTraversalRejection(t *testing.T) {
	srv := newTestServer(t) // reuse existing test server setup helper

	maliciousPaths := []string{
		"/api/game/..%2F..%2Fetc/state",
		"/api/game/test-game/entity/..%2F..%2Fsecret",
		"/api/game/test-game/character/..%2F..%2Fportrait/portrait",
		"/api/world/..%2F..%2Fetc",
		"/api/world/test-world/entity/..%2F..%2Fsecret",
		"/api/system/..%2F..%2Fetc",
	}

	for _, path := range maliciousPaths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s returned code %d, expected 400 Bad Request", path, rec.Code)
		}
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test -run TestRoutePathTraversalRejection ./pkg/gui/...`  
Expected: FAIL (returns 404 or 500 instead of 400)

- [ ] **Step 3: Modify `pkg/gui/server.go` to validate path parameters**

Import `"github.com/darkliquid/localrpg/pkg/pathutil"`.

1. In `handleGameRoutes` (`pkg/gui/server.go`):
```go
func (s *Server) handleGameRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/game/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "missing game id", http.StatusBadRequest)
		return
	}
	gameID := parts[0]
	if err := pathutil.ValidateID(gameID); err != nil {
		http.Error(w, "invalid game id", http.StatusBadRequest)
		return
	}
...
	if len(parts) >= 3 && parts[1] == "entity" {
		entityID := parts[2]
		if err := pathutil.ValidateID(entityID); err != nil {
			http.Error(w, "invalid entity id", http.StatusBadRequest)
			return
		}
...
	if len(parts) >= 3 && parts[1] == "character" {
		charID := parts[2]
		if err := pathutil.ValidateID(charID); err != nil {
			http.Error(w, "invalid character id", http.StatusBadRequest)
			return
		}
```

2. In `handleWorldRoutes` (`pkg/gui/server.go`):
```go
func (s *Server) handleWorldRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/world/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "missing world id", http.StatusBadRequest)
		return
	}
	worldID := parts[0]
	if err := pathutil.ValidateID(worldID); err != nil {
		http.Error(w, "invalid world id", http.StatusBadRequest)
		return
	}
	if len(parts) >= 3 && parts[1] == "entity" {
		entityID := parts[2]
		if err := pathutil.ValidateID(entityID); err != nil {
			http.Error(w, "invalid entity id", http.StatusBadRequest)
			return
		}
```

3. In `handleSystemRoutes` (`pkg/gui/server.go`):
```go
func (s *Server) handleSystemRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/system/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "missing system id", http.StatusBadRequest)
		return
	}
	systemID := parts[0]
	if err := pathutil.ValidateID(systemID); err != nil {
		http.Error(w, "invalid system id", http.StatusBadRequest)
		return
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestRoutePathTraversalRejection ./pkg/gui/...`  
Expected: PASS

- [ ] **Step 5: Run existing server test suite**

Run: `go test -v ./pkg/gui/...`  
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/gui/server.go pkg/gui/server_test.go
git commit -m "fix(gui): validate route parameters at HTTP perimeter to reject traversal"
```

---

### Task 4: Service Layer ID Validation & Safe Asset Resolution

**Files:**
- Modify: `pkg/gui/service.go`
- Test: `pkg/gui/service_test.go`

- [ ] **Step 1: Write unit tests in `pkg/gui/service_test.go` for service validation**

```go
func TestServiceRejectsInvalidIDs(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	traversalID := "../escaped"

	if _, err := svc.GetEntity(ctx, "valid-game", traversalID); err == nil {
		t.Error("GetEntity expected error for traversal entityID, got nil")
	}
	if err := svc.SaveEntity(ctx, "valid-game", traversalID, "markdown"); err == nil {
		t.Error("SaveEntity expected error for traversal entityID, got nil")
	}
	if _, err := svc.MergeEntities(ctx, "valid-game", traversalID, "target"); err == nil {
		t.Error("MergeEntities expected error for traversal sourceID, got nil")
	}
	if _, err := svc.MergeEntities(ctx, "valid-game", "source", traversalID); err == nil {
		t.Error("MergeEntities expected error for traversal targetID, got nil")
	}
	if err := svc.DeleteGame(ctx, traversalID); err == nil {
		t.Error("DeleteGame expected error for traversal gameID, got nil")
	}
	if _, err := svc.GetWorldEntity(ctx, "valid-world", traversalID); err == nil {
		t.Error("GetWorldEntity expected error for traversal entityID, got nil")
	}
	if err := svc.SaveWorldEntity(ctx, "valid-world", traversalID, "markdown"); err == nil {
		t.Error("SaveWorldEntity expected error for traversal entityID, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test -run TestServiceRejectsInvalidIDs ./pkg/gui/...`  
Expected: FAIL

- [ ] **Step 3: Modify `pkg/gui/service.go`**

Import `"github.com/darkliquid/localrpg/pkg/pathutil"`.

1. In `GetEntity`:
```go
func (s *Service) GetEntity(ctx context.Context, gameID, entityID string) (*EntityDTO, error) {
	if err := pathutil.ValidateID(gameID); err != nil {
		return nil, fmt.Errorf("invalid game id: %w", err)
	}
	if err := pathutil.ValidateID(entityID); err != nil {
		return nil, fmt.Errorf("invalid entity id: %w", err)
	}
```

2. In `SaveEntity`:
```go
func (s *Service) SaveEntity(ctx context.Context, gameID, entityID, rawMarkdown string) error {
	if err := pathutil.ValidateID(gameID); err != nil {
		return fmt.Errorf("invalid game id: %w", err)
	}
	if err := pathutil.ValidateID(entityID); err != nil {
		return fmt.Errorf("invalid entity id: %w", err)
	}
```

3. In `MergeEntities`:
```go
func (s *Service) MergeEntities(ctx context.Context, gameID, sourceID, targetID string) (*EntityDTO, error) {
	if err := pathutil.ValidateID(gameID); err != nil {
		return nil, fmt.Errorf("invalid game id: %w", err)
	}
	if err := pathutil.ValidateID(sourceID); err != nil {
		return nil, fmt.Errorf("invalid source id: %w", err)
	}
	if err := pathutil.ValidateID(targetID); err != nil {
		return nil, fmt.Errorf("invalid target id: %w", err)
	}
```

4. In `DeleteGame` and `RestartGame`:
```go
	if err := pathutil.ValidateID(gameID); err != nil {
		return fmt.Errorf("invalid game id: %w", err)
	}
```

5. In `GetWorld`, `GetWorldEntity`, `SaveWorldEntity`, `DeleteWorldEntity`:
Validate `worldID` and `entityID` using `pathutil.ValidateID`.

6. In `GetSystem` and `SaveSystem`:
Validate `id` / `req.ID` using `pathutil.ValidateID`.

7. In `findAssetFile`:
```go
func findAssetFile(dir string, name string) (string, string) {
	if err := pathutil.ValidateID(name); err != nil {
		return "", ""
	}
	assetsDir := filepath.Join(dir, "assets")
	exts := []string{".png", ".webp", ".jpg", ".jpeg", ".svg"}
	for _, ext := range exts {
		safePath, err := pathutil.ResolveSafeChild(assetsDir, name+ext)
		if err != nil {
			continue
		}
		if fi, err := os.Stat(safePath); err == nil && !fi.IsDir() {
			return safePath, ext
		}
	}
	return "", ""
}
```

8. In `SaveSettings`:
Validate user-specified directory paths with `pathutil.ValidateUserPath`:
```go
	for _, p := range []string{cfg.Paths.Systems, cfg.Paths.Worlds, cfg.Paths.Games, cfg.Paths.Cache} {
		if p != "" {
			if _, err := pathutil.ValidateUserPath(p); err != nil {
				return nil, fmt.Errorf("invalid path %q: %w", p, err)
			}
		}
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestServiceRejectsInvalidIDs ./pkg/gui/...`  
Expected: PASS

- [ ] **Step 5: Run all gui tests**

Run: `go test -v ./pkg/gui/...`  
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/gui/service.go pkg/gui/service_test.go
git commit -m "fix(gui): enforce identifier validation across service methods and asset resolution"
```

---

### Task 5: Engine, Storage, and Export Path Hardening

**Files:**
- Modify: `pkg/engine/game.go`
- Modify: `pkg/engine/timeline.go`
- Modify: `pkg/engine/portrait_worker.go`
- Modify: `pkg/engine/startlocation.go`
- Modify: `pkg/storage/game.go`
- Modify: `pkg/storage/sync.go`
- Modify: `pkg/export/web.go`
- Modify: `pkg/export/video.go`
- Modify: `pkg/gui/export.go`
- Modify: `pkg/provider/ttsfishaudio/client.go`

- [ ] **Step 1: Write tests for engine/storage sanitization**

In `pkg/engine/game_test.go`:
```go
func TestInitGameRejectsInvalidIDs(t *testing.T) {
	paths := core.NewPathResolver(t.TempDir(), "", "", "", "")
	_, err := engine.InitGame(paths, engine.InitOptions{
		GameID:   "../escaped",
		SystemID: "sys",
		WorldID:  "world",
		Name:     "Test",
	})
	if err == nil {
		t.Error("InitGame expected error for traversal GameID, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test -run TestInitGameRejectsInvalidIDs ./pkg/engine/...`  
Expected: FAIL

- [ ] **Step 3: Update `pkg/engine/game.go`**

Import `"github.com/darkliquid/localrpg/pkg/pathutil"`.
In `InitGame`:
```go
	if err := pathutil.ValidateID(opts.GameID); err != nil {
		return nil, fmt.Errorf("invalid game id %q: %w", opts.GameID, err)
	}
	if err := pathutil.ValidateID(opts.SystemID); err != nil {
		return nil, fmt.Errorf("invalid system id %q: %w", opts.SystemID, err)
	}
	if err := pathutil.ValidateID(opts.WorldID); err != nil {
		return nil, fmt.Errorf("invalid world id %q: %w", opts.WorldID, err)
	}
```
When copying templates from world entities:
```go
	templateID = pathutil.SanitizeID(templateID)
```

- [ ] **Step 4: Update `pkg/engine/timeline.go`**

In `writeEntities`:
```go
	for _, id := range ids {
		safeID := pathutil.SanitizeID(id)
		path := filepath.Join(dir, safeID+".md")
...
```

- [ ] **Step 5: Update `pkg/engine/portrait_worker.go`**

In `writePortrait` and `removeStalePortraits`:
```go
	safeID := pathutil.SanitizeID(ent.ID)
	relPath := filepath.Join("assets", "portraits", fmt.Sprintf("%s-v%d%s", safeID, version, ext))
```
and in `removeStalePortraits`:
```go
	safeID := pathutil.SanitizeID(id)
	_ = os.Remove(filepath.Join(dir, fmt.Sprintf("%s-v%d%s", safeID, version, ext)))
```

- [ ] **Step 6: Update `pkg/engine/startlocation.go`**

In `ResolveStartLocation`:
Ensure `manifest.ID` is sanitized: `safeGameID := pathutil.SanitizeID(manifest.ID)`.

- [ ] **Step 7: Update `pkg/storage/game.go`**

In `OpenGameStore` and `CloseGameStore`:
```go
	if err := pathutil.ValidateID(gameID); err != nil {
		return nil, fmt.Errorf("open game store: invalid game id %q: %w", gameID, err)
	}
```

- [ ] **Step 8: Update `pkg/storage/sync.go`**

In `SyncFile`:
```go
	cleanPath := filepath.Clean(path)
	data, err := os.ReadFile(cleanPath)
```

- [ ] **Step 9: Update `pkg/export/web.go`, `pkg/export/video.go`, `pkg/gui/export.go`**

In `export/web.go`:
```go
	cleanOut, err := pathutil.ValidateUserPath(outPath)
	if err != nil {
		return "", fmt.Errorf("invalid export output path: %w", err)
	}
	outPath = cleanOut
```
In `export/video.go`:
```go
	cleanOut, err := pathutil.ValidateUserPath(outputFile)
	if err != nil {
		return fmt.Errorf("invalid video output path: %w", err)
	}
	outputFile = cleanOut
```
In `gui/export.go`:
In `defaultExportPath`:
```go
	safeGameID := pathutil.SanitizeID(gameID)
	return filepath.Join(outDir, safeGameID+"."+format)
```
In `startExport`:
```go
	if req.OutputFile != "" {
		if _, err := pathutil.ValidateUserPath(req.OutputFile); err != nil {
			fail(fmt.Errorf("invalid output file path: %w", err))
			return
		}
	}
```

- [ ] **Step 10: Update `pkg/provider/ttsfishaudio/client.go`**

In `resolveReferenceAudio`:
```go
	cleanPath, err := pathutil.ValidateUserPath(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid audio reference path: %w", err)
	}
	data, err := os.ReadFile(cleanPath)
```

- [ ] **Step 11: Run engine, storage, export, and provider tests**

Run: `go test -v ./pkg/engine/... ./pkg/storage/... ./pkg/export/... ./pkg/provider/ttsfishaudio/...`  
Expected: PASS

- [ ] **Step 12: Commit**

```bash
git add pkg/engine/ pkg/storage/ pkg/export/ pkg/gui/export.go pkg/provider/ttsfishaudio/
git commit -m "fix(security): sanitize derived entity/media paths and validate export destinations"
```

---

### Task 6: Remediate Zip Slip Vulnerability

**Files:**
- Modify: `pkg/models/manager.go:445-455`
- Test: `pkg/models/manager_test.go`

- [ ] **Step 1: Write test for Zip Slip rejection**

In `pkg/models/manager_test.go` (or create if absent):
```go
func TestExtractArchiveRejectsZipSlip(t *testing.T) {
	destDir := t.TempDir()

	// Create a tar archive with a malicious traversal entry
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{
		Name: "../evil.txt",
		Mode: 0600,
		Size: int64(len("malicious")),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("malicious")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	tarPath := filepath.Join(destDir, "test.tar")
	if err := os.WriteFile(tarPath, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := models.NewManager(t.TempDir())
	err := mgr.ExtractArchive(tarPath, filepath.Join(destDir, "out"), "tar")
	if err == nil {
		t.Error("ExtractArchive expected error for Zip Slip traversal entry, got nil")
	}

	// Verify evil.txt was NOT written outside destDir/out
	if _, err := os.Stat(filepath.Join(destDir, "evil.txt")); err == nil {
		t.Error("evil.txt was written outside target directory!")
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test -run TestExtractArchiveRejectsZipSlip ./pkg/models/...`  
Expected: FAIL

- [ ] **Step 3: Modify `pkg/models/manager.go`**

In `extractArchive`:
Replace:
```go
		cleanPath := filepath.Clean(header.Name)
		if filepath.IsAbs(cleanPath) || cleanPath == ".." || len(cleanPath) > 2 && cleanPath[:3] == "../" {
			continue // Zip Slip prevention
		}

		target := filepath.Join(destDir, cleanPath)
```
With:
```go
		cleanName := filepath.Clean(header.Name)
		target := filepath.Join(destDir, cleanName)
		rel, relErr := filepath.Rel(destDir, target)
		if relErr != nil || strings.HasPrefix(rel, "..") || rel == ".." || filepath.IsAbs(cleanName) {
			return fmt.Errorf("archive entry %q escapes destination directory", header.Name)
		}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestExtractArchiveRejectsZipSlip ./pkg/models/...`  
Expected: PASS

- [ ] **Step 5: Run all models tests**

Run: `go test -v ./pkg/models/...`  
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/models/manager.go pkg/models/manager_test.go
git commit -m "fix(models): enforce strict directory containment to prevent Zip Slip"
```

---

### Task 7: Remediate Integer Conversion, Allocation Overflow, and Quoting

**Files:**
- Modify: `pkg/media/pcm.go:60-65`
- Modify: `pkg/media/opus/opus.go:58-63`
- Modify: `pkg/trace/trace.go:133-141`
- Modify: `pkg/trace/stderr.go:50-58`
- Modify: `pkg/provider/oracle/provider.go:120-128`
- Test: `pkg/media/pcm_test.go`, `pkg/media/opus/opus_test.go`, `pkg/trace/trace_test.go`, `pkg/provider/oracle/provider_test.go`

- [ ] **Step 1: Write tests for integer bounds and quote escaping**

1. In `pkg/media/pcm_test.go`:
```go
func TestPCMInvalidSampleRate(t *testing.T) {
	if _, err := pcm.Encode([]int16{0}, -1, 1); err == nil {
		t.Error("pcm.Encode expected error for negative sample rate")
	}
}
```
2. In `pkg/provider/oracle/provider_test.go`:
```go
func TestOracleQuoting(t *testing.T) {
	p := oracle.NewOracleProvider()
	resp, err := p.Generate(context.Background(), `I say "hello"`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(resp, `""hello""`) {
		t.Errorf("unexpected broken quote format: %s", resp)
	}
}
```

- [ ] **Step 2: Update `pkg/media/pcm.go`**

In `pkg/media/pcm.go`:
Import `"math"`.
When parsing sample rate in header:
```go
	if rate, err := strconv.ParseUint(strings.TrimPrefix(part, "rate="), 10, 32); err == nil && rate > 0 {
		sampleRate = int(rate)
	}
```
In `Encode`:
```go
	if sampleRate <= 0 || sampleRate > math.MaxUint32 {
		return nil, fmt.Errorf("pcm: invalid sample rate %d", sampleRate)
	}
```

- [ ] **Step 3: Update `pkg/media/opus/opus.go`**

In `pkg/media/opus/opus.go`:
Import `"math"`.
In `Encode`:
```go
	if sampleRate <= 0 || sampleRate > math.MaxUint32 {
		return nil, fmt.Errorf("opus: invalid sample rate %d", sampleRate)
	}
```

- [ ] **Step 4: Update `pkg/trace/trace.go` and `pkg/trace/stderr.go`**

In `pkg/trace/trace.go`:
Replace:
```go
	stamped := Sanitize(fields, m.level, defaultPayloadChars)
	if m.game != "" {
		stamped = make(map[string]interface{}, len(fields)+1)
		for key, value := range fields {
			stamped[key] = value
		}
		stamped["game"] = m.game
	}
```
With:
```go
	stamped := Sanitize(fields, m.level, defaultPayloadChars)
	if m.game != "" {
		stamped = make(map[string]interface{})
		for key, value := range fields {
			stamped[key] = value
		}
		stamped["game"] = m.game
	}
```
Apply the same edit in `pkg/trace/stderr.go`.

- [ ] **Step 5: Update `pkg/provider/oracle/provider.go`**

In `pkg/provider/oracle/provider.go:120-128`:
```go
	cleanAction := strings.ReplaceAll(playerAction, "\"", "'")
	return fmt.Sprintf("%s\n\nAs you declare: '%s', the stones echo your effort.%s What do you do next?", chosenOpener, cleanAction, entityWitness)
```

- [ ] **Step 6: Run tests to verify**

Run: `go test -v ./pkg/media/... ./pkg/trace/... ./pkg/provider/oracle/...`  
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add pkg/media/ pkg/trace/ pkg/provider/oracle/
git commit -m "fix(security): resolve integer conversion bounds, allocation hint, and quoting alerts"
```

---

### Task 8: DOM XSS Image Source Validation in Frontend

**Files:**
- Create: `frontend/src/utils/security.ts`
- Modify: `frontend/src/components/WorldsStudio.tsx:870-935`
- Modify: `frontend/src/components/launcher/NewCampaignModal.tsx:620-668`

- [ ] **Step 1: Create `frontend/src/utils/security.ts`**

```typescript
/**
 * safeImagePreview validates that an image source URL begins strictly with
 * an expected safe scheme before rendering into the DOM.
 */
export function safeImagePreview(url: string | null | undefined): string | undefined {
  if (!url) return undefined;
  const trimmed = url.trim();
  if (
    trimmed.startsWith('blob:') ||
    trimmed.startsWith('/api/') ||
    trimmed.startsWith('data:image/')
  ) {
    return trimmed;
  }
  return undefined;
}
```

- [ ] **Step 2: Update `frontend/src/components/WorldsStudio.tsx`**

Import `safeImagePreview` from `../utils/security`.
Around line 873:
```tsx
                    {safeImagePreview(bannerPreview) ? (
                      <img
                        src={safeImagePreview(bannerPreview)}
                        alt="Banner Preview"
                        className="w-full h-full object-cover"
                        onError={() => setBannerPreview(null)}
                      />
                    ) : (
```
Around line 930:
```tsx
                    {safeImagePreview(iconPreview) ? (
                      <img
                        src={safeImagePreview(iconPreview)}
                        alt="Icon Preview"
                        className="w-16 h-16 rounded-xl object-cover"
                        onError={() => setIconPreview(null)}
                      />
                    ) : (
```

- [ ] **Step 3: Update `frontend/src/components/launcher/NewCampaignModal.tsx`**

Import `safeImagePreview` from `../../utils/security`.
Around line 623:
```tsx
                  {safeImagePreview(bannerPreview) ? (
                    <img src={safeImagePreview(bannerPreview)} alt="Banner Preview" className="w-full h-full object-cover" />
                  ) : (
```
Around line 664:
```tsx
                  {safeImagePreview(iconPreview) ? (
                    <img src={safeImagePreview(iconPreview)} alt="Icon Preview" className="w-12 h-12 rounded-lg object-cover" />
                  ) : (
```

- [ ] **Step 4: Run frontend TypeScript verification**

Run: `mise run test:frontend` (or `cd frontend && npx tsc --noEmit`)  
Expected: PASS with 0 type errors.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/utils/security.ts frontend/src/components/WorldsStudio.tsx frontend/src/components/launcher/NewCampaignModal.tsx
git commit -m "fix(frontend): validate preview image URLs against safe schemes to prevent DOM XSS"
```

---

### Task 9: Full Verification and Lint Gate

**Files:**
- N/A (Project-wide test & lint verification)

- [ ] **Step 1: Run full backend test suite**

Run: `mise run test:backend`  
Expected: All backend tests pass with exit code 0.

- [ ] **Step 2: Run frontend type checks**

Run: `mise run test:frontend`  
Expected: Clean output with exit code 0.

- [ ] **Step 3: Run project linters**

Run: `mise run lint`  
Expected: All linters (go vet, markdownlint, goreleaser, actionlint) pass with exit code 0.

- [ ] **Step 4: Run secret scanning backstop**

Run: `mise run secrets:scan`  
Expected: Clean scan with exit code 0.

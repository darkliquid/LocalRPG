# CodeQL Security and Path Sanitization Design

**Date:** 2026-10-04  
**Status:** Approved  
**Scope:** Security remediation for all 122 GitHub CodeQL alerts (108 open, 14 closed); centralized path validation; Zip Slip, integer conversion, allocation size, quoting, and DOM XSS fixes.  
**Related:** `pkg/core/types.go`, `pkg/gui/server.go`, `pkg/gui/service.go`, `pkg/engine/game.go`, `pkg/engine/timeline.go`, `pkg/engine/portrait_worker.go`, `pkg/engine/startlocation.go`, `pkg/storage/game.go`, `pkg/storage/sync.go`, `pkg/export/web.go`, `pkg/export/video.go`, `pkg/models/manager.go`, `pkg/media/opus/opus.go`, `pkg/media/pcm.go`, `pkg/trace/trace.go`, `pkg/trace/stderr.go`, `pkg/provider/oracle/provider.go`, `frontend/src/components/WorldsStudio.tsx`, `frontend/src/components/launcher/NewCampaignModal.tsx`.

---

## 1. Problem & Findings Audit

A full audit of the repository's GitHub CodeQL scan history identifies **122 alerts** (108 currently open, 14 closed):

| Rule ID | Severity | Open | Closed | Target Area |
| :--- | :--- | :--- | :--- | :--- |
| `go/path-injection` | Error (High) | 98 | 14 | Uncontrolled data used in path expressions across HTTP handlers, service methods, engine file operations, and storage stores. (10 closed were marked "won't fix", 4 closed due to line movements). |
| `js/xss-through-dom` | Warning (High) | 4 | 0 | File object URLs rendered directly into `<img src={...}>` in `WorldsStudio.tsx` and `NewCampaignModal.tsx`. |
| `go/zipslip` | Error (High) | 1 | 0 | Incomplete directory escape prevention during tar archive extraction in `pkg/models/manager.go`. |
| `go/incorrect-integer-conversion` | Warning (Medium) | 2 | 0 | Architecture-dependent `int` from `strconv.Atoi` narrowed to `uint32` without bounds checking in `opus.go` and `pcm.go`. |
| `go/allocation-size-overflow` | Warning (Medium) | 2 | 0 | `make(map[...], len(fields)+1)` in `trace.go` and `stderr.go` vulnerable to theoretical integer overflow. |
| `go/unsafe-quoting` | Warning (Critical) | 1 | 0 | Raw user action interpolated directly inside double quotes in `pkg/provider/oracle/provider.go`. |

### Intentional vs. Derived Paths
LocalRPG allows users to specify file and directory paths in specific settings:
1. **Intentional user paths:**
   - Storage directory overrides in Settings (`cfg.Paths.Systems`, `cfg.Paths.Worlds`, `cfg.Paths.Games`, `cfg.Paths.Cache`).
   - Story export output destinations (`req.OutputFile` / CLI `-o`).
   - Custom voice reference audio paths (`ttsfishaudio`).
2. **Derived paths (security boundary):**
   - Paths derived from entities, campaigns, worlds, rule systems, character IDs, turn numbers, or asset kinds (e.g. `games/<id>/entities/<entityID>.md`, `assets/portraits/<id>-v<N>.<ext>`).
   - These **must never** permit path traversal characters (`..`, `/`, `\`, null bytes, control sequences) or escape their intended base directories.

---

## 2. Core Architecture: `pkg/pathutil`

A new dedicated package, [`pkg/pathutil`](file:///home/darkliquid/Projects/LocalRPG/pkg/pathutil), provides uniform validation, sanitization, and containment primitives.

### 2.1 Identifier Validation (`ValidateID`)
```go
package pathutil

import (
    "errors"
    "fmt"
    "regexp"
    "strings"
)

var (
    ErrEmptyID       = errors.New("identifier cannot be empty")
    ErrPathTraversal = errors.New("identifier cannot contain path separators or traversal sequences")
    ErrInvalidID     = errors.New("identifier contains invalid characters")

    validIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

// ValidateID ensures an identifier consists strictly of alphanumeric characters,
// dashes, or underscores, and contains no directory separators or traversal tokens.
func ValidateID(id string) error {
    trimmed := strings.TrimSpace(id)
    if trimmed == "" {
        return ErrEmptyID
    }
    if strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") || strings.Contains(trimmed, "..") {
        return ErrPathTraversal
    }
    if !validIDPattern.MatchString(trimmed) {
        return fmt.Errorf("%w: %q", ErrInvalidID, trimmed)
    }
    return nil
}
```

### 2.2 Identifier Sanitization (`SanitizeID`)
For defense-in-depth where a function must not fail but must guarantee safe path construction (such as internal directory construction in `PathResolver`):
```go
// SanitizeID normalizes an input string into a safe kebab-case identifier,
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
        case r == ' ' || r == '-' || r == '_':
            if buf.Len() > 0 && !precededBySeparator {
                buf.WriteRune('-')
                precededBySeparator = true
            }
        }
    }
    res := strings.TrimSuffix(buf.String(), "-")
    if res == "" {
        return "unnamed"
    }
    return res
}
```

### 2.3 Safe Child Path Containment (`ResolveSafeChild`)
Provides canonical directory containment that satisfies CodeQL path-traversal barrier checks:
```go
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
```

### 2.4 Intentional User Path Validation (`ValidateUserPath`)
For user-configured paths (Settings, export destinations, reference audio files):
```go
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

---

## 3. Perimeter & Service Layer Integration

### 3.1 HTTP Route Perimeter ([`pkg/gui/server.go`](file:///home/darkliquid/Projects/LocalRPG/pkg/gui/server.go))
Validate all URL path parameters before service dispatch:
1. `handleGameRoutes`:
   - Validate `gameID := parts[0]` with `pathutil.ValidateID(gameID)`. Return `400 Bad Request` if invalid.
   - For entity routes (`/api/game/{gameID}/entity/{entityID}`), validate `parts[2]`.
   - For character routes (`/api/game/{gameID}/character/{charID}/portrait`), validate `parts[2]`.
2. `handleWorldRoutes`:
   - Validate `worldID := parts[0]`.
   - For world entity routes (`/api/world/{worldID}/entity/{entityID}`), validate `parts[2]`.
   - For world asset routes (`/api/world/{worldID}/banner`, `/api/world/{worldID}/icon`), validate action.
3. `handleSystemRoutes`:
   - Validate `systemID := parts[0]`.
4. `http.ServeFile`:
   - Ensure served files are resolved through safe child checks against the app's asset directories.

### 3.2 Service Layer ([`pkg/gui/service.go`](file:///home/darkliquid/Projects/LocalRPG/pkg/gui/service.go))
1. **Game & Campaign Operations:**
   - `GetGameState`, `GetEntity`, `SaveEntity`, `ListEntities`: Validate `gameID` and `entityID`.
   - `MergeEntities`: Validate both `sourceID` and `targetID`.
   - `DeleteGame`, `RestartGame`: Validate `gameID`.
2. **World Operations:**
   - `GetWorld`, `SaveWorld`, `writeWorld`, `UpdateWorld`: Validate `worldID`.
   - `GetWorldEntity`, `SaveWorldEntity`, `DeleteWorldEntity`: Validate `worldID` and `entityID`.
3. **System Operations:**
   - `GetSystem`, `SaveSystem`: Validate `systemID`.
4. **Asset Operations:**
   - `GetGameAsset`, `SaveGameAsset`, `GetWorldAsset`, `SaveWorldAsset`: Validate `assetKind` (must be one of `banner`, `icon`, `portraits`, `scenes`, `audio`, or match `[a-zA-Z0-9_-]+`) and file extension (`.png`, `.jpg`, `.jpeg`, `.webp`, `.svg`).
   - `findAssetFile`: Use `pathutil.ResolveSafeChild` to ensure asset searches cannot escape `assets/`.
5. **Settings Operations (`SaveSettings`):**
   - Validate `cfg.Paths.Systems`, `cfg.Paths.Worlds`, `cfg.Paths.Games`, `cfg.Paths.Cache` via `pathutil.ValidateUserPath` before creating directories with `os.MkdirAll`.

### 3.3 Core Path Resolver ([`pkg/core/types.go`](file:///home/darkliquid/Projects/LocalRPG/pkg/core/types.go))
Enforce defense-in-depth so internal callers never traverse outside the base directories:
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

---

## 4. Engine, Storage, and Worker Hardening

### 4.1 Engine Operations
- **`pkg/engine/game.go`**:
  - `InitGame`: Validate `systemID`, `worldID`, and `gameID`. Sanitize template entity IDs (`pathutil.SanitizeID(template.ID)`) when copying world templates into the game's `entities/` folder.
  - `ensurePlayerNote`: Sanitize the generated player note filename.
- **`pkg/engine/timeline.go`**:
  - `writeEntities`: Sanitize `id` (`pathutil.SanitizeID(id)`) before writing `filepath.Join(dir, id+".md")`.
- **`pkg/engine/portrait_worker.go`**:
  - `writePortrait` & `removeStalePortraits`: Sanitize `ent.ID` / `id` so filenames like `<id>-v<version>.<ext>` cannot contain path traversal characters.
- **`pkg/engine/startlocation.go`**:
  - Sanitize `manifest.ID` before generating opening scene markdown note.
- **`pkg/engine/history.go`**:
  - Reading history relies on `paths.GameDir(gameID)`, which is now sanitized against traversal.

### 4.2 Storage & Syncing
- **`pkg/storage/game.go`**:
  - `OpenGameStore` and `CloseGameStore`: Sanitize `gameID` so `GameDBPath` and legacy `.legacy` database cleanup remain strictly within the campaign's directory.
- **`pkg/storage/sync.go`**:
  - `Sync` and `SyncFile`: Validate that all examined note paths are resolved within the entities directory using `pathutil.ResolveSafeChild`.

### 4.3 Exporting
- **`pkg/export/web.go` & `pkg/export/video.go`**:
  - Normalize export output destinations with `pathutil.ValidateUserPath`.
- **`pkg/gui/export.go`**:
  - In `defaultExportPath`, sanitize `gameID` so the generated HTML/WebM file is contained within `outDir`.
  - Validate custom `req.OutputFile` with `pathutil.ValidateUserPath`.

### 4.4 Voice Audio Reference
- **`pkg/provider/ttsfishaudio/client.go`**:
  - In `resolveReferenceAudio`, validate non-URL audio file paths with `pathutil.ValidateUserPath` before calling `os.ReadFile`.

---

## 5. Non-Path Vulnerability Remediations

### 5.1 Zip Slip Prevention ([`pkg/models/manager.go`](file:///home/darkliquid/Projects/LocalRPG/pkg/models/manager.go#L445-L450)) - Alert #114
Replace string-slice prefix checks with `filepath.Rel` directory containment:
```go
target := filepath.Join(destDir, filepath.Clean(header.Name))
rel, err := filepath.Rel(destDir, target)
if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
    return fmt.Errorf("archive entry %q escapes destination directory", header.Name)
}
```

### 5.2 Integer Conversion Bounds Checking ([`pkg/media/opus/opus.go`](file:///home/darkliquid/Projects/LocalRPG/pkg/media/opus/opus.go#L60), [`pkg/media/pcm.go`](file:///home/darkliquid/Projects/LocalRPG/pkg/media/pcm.go#L62)) - Alerts #5 & #6
- In `pkg/media/pcm.go`:
  - When parsing sample rate, parse directly with `strconv.ParseUint(..., 10, 32)` or verify `sampleRate > 0 && sampleRate <= math.MaxUint32`.
- In `pkg/media/opus/opus.go`:
  - In `Encode`, verify `sampleRate > 0 && sampleRate <= math.MaxUint32` before casting `uint32(sampleRate)`.

### 5.3 Allocation Size Overflow ([`pkg/trace/trace.go`](file:///home/darkliquid/Projects/LocalRPG/pkg/trace/trace.go#L135), [`pkg/trace/stderr.go`](file:///home/darkliquid/Projects/LocalRPG/pkg/trace/stderr.go#L52)) - Alerts #116 & #117
In both `Event` implementations, avoid arithmetic inside allocation size hint:
```go
stamped = make(map[string]interface{})
for key, value := range fields {
    stamped[key] = value
}
stamped["game"] = l.game // or m.game
```

### 5.4 Potentially Unsafe Quoting ([`pkg/provider/oracle/provider.go`](file:///home/darkliquid/Projects/LocalRPG/pkg/provider/oracle/provider.go#L126)) - Alert #115
Escape or replace double quotes in `playerAction` to prevent quote breaking:
```go
cleanAction := strings.ReplaceAll(playerAction, "\"", "'")
return fmt.Sprintf("%s\n\nAs you declare: '%s', the stones echo your effort.%s What do you do next?", chosenOpener, cleanAction, entityWitness)
```

### 5.5 DOM Image Source Validation ([`WorldsStudio.tsx`](file:///home/darkliquid/Projects/LocalRPG/frontend/src/components/WorldsStudio.tsx), [`NewCampaignModal.tsx`](file:///home/darkliquid/Projects/LocalRPG/frontend/src/components/launcher/NewCampaignModal.tsx)) - Alerts #1, #2, #3, #4
Add a shared utility function:
```typescript
export function safeImagePreview(url: string | null | undefined): string | undefined {
  if (!url) return undefined;
  const trimmed = url.trim();
  if (trimmed.startsWith('blob:') || trimmed.startsWith('/api/') || trimmed.startsWith('data:image/')) {
    return trimmed;
  }
  return undefined;
}
```
Use `safeImagePreview(bannerPreview)` and `safeImagePreview(iconPreview)` on all `<img src={...}>` bindings.

---

## 6. Testing & Verification

1. **Unit Tests (`pkg/pathutil/pathutil_test.go`):**
   - Comprehensive test cases for `ValidateID`, `SanitizeID`, `ResolveSafeChild`, and `ValidateUserPath`.
   - Coverage for empty, traversal (`../`), separator (`/`, `\`), null byte (`\x00`), and Unicode/special characters.
2. **Perimeter Tests (`pkg/gui/server_test.go`):**
   - HTTP tests sending path traversal payloads to all route families (`/api/game/...`, `/api/world/...`, `/api/system/...`).
   - Confirm immediate `400 Bad Request` responses without filesystem touch.
3. **Service Layer Tests (`pkg/gui/service_test.go`):**
   - Test `MergeEntities`, `DeleteGame`, `RestartGame`, `SaveEntity`, `SaveWorldEntity` with invalid and traversal IDs.
4. **Zip Slip Extraction Tests (`pkg/models/manager_test.go`):**
   - Test archive containing `../traversal.bin` entry; ensure error returned and no file created outside `destDir`.
5. **Media & Trace Bounds Tests:**
   - Test `pcm.Encode` and `opus.Encode` bounds checking.
   - Test trace logger with varying field map sizes.
6. **Lint & Toolchain Verification:**
   - `mise run test:backend`
   - `mise run test:frontend`
   - `mise run lint`

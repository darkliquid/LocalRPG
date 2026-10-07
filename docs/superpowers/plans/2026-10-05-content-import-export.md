# Content Import and Export Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Export a system or world as a `.lrpgpack` and import one with checksum verification and explicit conflict handling, from the CLI and the studio.

**Architecture:** Two endpoints wrapping `pkg/content`; import stages, validates, and installs atomically; CLI verbs and studio actions call the same service.

**Tech Stack:** Go standard library; React 19.

**Spec:** `docs/superpowers/specs/2026-10-05-content-import-export-design.md`
**Depends on:** PKG-1.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- Never overwrite silently; `refuse` is the default.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The export endpoint

**Files:**
- Modify: `pkg/gui/routes.go`, `pkg/gui/server.go`, `pkg/gui/service.go`, `pkg/gui/types.go`
- Test: `pkg/gui/content_export_test.go`

**Interfaces:**
- Consumes: `content.Pack`, `core.PathResolver`.
- Produces: `POST /api/content/export`, `Service.ExportContent(ctx, typ, id string, w io.Writer) (content.Manifest, error)`.

- [ ] **Step 1: Write the failing test**

```go
func TestExportContentStreamsAPackage(t *testing.T) {
	svc := newTestServiceWithWorld(t, "ashen_reach")
	var buf bytes.Buffer
	m, err := svc.ExportContent(context.Background(), "world", "ashen_reach", &buf)
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "ashen_reach" || buf.Len() == 0 {
		t.Fatalf("manifest %+v, %d bytes", m, buf.Len())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/gui/ -run TestExportContentStreamsAPackage -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Resolve the directory by type, call `content.Pack`, and stream the bytes with the right headers. Mount
the route.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/gui/ -run TestExportContentStreamsAPackage -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui
git commit -m "feat(gui): export content as a package"
```

---

### Task 2: The import endpoint

**Files:**
- Modify: `pkg/gui/routes.go`, `pkg/gui/server.go`, `pkg/gui/service.go`, `pkg/gui/types.go`
- Test: `pkg/gui/content_import_test.go`

**Interfaces:**
- Consumes: `content.Unpack`, `core.LoadWorldManifest`/`LoadSystemManifest`.
- Produces: `POST /api/content/import`, `Service.ImportContent(ctx, r io.Reader, onConflict string) (ImportResultDTO, error)`.

- [ ] **Step 1: Write the failing tests**

```go
func TestImportContentInstallsANewWorld(t *testing.T) { /* … */ }
func TestImportContentConflictModes(t *testing.T) {
	// refuse -> 409-equivalent error; rename -> a new id; overwrite -> replaced.
}
func TestImportContentRejectsTampered(t *testing.T) { /* … */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/gui/ -run TestImportContent -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Stream the upload to a temp file, `content.Unpack` into a staging dir, validate the manifest and
parse, resolve the conflict per `onConflict`, then rename into place. Return a DTO with the manifest
summary and `has_script`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/gui/ -run TestImportContent -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui
git commit -m "feat(gui): import content with conflict handling"
```

---

### Task 3: The CLI verbs

**Files:**
- Create: `cmd/localrpg/content.go`
- Modify: `cmd/localrpg/main.go`
- Test: `cmd/localrpg/content_test.go`

**Interfaces:**
- Consumes: `Service.ExportContent`/`ImportContent` (or `pkg/content` directly).
- Produces: `localrpg content export <type> <id> [--out f]` and `localrpg content import <file> [--on-conflict …] [--yes]`.

- [ ] **Step 1: Write the failing tests**

```go
func TestContentExportCLI(t *testing.T) { /* writes a package to --out or stdout */ }
func TestContentImportCLIRequiresYes(t *testing.T) { /* refuses without --yes */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/localrpg/ -run TestContent -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Implement the two verbs, printing the manifest summary on import and requiring `--yes` (or a
conflict flag) before installing.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/localrpg/ -run TestContent -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/localrpg
git commit -m "feat(cli): add content export and import"
```

---

### Task 4: The studio actions

**Files:**
- Modify: `frontend/src/api/client.ts`, `frontend/src/types.ts`
- Modify: `frontend/src/components/SystemsStudio.tsx`, `frontend/src/components/WorldsStudio.tsx`, `frontend/src/components/LauncherHub.tsx`
- Test: `frontend/src/components/ContentImportDialog.test.tsx`

**Interfaces:**
- Consumes: the endpoints (Tasks 1-2).
- Produces: export and import actions with a manifest confirmation.

- [ ] **Step 1: Write the failing test**

```tsx
test("import shows the manifest and a mechanics.js warning", () => {
  render(<ContentImportDialog manifest={{ id: "x", name: "X", version: "1", has_script: true }}
    onConfirm={() => {}} onCancel={() => {}} />);
  expect(screen.getByText(/mechanics\.js/)).toBeInTheDocument();
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- ContentImportDialog`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add `exportContent`/`importContent` to `client.ts`. Add an export action to both studios and a
`ContentImportDialog` that shows the manifest summary (including a `mechanics.js` warning) and the
conflict choice before installing.

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run test -- ContentImportDialog`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src
git commit -m "feat(frontend): export and import content from the studios"
```

---

### Task 5: Verification

- [ ] **Step 1: Round-trip guard**

Add a test asserting export-then-import reproduces a world's files.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- Export streams a valid package.
- Import installs, refuses, renames, or overwrites per choice.
- A tampered package is rejected.
- The confirmation names a bundled script.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the content round trip"
```

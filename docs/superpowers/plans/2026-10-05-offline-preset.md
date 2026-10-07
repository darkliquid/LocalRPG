# Offline Preset Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One action that configures a fully offline stack and a check that reports any provider that is not offline.

**Architecture:** A config transform sets the four families to the offline providers; `VerifyOffline` resolves each provider and inspects its descriptor; the settings UI and CLI expose both.

**Tech Stack:** Go standard library; React 19.

**Spec:** `docs/superpowers/specs/2026-10-05-offline-preset-design.md`
**Depends on:** MP-1, LF-3.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- The bundle leaves paths and preferences untouched.
- Applying twice is idempotent.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The bundle transform

**Files:**
- Create: `pkg/config/offline.go`
- Test: `pkg/config/offline_test.go`

**Interfaces:**
- Consumes: `Config`.
- Produces: `func ApplyOfflinePreset(cfg *Config, tts string) []string` returning the changes.

- [ ] **Step 1: Write the failing test**

```go
func TestApplyOfflinePreset(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Paths.Games = "/custom"
	changes := ApplyOfflinePreset(cfg, "native-os")
	if cfg.Agents.Roles["gm"].BuiltinName != "narrative-oracle" {
		t.Fatalf("gm = %+v", cfg.Agents.Roles["gm"])
	}
	if cfg.Media.TTS.BuiltinName != "native-os" || cfg.Media.Image.BuiltinName != "procedural-art" {
		t.Fatalf("media = %+v", cfg.Media)
	}
	if cfg.Paths.Games != "/custom" {
		t.Fatal("the preset must not touch paths")
	}
	if len(changes) == 0 {
		t.Fatal("the preset should report its changes")
	}
}
func TestApplyOfflinePresetIdempotent(t *testing.T) { /* applying twice yields the same config */ }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/config/ -run TestApplyOfflinePreset -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Set `gm` to the oracle, the other roles to the oracle or disabled, TTS to the requested local
provider, image to `procedural-art`, and embeddings to `builtin`; collect a human-readable change
list.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/config/ -run TestApplyOfflinePreset -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/config/offline.go pkg/config/offline_test.go
git commit -m "feat(config): add the offline preset"
```

---

### Task 2: `VerifyOffline`

**Files:**
- Create: `pkg/provider/offline.go`
- Test: `pkg/provider/offline_test.go`

**Interfaces:**
- Consumes: `config.Config`, the registry, `harness.KeyFor`/`media.TTSKeyFor`/etc.
- Produces: `OfflineReport`, `OfflineIssue`, `func VerifyOffline(cfg *config.Config) OfflineReport`.

- [ ] **Step 1: Write the failing tests**

```go
func TestVerifyOfflineClean(t *testing.T) {
	cfg := config.DefaultConfig()
	config.ApplyOfflinePreset(cfg, "native-os")
	if r := VerifyOffline(cfg); !r.Offline {
		t.Fatalf("report = %+v", r)
	}
}
func TestVerifyOfflineFlagsCloud(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles["gm"] = config.AgentRoleConfig{Type: "builtin", BuiltinName: "gemini"}
	r := VerifyOffline(cfg)
	if r.Offline || len(r.Issues) == 0 {
		t.Fatalf("report = %+v", r)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/provider/ -run TestVerifyOffline -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

For each resolved role and media family, resolve its key, look up the descriptor, and check
`FeatureOffline` and the tier; collect issues with the role, key, tier, and a reason.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/provider/ -run TestVerifyOffline -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/provider/offline.go pkg/provider/offline_test.go
git commit -m "feat(provider): verify a configuration is offline"
```

---

### Task 3: The endpoint and the CLI

**Files:**
- Modify: `pkg/gui/routes.go`, `pkg/gui/server.go`, `pkg/gui/service.go`, `pkg/gui/types.go`
- Create: `cmd/localrpg/offline.go`
- Modify: `cmd/localrpg/main.go`
- Test: `pkg/gui/offline_test.go`, `cmd/localrpg/offline_test.go`

**Interfaces:**
- Consumes: Tasks 1-2.
- Produces: `POST /api/config/offline-preset`, `GET /api/config/offline-report`, and
  `localrpg config offline-preset|check-offline`.

- [ ] **Step 1: Write the failing tests**

```go
func TestOfflinePresetEndpoint(t *testing.T) { /* applying returns the change list */ }
func TestOfflineReportEndpoint(t *testing.T) { /* returns the report */ }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/gui/ -run TestOffline -v` and `go test ./cmd/localrpg/ -run TestOffline -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the endpoints and the two verbs, printing the changes and the report.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/gui/ -run TestOffline -v` and `go test ./cmd/localrpg/ -run TestOffline -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui cmd/localrpg
git commit -m "feat: expose the offline preset and report"
```

---

### Task 5: The settings UI

**Files:**
- Modify: `frontend/src/components/ProviderManager.tsx` or `SettingsStudio.tsx`, `frontend/src/api/client.ts`, `frontend/src/types.ts`
- Test: `frontend/src/components/OfflinePreset.test.tsx`

**Interfaces:**
- Consumes: the endpoints (Task 3).
- Produces: an "Offline preset" action with a confirmation, and a "Check offline" action with the
  report.

- [ ] **Step 1: Write the failing test**

```tsx
test("shows what the offline preset will change", () => {
  render(<OfflinePreset onChange={() => {}} />);
  fireEvent.click(screen.getByRole("button", { name: /offline preset/i }));
  // Assert a confirmation listing the affected roles/families appears.
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- OfflinePreset`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Add the two actions to the providers section, with a confirmation of the changes and a report panel.

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run test -- OfflinePreset`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src
git commit -m "feat(frontend): add the offline preset and check"
```

---

### Task 6: Verification

- [ ] **Step 1: Regression guard**

Add a test that applying the bundle does not alter paths or preferences.

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- The bundle sets the four families and reports its changes.
- Applying twice is idempotent.
- `VerifyOffline` flags a cloud and a local-server provider.
- Paths and preferences are untouched.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard the offline preset scope"
```

# CI/CD Acceleration with Aube, Go Test Caching, and Path Filtering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Accelerate CI/CD pipelines and local development loops by migrating frontend dependency management to `aube`, enabling Go test build/result caching, and skipping unchanged test suites on PRs via coordinator path-filtering.

**Architecture:** Add `aube` 2.7.2 to `mise.toml` and adopt `jdx/aube-action@v1` for instant content-addressable `node_modules` caching in GitHub Actions. Remove `-count=1` and `-shuffle=on` defaults from Go test scripts so `go test` caches unchanged package results while preserving coverage calculations. Add `dorny/paths-filter@v3` coordinator job in `ci.yml` to conditionally skip unchanged test suites on pull requests while running the full suite on `main`.

**Tech Stack:** `mise`, `aube` (2.7.2), `jdx/aube-action` (v1), `dorny/paths-filter` (v3), Go 1.27.2 test runner, GitHub Actions.

---

## File Map

- Modify: [`mise.toml`](file:///home/darkliquid/Projects/LocalRPG/mise.toml) - Pin `aube = "2.7.2"`, update `install:frontend`, add `test:backend:clean`, update `test:backend:fast` and `test:backend:race`
- Modify: [`scripts/check-coverage.sh`](file:///home/darkliquid/Projects/LocalRPG/scripts/check-coverage.sh) - Remove `-count=1` and make `-shuffle=on` opt-in
- Modify: [`scripts/test-cross-platform.sh`](file:///home/darkliquid/Projects/LocalRPG/scripts/test-cross-platform.sh) - Remove `-count=1 -shuffle=on`
- Modify: [`.github/workflows/ci.yml`](file:///home/darkliquid/Projects/LocalRPG/.github/workflows/ci.yml) - Add `changes` job via `dorny/paths-filter`, replace npm caching with `jdx/aube-action`, add conditional `if:` checks to jobs
- Modify: [`.github/workflows/release.yml`](file:///home/darkliquid/Projects/LocalRPG/.github/workflows/release.yml) - Update frontend packaging job to use `jdx/aube-action`
- Modify: [`AGENTS.md`](file:///home/darkliquid/Projects/LocalRPG/AGENTS.md) - Update documented commands and CI architecture notes

---

### Task 1: Adopt Aube in `mise.toml` for Frontend Dependency Management

**Files:**
- Modify: [`mise.toml`](file:///home/darkliquid/Projects/LocalRPG/mise.toml:1-25)

- [ ] **Step 1: Update `mise.toml` to pin `aube` and update `install:frontend`**

In `mise.toml`:
Add `aube = "2.7.2"` to `[tools]` and change `npm install` to `aube install`:

```toml
[tools]
go = "1.27.2"
node = "26.9.0"
aube = "2.7.2"
goreleaser = "2.18.2"
gitleaks = "8.30.1"
actionlint = "1.7.12"
vale = "3.24.0"
rumdl = "0.2.78"
staticcheck = "2026.2.2"
revive = "1.15.0"
"go:golang.org/x/vuln/cmd/govulncheck" = "1.9.0"

[tasks."install:frontend"]
description = "Install frontend dependencies when package manifests change"
dir = "frontend"
run = "aube install"
sources = ["package.json", "package-lock.json"]
outputs = ["node_modules/.package-lock.json"]
```

- [ ] **Step 2: Install Aube and test dependency installation**

Run:
```bash
mise install aube
mise run install:frontend
```
Expected: `aube 2.7.2` installs and `aube install` succeeds in `frontend/`.

- [ ] **Step 3: Verify git status on lockfiles**

Run:
```bash
git status
```
Expected: `package-lock.json` is not corrupted; working tree remains clean or minimally updated.

- [ ] **Step 4: Commit**

```bash
git add mise.toml
git commit -m "chore(tooling): pin aube 2.7.2 and update frontend install task (LocalRPG-ccz.10)"
```

---

### Task 2: Unlock Go Test Caching in Scripts and Mise Tasks

**Files:**
- Modify: [`scripts/check-coverage.sh`](file:///home/darkliquid/Projects/LocalRPG/scripts/check-coverage.sh:24-30)
- Modify: [`scripts/test-cross-platform.sh`](file:///home/darkliquid/Projects/LocalRPG/scripts/test-cross-platform.sh:10-15)
- Modify: [`mise.toml`](file:///home/darkliquid/Projects/LocalRPG/mise.toml:52-70)

- [ ] **Step 1: Update `scripts/check-coverage.sh`**

Replace line 26 of `scripts/check-coverage.sh`:
```bash
# -v so a CI log shows which test failed, and one profile for the whole run so the
# total is the project's rather than a package's.
shuffle_flag=""
[ -n "${SHUFFLE:-}" ] && shuffle_flag="-shuffle=on"
count_flag=""
[ -n "${CLEAN:-}" ] && count_flag="-count=1"

# shellcheck disable=SC2086
go test -v $count_flag $shuffle_flag -covermode=atomic -coverprofile="$profile" ./...
```

- [ ] **Step 2: Update `scripts/test-cross-platform.sh`**

Replace lines 12 and 14 in `scripts/test-cross-platform.sh`:
```bash
if [ "${RUNNER_OS:-}" = "Windows" ] || [[ "$(uname -s)" =~ ^(MINGW|MSYS|CYGWIN) ]]; then
  # Exclude GUI/Wails binaries, SQLite-locked stores, and native DLL-dependent media/provider packages on Windows runner.
  packages=$(go list ./... | grep -v -E "(cmd/localrpg|pkg/e2e|pkg/gui|pkg/engine|pkg/storage|pkg/export|pkg/media|pkg/provider)")
  # shellcheck disable=SC2086
  go test -v $packages
else
  go test -v ./...
fi
```

- [ ] **Step 3: Update `mise.toml` backend test tasks**

In `mise.toml`:
```toml
[tasks."test:backend"]
description = "Run all Go unit and integration tests, and check statement coverage"
run = "bash scripts/check-coverage.sh"

[tasks."test:backend:fast"]
description = "Run the Go tests without the coverage check"
run = "go test -v ./..."

[tasks."test:backend:clean"]
description = "Run the Go tests bypassing the test cache"
run = "go test -v -count=1 ./..."

[tasks."test:backend:race"]
description = "Run the Go tests with the race detector enabled"
run = "go test -race ./..."
```

- [ ] **Step 4: Verify test caching behavior**

Run `mise run test:backend` twice:
```bash
mise run test:backend
mise run test:backend
```
Expected: The second run finishes in < 1s with packages showing `(cached)`, and coverage calculation outputs `Go statement coverage: 72.4% (floor 72.0%) coverage: 72.4% clears the floor`.

- [ ] **Step 5: Verify clean test run**

Run:
```bash
CLEAN=1 mise run test:backend
```
Expected: Re-runs tests without caching and passes floor.

- [ ] **Step 6: Commit**

```bash
git add scripts/check-coverage.sh scripts/test-cross-platform.sh mise.toml
git commit -m "perf(test): unlock Go test result cache reuse (LocalRPG-ccz.10)"
```

---

### Task 3: Replace npm Caching with `jdx/aube-action` in GitHub Actions

**Files:**
- Modify: [`.github/workflows/ci.yml`](file:///home/darkliquid/Projects/LocalRPG/.github/workflows/ci.yml)
- Modify: [`.github/workflows/release.yml`](file:///home/darkliquid/Projects/LocalRPG/.github/workflows/release.yml)

- [ ] **Step 1: Update `frontend` and `web` jobs in `.github/workflows/ci.yml`**

In job `frontend`:
Replace the `actions/cache` step for `~/.npm` with `jdx/aube-action@v1`:
```yaml
      - uses: jdx/mise-action@v5

      - name: Setup aube and cache dependencies
        uses: jdx/aube-action@v1
        with:
          version: "2.7.2"
          working-directory: frontend
          run-install: true
          cache: true
```

In job `web`:
Replace the `actions/cache` step for `~/.npm` with `jdx/aube-action@v1`:
```yaml
      - uses: jdx/mise-action@v5

      - name: Setup aube and cache dependencies
        uses: jdx/aube-action@v1
        with:
          version: "2.7.2"
          working-directory: frontend
          run-install: true
          cache: true
```

- [ ] **Step 2: Update `frontend` job in `.github/workflows/release.yml`**

Replace `actions/cache` for `~/.npm` with `jdx/aube-action@v1`:
```yaml
      - uses: jdx/mise-action@v5

      - name: Setup aube and cache dependencies
        uses: jdx/aube-action@v1
        with:
          version: "2.7.2"
          working-directory: frontend
          run-install: true
          cache: true
```

- [ ] **Step 3: Validate workflows with `actionlint`**

Run:
```bash
mise run lint:actions
```
Expected: PASS (0 errors).

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml .github/workflows/release.yml
git commit -m "ci: use jdx/aube-action for node_modules caching (LocalRPG-ccz.10)"
```

---

### Task 4: Add Path Filtering Coordinator and Conditional Job Matrix to CI

**Files:**
- Modify: [`.github/workflows/ci.yml`](file:///home/darkliquid/Projects/LocalRPG/.github/workflows/ci.yml)

- [ ] **Step 1: Add `changes` job to `.github/workflows/ci.yml`**

Add the `changes` job right before `frontend` job:
```yaml
  changes:
    name: Detect changes
    runs-on: ubuntu-latest
    outputs:
      frontend: ${{ steps.filter.outputs.frontend }}
      backend: ${{ steps.filter.outputs.backend }}
      docs: ${{ steps.filter.outputs.docs }}
    steps:
      - uses: actions/checkout@v7

      - uses: dorny/paths-filter@v3
        id: filter
        with:
          filters: |
            frontend:
              - 'frontend/**'
              - 'package.json'
            backend:
              - 'cmd/**'
              - 'pkg/**'
              - 'tools/**'
              - 'scripts/**'
              - 'go.mod'
              - 'go.sum'
              - 'mise.toml'
              - '.github/workflows/ci.yml'
            docs:
              - 'pkg/gui/docs/**'
              - 'docs/**'
              - 'README.md'
              - '.vale.ini'
              - '.rumdl.toml'
              - 'styles/**'
```

- [ ] **Step 2: Add `needs` and `if` conditions to CI jobs**

Update jobs in `ci.yml`:

```yaml
  frontend:
    name: Build the embedded frontend
    needs: changes
    if: github.event_name != 'pull_request' || needs.changes.outputs.frontend == 'true' || needs.changes.outputs.backend == 'true'
    runs-on: ubuntu-latest
    ...

  go:
    name: Go vet and test
    needs: [changes, frontend]
    if: github.event_name != 'pull_request' || needs.changes.outputs.backend == 'true'
    runs-on: ubuntu-latest
    ...

  race:
    name: Go race detector
    needs: [changes, frontend]
    if: github.event_name != 'pull_request' || needs.changes.outputs.backend == 'true'
    runs-on: ubuntu-latest
    ...

  cross-platform:
    name: Cross-platform Go test (${{ matrix.os }})
    needs: [changes, frontend]
    if: github.event_name != 'pull_request' || needs.changes.outputs.backend == 'true'
    runs-on: ${{ matrix.os }}
    ...

  e2e:
    name: End-to-end browser tests
    needs: [changes, frontend]
    if: github.event_name != 'pull_request' || needs.changes.outputs.backend == 'true' || needs.changes.outputs.frontend == 'true'
    runs-on: ubuntu-latest
    ...

  web:
    name: Frontend type check and docs lint
    needs: [changes, frontend]
    if: github.event_name != 'pull_request' || needs.changes.outputs.frontend == 'true' || needs.changes.outputs.docs == 'true'
    runs-on: ubuntu-latest
    ...

  packaging:
    name: Release config and site build
    needs: changes
    if: github.event_name != 'pull_request' || needs.changes.outputs.backend == 'true' || needs.changes.outputs.docs == 'true'
    runs-on: ubuntu-latest
    ...
```

- [ ] **Step 3: Validate workflows with `actionlint`**

Run:
```bash
mise run lint:actions
```
Expected: PASS (0 errors).

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add path-filtering coordinator and conditional job matrix (LocalRPG-ccz.10)"
```

---

### Task 5: Update Documentation and Run Full Verification Suite

**Files:**
- Modify: [`AGENTS.md`](file:///home/darkliquid/Projects/LocalRPG/AGENTS.md)

- [ ] **Step 1: Update `AGENTS.md`**

In `AGENTS.md`:
Update the `Commands` section to document `aube` and `test:backend:clean`, and update the `CI, releases and the showcase site` section explaining the path-filtering and Aube caching design.

- [ ] **Step 2: Run full verification suite locally**

Run:
```bash
mise run lint
mise run test:backend
mise run test:frontend
```
Expected: All linter checks pass, backend statement coverage passes floor, frontend tests pass.

- [ ] **Step 3: Commit documentation**

```bash
git add AGENTS.md
git commit -m "docs: document aube package manager and CI caching design (LocalRPG-ccz.10)"
```

---

### Task 6: Push Branch, Open PR, and Verify GitHub Actions CI

- [ ] **Step 1: Push branch to origin**

```bash
git push -u origin chore/ci-cd-optimization
```

- [ ] **Step 2: Open PR**

```bash
gh pr create --title "perf(ci): accelerate CI and dev with aube, Go test caching, and path filtering" --body "..."
```

- [ ] **Step 3: Watch and verify CI checks**

```bash
gh pr checks <pr> --watch
```
Expected: `changes` job completes, `aube-action` restores/caches dependencies, Go tests succeed, all active checks pass green.

- [ ] **Step 4: Merge PR and close bead**

```bash
gh pr merge <pr> --merge --delete-branch
git checkout main
git pull
bd close LocalRPG-ccz.10 --reason "Done: migrated to aube with jdx/aube-action caching, enabled Go test cache reuse, and added coordinator path-filtering for pull requests"
bd ready
```

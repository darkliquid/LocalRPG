# Accelerate CI/CD and Dev Workflows with Aube, Go Test Caching, and Path Filtering Design

**Date:** 2026-10-18
**Status:** Proposed
**Issue:** `LocalRPG-ccz.10` (TQ-12)
**Epic:** `LocalRPG-ccz` (Tooling, CI and dependency hygiene)
**Scope:** Developer toolchain (`mise.toml`, scripts), package management (`aube`), and GitHub Actions workflows (`.github/workflows/ci.yml`, `.github/workflows/release.yml`).

## 1. Problem

CI run duration and local feedback loops currently suffer from three distinct bottlenecks:

1. **Slow and redundant frontend package installation:**
   - Frontend setup relies on `npm install` and `npm ci` with standard `~/.npm` tarball cache restoration.
   - Restoring and extracting npm dependencies into `node_modules/` takes 20–40 seconds on every runner that requires frontend artifacts or testing.
2. **Bypassed Go test cache:**
   - Both [`scripts/check-coverage.sh`](file:///home/darkliquid/Projects/LocalRPG/scripts/check-coverage.sh) and [`scripts/test-cross-platform.sh`](file:///home/darkliquid/Projects/LocalRPG/scripts/test-cross-platform.sh) execute with `-count=1` and `-shuffle=on`.
   - `-count=1` explicitly instructs the Go test runner to bypass test result caching.
   - `-shuffle=on` introduces a randomized execution seed on every run, modifying command invocation flags and causing Go build/test cache misses even when package contents have not changed.
   - Consequently, all 63 Go packages and statements are recompiled and re-executed from scratch on every run, despite `~/.cache/go-build` being preserved by `actions/cache`.
3. **Unconditional test execution on all pull requests:**
   - Every pull request executes the entire CI matrix regardless of changes: Go vet/test/fuzz, Go race detector, cross-platform matrix on `macos-latest` and `windows-latest`, end-to-end browser tests, frontend unit tests, docs lint, and site build.
   - A pure documentation or frontend edit provisions and runs macOS and Windows runners (~4–5 minutes each), wasting compute and delaying PR turnaround.

## 2. Goals

- **Adopt Aube for frontend package management**:
  - Pin `aube = "2.7.2"` in `mise.toml`.
  - Update `tasks."install:frontend"` to use `aube install`, which preserves `package-lock.json` in place with zero lockfile drift.
  - Employ `jdx/aube-action@v1` with `cache: true` in CI workflows, enabling content-addressable `node_modules` caching and instant dependency restoration.
- **Unlock Go test build and result caching**:
  - Remove `-count=1` and make `-shuffle=on` opt-in across `check-coverage.sh`, `test-cross-platform.sh`, and `mise.toml`.
  - Ensure Go statement coverage analysis (`scripts/check-coverage.sh`) continues to correctly read cached coverage statements and gate on the `72.0%` floor.
  - Provide explicit `CLEAN=1` and `SHUFFLE=1` flags for local developers who want clean or randomized runs.
- **Implement coordinator path-filtering for pull requests**:
  - Add a lightweight (~1s) `changes` job using `dorny/paths-filter@v3` at the start of `.github/workflows/ci.yml`.
  - On pull requests:
    - Skip Go test suites, race detector, and cross-platform matrix when Go backend code is unchanged.
    - Skip frontend tests/lint when frontend code is unchanged.
    - Skip heavy browser tests when only documentation has changed.
  - On pushes to `main`, release tags (`v*`), and manual triggers (`workflow_dispatch`), run the complete matrix unconditionally.

## 3. Non-Goals

- Replacing npm with a new lockfile format (`aube-lock.yaml`, `pnpm-lock.yaml`). Aube directly reads and updates `package-lock.json`, preserving existing contributor workflows and Dependabot npm automation.
- Skipping security or lint scans on modified files (CodeQL, secrets scanning, vulnerability scanning, and releaser checks remain active).
- Skipping tests on `main`. The `main` branch must always run the full test suite to guarantee trunk health and seed fresh cache entries for future PR branches.

## 4. Design & Architecture

### 4.1 Aube Integration

1. **Toolchain (`mise.toml`)**:
   Add `aube` tool version:
   ```toml
   [tools]
   go = "1.27.2"
   node = "26.9.0"
   aube = "2.7.2"
   ```
   Update `install:frontend`:
   ```toml
   [tasks."install:frontend"]
   description = "Install frontend dependencies when package manifests change"
   dir = "frontend"
   run = "aube install"
   sources = ["package.json", "package-lock.json"]
   outputs = ["node_modules/.package-lock.json"]
   ```
2. **GitHub Actions (`ci.yml`, `release.yml`)**:
   Replace legacy `actions/cache` for `~/.npm` with `jdx/aube-action@v1`:
   ```yaml
   - name: Setup aube and install dependencies
     uses: jdx/aube-action@v1
     with:
       version: "2.7.2"
       working-directory: frontend
       run-install: true
       cache: true
   ```
   When `cache: true` is set, `jdx/aube-action` installs with `--frozen-lockfile` and directly caches `frontend/node_modules` keyed against OS, architecture, Node version, aube version, and the hash of `package.json` and `package-lock.json`. Cache hits restore in < 2s.

### 4.2 Go Test Caching

1. **Coverage Script (`scripts/check-coverage.sh`)**:
   Update `go test` invocation to remove `-count=1` and randomize shuffle only when explicitly requested:
   ```bash
   shuffle_flag=""
   [ -n "${SHUFFLE:-}" ] && shuffle_flag="-shuffle=on"
   count_flag=""
   [ -n "${CLEAN:-}" ] && count_flag="-count=1"

   go test -v $count_flag $shuffle_flag -covermode=atomic -coverprofile="$profile" ./...
   ```
   When packages are unchanged, Go outputs:
   ```
   ok  	github.com/darkliquid/localrpg/pkg/turnstream	(cached)	coverage: 89.4% of statements
   ```
   The generated atomic coverage profile retains full statement data, ensuring that `go tool cover -func="$profile"` accurately computes the total coverage without requiring re-execution.
2. **Cross-Platform Script (`scripts/test-cross-platform.sh`)**:
   Remove `-count=1 -shuffle=on`, allowing consecutive runs on macOS and Windows to reuse test artifacts.
3. **Mise Tasks (`mise.toml`)**:
   - `test:backend:fast`: `go test -v ./...`
   - `test:backend:clean`: `go test -v -count=1 ./...`
   - `test:backend:race`: `go test -race ./...`

### 4.3 Change Detection Coordinator in CI

1. **`changes` Job (`dorny/paths-filter@v3`)**:
   Runs as the first step of `ci.yml`:
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
2. **Conditional Matrix**:
   - `frontend` (Build embedded frontend):
     `if: github.event_name != 'pull_request' || needs.changes.outputs.frontend == 'true' || needs.changes.outputs.backend == 'true'`
   - `go` (Go vet and test):
     `if: github.event_name != 'pull_request' || needs.changes.outputs.backend == 'true'`
   - `race` (Go race detector):
     `if: github.event_name != 'pull_request' || needs.changes.outputs.backend == 'true'`
   - `cross-platform` (macOS and Windows):
     `if: github.event_name != 'pull_request' || needs.changes.outputs.backend == 'true'`
   - `e2e` (End-to-end browser tests):
     `if: github.event_name != 'pull_request' || needs.changes.outputs.backend == 'true' || needs.changes.outputs.frontend == 'true'`
   - `web` (Frontend test, lint, docs lint):
     `if: github.event_name != 'pull_request' || needs.changes.outputs.frontend == 'true' || needs.changes.outputs.docs == 'true'`
   - `packaging` (Release config & site build):
     `if: github.event_name != 'pull_request' || needs.changes.outputs.backend == 'true' || needs.changes.outputs.docs == 'true'`

## 5. Verification & Testing

1. **Aube Verification**:
   - Verify `mise install aube` downloads and installs `aube 2.7.2`.
   - Verify `aube install` succeeds cleanly in `frontend/` without modifying `package-lock.json` format.
   - Verify `mise run build:frontend`, `mise run test:frontend`, and `mise run lint:frontend` succeed.
2. **Go Test Caching Verification**:
   - Run `mise run test:backend` twice in succession.
   - Verify that the second run completes in under 1 second, showing `(cached)` for package tests while still reporting and passing the statement coverage floor check.
   - Verify `CLEAN=1 mise run test:backend` forces a complete re-run.
3. **CI Path Filter Verification**:
   - Run `actionlint` locally to validate workflow syntax.
   - Create a pull request to test the CI pipeline on GitHub Actions:
     - Verify `changes` job completes in ~1s.
     - Verify `jdx/aube-action` saves and restores `node_modules` cache.
     - Verify Go test cache saves and restores `~/.cache/go-build`.
     - Verify all active jobs pass green.

## 6. Risks & Mitigation

- **Aube compatibility with npm lockfiles**:
  - *Risk:* Aube changes `package-lock.json` structure or breaks standard npm tooling.
  - *Mitigation:* Aube preserves native `package-lock.json` v2/v3 schemas in place. CI and local environments continue using standard node tools without lockfile changes.
- **Coverage calculation with cached tests**:
  - *Risk:* `scripts/check-coverage.sh` fails to extract coverage totals if test outputs are marked `(cached)`.
  - *Mitigation:* Verified in Go: `go test -coverprofile` writes the complete atomic coverage profile even when packages hit the test cache, and `go tool cover -func` successfully calculates coverage.
- **Skipped checks in GitHub branch protection**:
  - *Risk:* Required status checks in GitHub might block merging if a job is skipped.
  - *Mitigation:* GitHub Actions evaluates conditional jobs (`if:`) with skipped status as successful in branch protection rules. Furthermore, pushes to `main` continue running all checks unconditionally.

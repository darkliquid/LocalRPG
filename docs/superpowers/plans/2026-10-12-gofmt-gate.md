# Format the Go tree and gate on it Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every Go file `gofmt`-clean and fail the build when one is not.
**Architecture:** A single formatting sweep, then a `lint:gofmt` mise task wired into
`mise run lint` and the CI `go` job.
**Tech Stack:** Go 1.27 toolchain (`gofmt`), mise tasks, GitHub Actions.
**Spec:** `docs/superpowers/specs/2026-10-12-gofmt-gate-design.md`

## Global Constraints

- Formatting only: no behaviour change, no rename, no comment edit.
- The gate must run the same command locally and in CI.
- `mise run lint` stays the single entry point for local linting.

---

### Task 1: Sweep the tree

**Files:**
- Modify: 65 files under `cmd/`, `pkg/` and `tools/`

- [x] **Step 1: Record the baseline**

Run: `gofmt -l cmd pkg tools | wc -l`
Result: 65.

- [x] **Step 2: Format everything**

Run: `gofmt -w cmd pkg tools`

- [x] **Step 3: Verify the sweep is complete**

Run: `gofmt -l cmd pkg tools | wc -l`
Expected: 0.

- [x] **Step 4: Verify nothing broke**

Run: `go build ./... && go vet ./... && go test -count=1 ./...`
Expected: build and vet silent; 63 packages `ok`.

- [x] **Step 5: Commit**

Commit as `chore: format the Go tree with gofmt`.

---

### Task 2: Add the gate

**Files:**
- Modify: `mise.toml`
- Modify: `.github/workflows/ci.yml`

- [x] **Step 1: Add the failing check as a task**

Add to `mise.toml`:

```toml
[tasks."lint:gofmt"]
description = "Fail if any Go file is not gofmt-formatted"
run = """
files="$(gofmt -l cmd pkg tools)"
if [ -n "$files" ]; then
  printf 'gofmt: these files are not formatted (run `gofmt -w` on them):\\n%s\\n' "$files"
  exit 1
fi
"""
```

- [x] **Step 2: Verify it passes on the clean tree**

Run: `mise run lint:gofmt`
Expected: exit 0, no output beyond the echoed command.

- [x] **Step 3: Verify it fails on a dirty tree**

Run: `printf 'package probe\n\n\n\nfunc A() {}\n' > pkg/zz_gofmt_probe.go && mise run lint:gofmt`
Expected: `ERROR task failed`, naming `pkg/zz_gofmt_probe.go`.
Then: `rm pkg/zz_gofmt_probe.go`.

- [x] **Step 4: Wire it into `mise run lint`**

Add `lint:gofmt` to the `lint` task's `depends` list.

- [x] **Step 5: Wire it into CI**

In `.github/workflows/ci.yml`, in the `go` job, add before the "Vet" step:

```yaml
      - name: Check formatting
        run: mise run lint:gofmt
```

- [x] **Step 6: Lint the workflow**

Run: `actionlint`
Expected: silent.

- [x] **Step 7: Commit**

Commit as `chore(ci): fail the build on unformatted Go`.

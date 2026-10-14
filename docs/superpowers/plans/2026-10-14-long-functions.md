# The long functions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Work the 71 non-test functions over eighty lines down to none, one package group per pull request, with no behaviour change.
**Architecture:** Each phase of a long function becomes a named function with one job; the original becomes the sequence of its phases. Guards that a scanner models stay inlined at the sink.
**Tech Stack:** Go 1.27, `go/parser` for the measurement, the existing per-package suites as the proof.
**Spec:** `docs/superpowers/specs/2026-10-14-long-functions-design.md`

## Global Constraints

- **Move, do not rewrite.** The same error strings, the same order, the same `defer`s.
- **A guard stays where the sink sees it.** Extracting a containment check into a helper changes what CodeQL counts; the inline check stays and the helper stays behind it.
- **The suite must pass unchanged**, and it is checked before the measurement.
- **One batch per pull request**, so each is reviewable and its own revert.
- The bead stays open across batches; its comment records what has landed.

## The measurement

```go
// The program the "before" and "after" numbers come from. Run it with the
// repository root: go run main.go .
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type fn struct {
	lines int
	file  string
	name  string
}

func main() {
	root := os.Args[1]
	var fns []fn
	fset := token.NewFileSet()
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".worktrees", "node_modules", "frontend", "website", "styles":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		ast.Inspect(f, func(n ast.Node) bool {
			fd, ok := n.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				return true
			}
			lines := fset.Position(fd.End()).Line - fset.Position(fd.Pos()).Line + 1
			if lines > 80 {
				fns = append(fns, fn{lines, rel, fd.Name.Name})
			}
			return true
		})
		return nil
	})
	sort.Slice(fns, func(i, j int) bool { return fns[i].lines > fns[j].lines })
	fmt.Printf("functions over 80 lines: %d\n", len(fns))
	for _, f := range fns {
		fmt.Printf("%d\t%s\t%s\n", f.lines, f.file, f.name)
	}
}
```

**Baseline: 72.** Everything below is measured against that, and `ProcessActionStream` (RD-3's) is in the count but not in the work.

---

### Task 1: `pkg/content` (this batch)

**Files:**
- Modify: `pkg/content/unpack.go`, `pkg/content/provenance.go`, `pkg/content/pack.go`

- [x] **Step 1: Record the count for the package**

Run the measurement, filter for `pkg/content`. Expected: five, from 230 lines down to 87.

- [x] **Step 2: `Unpack` (230) becomes its phases**

`prepareDestination` (validate and create the parent), `openPackageArchive` (gzip in, tar out), `readManifestMember` (the first member, its size caps, parse, and the ID, type and file checks), `declaredFiles` (the safe-path and duplicate index), `extractMembers` (the loop, with `readSignatureMember` and `writeVerifiedMember` for the two things inside it), `missingDeclaredFiles`, and the rename. The signature bytes and the extracted set travel in a small struct rather than as five return values, and the containment check stays inlined in `writeVerifiedMember`.

- [x] **Step 3: `SignPackageFile` (149) becomes its phases**

Building the signature block, writing it into the archive, and the read-back verification are three phases.

- [x] **Step 4: `BuildManifest` (128), `ReadPackageManifestAndSig` (112), `Pack` (87)**

Same treatment, smaller.

- [x] **Step 5: Test**

Run: `go test ./pkg/content/ ./pkg/gui/`
Expected: pass unchanged, including `unpack_test.go`, `provenance_test.go`, `determinism_test.go` and the content import tests in `pkg/gui`.

- [x] **Step 6: Re-measure and verify**

Run: `go vet ./... && mise run test && mise run lint`
Expected: pass, and `pkg/content` no longer appears in the measurement.

---

### Task 2: `cmd/localrpg` (next)

**Files:** `runGUICommand` (203), `runPlayCommand` (167), `runExportCommand` (143), `runDebugCommand` (120), `runTTSBatchCommand` (115), `runRegistryInstall` (87)

The command bodies are phases by construction: parse, build what the command needs, run, report. `runGUICommand`'s three modes (TCP, socket, Wails) are three functions behind one dispatch.

### Task 3: `pkg/gui` route handlers

**Files:** `handleWorldRoutes` (188), `handleGameTurnRoute` (161), `handleGameEntityRoute` (105). Each is one function per sub-resource, the way RD-2 split the game routes.

### Task 4: `pkg/gui` service methods

**Files:** `ImportContentWithOptions` (206), `TestProvider` (191), `prepareTurn` (179), `Run` (179), `CreateGame` (119), `GetCharacterPortrait` (114), `runExport` (111), `MergeEntities` (102), `GenerateWorld` (94), `GetGameState` (92), `GenerateText` (89), `GenerateCharacter` (86), `StartTTSBatch` (86), `Renegotiate` (82).

### Task 5: `pkg/engine`

**Files:** `runGenerationLoop` (416, after RD-3), `stageEntities` (173), `InitGame` (161), `stream` (117), and the three in the 80s.

### Task 6: `pkg/harness`

**Files:** `relevantHistory` (138), `fitToBudget` (108), `RouterFromConfigWithLogger` (100), `FormatMechanicsInstructions` (93).

### Task 7: `pkg/provider`

**Files:** `GenerateImage` (185 and 116, two providers), `streamOnce` (175), `buildContents` (125), `callInteractions` (115), `Synthesize` (109), `formatKokoroVoice` (101), `parsePrompt` (94), `ListVoices` (93).

### Task 8: `pkg/export`

**Files:** `writeFrames` (198), `Compile` (126), `Export` (99), `RenderVideo` (82).

### Task 9: the singles

**Files:** `pkg/config` (3), `pkg/rules` (2), `pkg/sysgen` (2), `pkg/tools` (2), and one each in `pkg/ingest`, `pkg/media`, `pkg/models`, `pkg/registry`, `pkg/scene`, `pkg/storage`, `pkg/driver`, `pkg/entity` and `tools/sitegen`.

### Task 10: the gate

- [x] **Step 1: Make the measurement a check**

A test that fails when a function crosses eighty lines, run with the suite, so the list cannot grow back. The same instinct as the `interface{}` rule and the docs guards.

- [x] **Step 2: Record why any function is exempt**

If an entry turns out to read well over the line, its reason goes in the check's table rather than in silence.
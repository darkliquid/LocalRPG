# The CLI command shapes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every command in `cmd/localrpg` has a testable `runXCommand(args, stdout, stderr) int` behind a wrapper that exits, so the process is reached from two places and a test can capture what a command says.
**Architecture:** Nine command files gain the pair `handleXCommand`/`runXCommand` that five already have; `os.Exit` becomes a return, writes go to the writers the command was given, and flag sets use `flag.ContinueOnError`.
**Tech Stack:** Go 1.27, `flag`, `io.Writer`, the standard test library.
**Spec:** `docs/superpowers/specs/2026-10-14-cli-command-shapes-design.md`

## Global Constraints

- No command changes what it does. Only where its output goes and who exits.
- The process is reached from `main` and the wrappers, and nowhere else in a command body.
- `main` keeps calling `handleXCommand(args[1:])`, so `main.go` barely changes.
- A test asserts the *shape* for every converted command: it calls `runXCommand` with buffers and checks the code and the bytes.
- Build after each file. The conversions are independent, and a build failure should point at one of them.

## File Map

| File | Shape |
|---|---|
| `cmd/localrpg/roll.go` | 2 exits, 6 output sites |
| `cmd/localrpg/prompt.go` | 2 exits, 3 output sites, one `ExitOnError` |
| `cmd/localrpg/media.go` | 5 exits, 7 output sites (tts and image) |
| `cmd/localrpg/play.go` | 9 exits, 11 output sites, the discarded load error |
| `cmd/localrpg/gui.go` | 3 exits, 12 output sites, the `shutdown` closure |
| `cmd/localrpg/export.go` | 13 exits, 22 output sites, one `ExitOnError` |
| `cmd/localrpg/debug.go` | 6 exits, 15 output sites |
| `cmd/localrpg/*_test.go` | new: one test per converted entry point |

---

### Task 1: The smallest two, to settle the shape

**Files:**
- Modify: `cmd/localrpg/roll.go`, `cmd/localrpg/prompt.go`
- Create: `cmd/localrpg/roll_command_test.go`

- [ ] **Step 1: Write the tests first**

Call `runRollCommand([]string{"1d20"}, &out, &errOut)` with buffers and assert the code and that the result landed in `out`. It fails to compile, which is the point.

- [ ] **Step 2: Convert both files**

The pair, the returns, the writers. `prompt.go`'s flag set moves to `ContinueOnError` with `SetOutput(stderr)`.

- [ ] **Step 3: Test and build**

Run: `go build ./... && go test -run 'TestRunRoll' ./cmd/localrpg/`
Expected: pass.

---

### Task 2: The media commands

**Files:**
- Modify: `cmd/localrpg/media.go`
- Create: `cmd/localrpg/media_command_test.go`

- [ ] **Step 1: Convert**

`tts` and `image` share the file. `ErrNoBatchAPI` and the other sentinels from the mapper come back through the same writers.

- [ ] **Step 2: Test the usage path, which needs no provider**

An argument-less `runTTSCommand` writes usage to the buffer and returns 1.

---

### Task 3: `play`

**Files:**
- Modify: `cmd/localrpg/play.go`

- [ ] **Step 1: Convert**

Nine exits become returns, and the writers thread through to the TUI program.

- [ ] **Step 2: Handle the load error**

`cfg, _ := cfgMgr.Load()` becomes the check its neighbours have, reporting to `stderr` and returning 1.

- [ ] **Step 3: Build**

Run: `go build ./...`

---

### Task 4: `gui`

**Files:**
- Modify: `cmd/localrpg/gui.go`

- [ ] **Step 1: Convert**

- [ ] **Step 2: Move the drain into a defer**

The `shutdown` closure exists because `os.Exit` skips defers. With returns, the same work belongs in a `defer` in the run function.

- [ ] **Step 3: Build**

Run: `go build ./...`

---

### Task 5: `export`

**Files:**
- Modify: `cmd/localrpg/export.go`
- Create: `cmd/localrpg/export_command_test.go`

- [ ] **Step 1: Convert the flag mode and the shape**

Thirteen exits, and the writers through to the web and video exporters, which take them.

- [ ] **Step 2: Test a bad flag**

It writes usage to the buffer and returns 1, rather than the standard library exiting 2.

---

### Task 6: `debug`

**Files:**
- Modify: `cmd/localrpg/debug.go`

- [ ] **Step 1: Convert**

Six exits. The embedded debug servers keep their own streams, which is out of scope.

- [ ] **Step 2: Build**

Run: `go build ./...`

---

### Task 7: Verify

**Files:** none

- [ ] **Step 1: The process is reached from two places**

Run: `grep -rn 'os.Exit' cmd/localrpg/*.go | grep -v _test`
Expected: `main.go` and the nine wrappers, nothing inside a command body.

- [ ] **Step 2: Every command has the shape**

Run: `grep -c 'func run[A-Za-z]*Command(args \[\]string, stdout, stderr io.Writer) int' cmd/localrpg/*.go`
Expected: fourteen.

- [ ] **Step 3: The whole suite and the gate**

Run: `mise run test && mise run lint`
Expected: both pass.

- [ ] **Step 4: Commit**

Commit as `refactor(cli): give every command one testable shape`.

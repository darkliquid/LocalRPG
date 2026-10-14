# The CLI command shapes Design

**Date:** 2026-10-14
**Status:** Proposed
**Issue:** [#178](https://github.com/darkliquid/LocalRPG/issues/178)
**Epic:** [#151](https://github.com/darkliquid/LocalRPG/issues/151)
**Proposal:** `docs/architecture/quality-review-2026-10-12.md` (RD-12, finding R-13)
**Scope:** `cmd/localrpg`, one entry point per command. No change to what any command does.

## 1. Problem

There are two shapes in `cmd/localrpg`, and which one a command has decides whether it can be tested:

```go
// Five commands: content, config, publisher, registry, styles.
func handleContentCommand(args []string) {
	code := runContentCommand(args, os.Stdout, os.Stderr)
	if code != 0 {
		os.Exit(code)
	}
}
func runContentCommand(args []string, stdout, stderr io.Writer) int { ... }

// Nine commands: roll, prompt, play, tts, image, gui, export, debug.
func handleRollCommand(args []string) {
	...
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}
```

The second shape reaches the process directly, so the command's behaviour can only be observed by starting a subprocess and reading its streams. That is why finding T-4 counts every CLI entry point among the functions at 0% coverage, and why TC-2 ("test the CLI entry points") cannot be written yet: there is nothing to call.

Three smaller inconsistencies ride along, and the review names all three:

- **Flag error modes differ.** `export` and `prompt` build their flag sets with `flag.ExitOnError`, which prints a usage message and calls `os.Exit(2)` from inside the standard library; the other eleven use `ContinueOnError` and write usage to the command's own stderr.
- **`play` discards a load error.** `cfg, _ := cfgMgr.Load()` and then `cfg.Telemetry`, where every other command checks the error. It cannot nil-deref today only because `Load` happens never to return one, which section 3 records.
- **51 `os.Exit` calls** are spread through the command bodies, so the exit point of a command is wherever the failure happened to be.

## 2. Goals

- One shape: `runXCommand(args []string, stdout, stderr io.Writer) int` behind a `handleXCommand` that exits.
- The process is reached from two places only: `main`, and the nine one-line wrappers.
- A command's output goes to the writers it was given, so a test can capture it.
- One flag mode, with usage on the command's own stderr and a failure code a test can assert.
- A command that cannot load its configuration says so instead of carrying on.

## 3. Non-goals

- **The command tests themselves.** This proposal makes the entry points callable; TC-2 owns the table that exercises them. A handful are tested here to prove the shape is real.
- **Making `ConfigManager.Load` report a malformed file.** It cannot fail today: it starts from `DefaultConfig()` and discards the YAML error, so a user with a broken `config.yaml` silently gets defaults. That is a real defect and a behaviour change for every command, so it is its own bead rather than a rider on this one. The `play` fix here is the defensive check its neighbours already have.
- **The TUI and the GUI's own lifecycles.** `play` hands the terminal to Bubbletea and `gui` hands it to Wails; both keep doing that, with the writer they were given where the library accepts one.
- **`main`'s own four exits.** Dispatching and the `--version` flag are the process's business.

## 4. Design

### The shape

Every command file ends up with the pair `content.go` already has:

```go
func handleRollCommand(args []string) {
	code := runRollCommand(args, os.Stdout, os.Stderr)
	if code != 0 {
		os.Exit(code)
	}
}

func runRollCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "Usage: localrpg roll <notation>")
		return 1
	}
	...
}
```

`main` is unchanged: it still calls `handleXCommand(args[1:])`. The wrapper is the only place in a command that exits.

### Inside a command

- `os.Exit(n)` becomes `return n`.
- A write to `os.Stdout` or `os.Stderr` becomes a write to `stdout` or `stderr`, and writes through `fmt.Printf`/`fmt.Println` become `fmt.Fprintf(stdout, ...)`. Helpers that print take the writer they write to, as `runContentExport` already does.
- Where a library owns the terminal, it is given the writer: Bubbletea takes `tui`'s own program options and the GUI is unaffected because it renders in its own window.

### Flags

One mode, everywhere:

```go
fs := flag.NewFlagSet("export", flag.ContinueOnError)
fs.SetOutput(stderr)
...
if err := fs.Parse(flags); err != nil {
	return 1
}
```

`ContinueOnError` keeps the process out of the standard library's hands: the usage text goes to the command's stderr, the code is the command's to choose, and a test can assert both. The two `ExitOnError` command files change to it; the eleven already using it gain nothing but consistency, and are left alone.

### Exit codes

`0` for success, `1` for a failure the command reported, and the same `1` for a bad flag, which is what the eleven `ContinueOnError` sites already do. Nothing in the repository switches on `2`, so no caller changes.

### The dynamic block in `gui`

`handleGUICommand` has a `shutdown` closure that drains the service, shuts telemetry down and then calls `os.Exit`, because `os.Exit` skips deferred calls. Under the new shape the drain moves to a `defer` inside `runGUICommand` and the exits become returns, so the same work happens on the same paths with the process reached once, from the wrapper.

## 5. Behaviour

None that a user can observe, with two exceptions, both deliberate:

- **A bad flag on `export` or `prompt`** was handled by the standard library, which printed to the process's stderr, exited `2`, and ignored the command's own writers. It is now the command's: usage on the command's stderr, exit `1`.
- **`play` with an unloadable configuration** reports the error and exits `1` rather than dereferencing whatever `Load` returned. `Load` returns an error today only in principle, which is why section 3 files that separately.

## 6. Testing

- **A test per converted entry point**, calling `runXCommand` with buffers and asserting the code and what was written: this is the property the proposal exists to establish, and each one fails to compile before the change.
- **A flag error is a return, not a process exit**, asserted for one command that changed mode and one that already used `ContinueOnError`.
- **`play`'s load error** is exercised by the command reporting it, which needs `Load` to be able to fail; until then the test asserts the happy path and the check is recorded as defensive.
- `go vet`, `mise run test`, `mise run lint`.

## 7. Rollout

One commit, file by file, with a build after each: `roll`, `prompt`, `media`/`tts`, `play`, `gui`, `export`, `debug`. The conversions are independent, so the order is about the size of the diff rather than about correctness.

## 8. Risks

- **A mechanical conversion of 95 output sites and 51 exits** is exactly the kind of change that slips a behaviour. The mitigation is that the tests capture the writers: a write that still goes to `os.Stdout` is invisible to the test that captures `stdout`, so the shape is checked rather than assumed.
- **`play` and `gui` own a terminal.** Their conversions are the least mechanical, because the library, not the command, decides where the bytes go. Both are left owning it, and the writers are passed where the library takes one.
- **Two commands keep their own stream use**: `main`'s usage and `debug`'s embedded servers. Both are outside the command bodies this proposal is about.

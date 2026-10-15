# Report a Malformed Config File Design

**Date:** 2026-10-15
**Status:** Proposed
**Issue:** gh-175 (the bead LocalRPG-4kz; found while giving the CLI its one shape, RD-12)
**Scope:** `pkg/config/manager.go`, five call sites in `cmd/localrpg/`. No schema change.

## 1. Problem

`ConfigManager.Load` cannot fail on a malformed file:

```go
merged := DefaultConfig()
if data, err := os.ReadFile(m.userConfigPath); err == nil {
    _ = yaml.Unmarshal(data, merged)          // the error is discarded
}
if data, err := os.ReadFile(m.localConfigPath); err == nil {
    if err := yaml.Unmarshal(data, merged); err == nil {
        m.isOverride = true                    // the error only gates isOverride
    }
}
...
return m.activeConfig, nil                     // always nil
```

So a user with a broken `config.yaml` or `localrpg.yaml` gets defaults and no message. Two further
consequences:

1. **The commands that check the error check something that cannot fail.** `media`, `offline`,
   `play`, `provenance`, `registry` and `ttsbatch` all write `Error loading configuration: %v` and
   exit 1, and never reach it.
2. **A malformed file can be half-applied.** `yaml.Unmarshal` sets the fields it reaches before it
   fails, so the defaults the caller falls back on are not defaults at all: they are defaults plus
   an arbitrary prefix of the broken file.

The paths that discard the error cannot be blamed for discarding it, because there is nothing to
check.

## 2. Goals

- `Load` returns an error that **names the file** and the parse failure, for each malformed file.
- **A malformed file applies nothing**, rather than a prefix of itself.
- The failure is **visible even to a caller with no error channel**: the GUI's service and the
  export compiler build the manager and use it without one.
- The five command paths that discard the error learn to report it.
- No change for a well-formed configuration.

## 3. Non-goals

- **Validating the contents.** `Validate()` already does that and its findings are warnings on
  purpose: an unknown provider or an absent voice is a configuration to correct, not a file that
  cannot be read. A malformed file is the latter.
- **A missing or empty file.** Absent is the ordinary first run; empty parses to nothing and layers
  nothing. Neither is an error.
- **Changing the merge order**, the write path, or `DetectConfigFile`.
- **Making the GUI fail to start.** `gui.NewService` has no error to return at that point (it is
  mid-construction, and the app is expected to open so the user can fix the file in Settings). Its
  channel is the warnings list, which the Settings payload already carries and the service already
  logs.

## 4. Design

### The load contract

```go
// Load reads the user configuration, then a local override when one is there, and
// returns the layered result. A file that does not parse is reported: the error
// names it, and the same failure is appended to Warnings so a caller with no error
// channel still surfaces it. The configuration returned is always usable and holds
// what did parse, so a caller that sees the error must report it rather than
// quietly use the defaults it fell back to.
func (m *ConfigManager) Load() (*Config, error)
```

Returning the usable configuration alongside the error is deliberate. Eight call sites in the GUI,
the export compiler and the CLI depend on the returned config being non-nil, and turning those into
nil checks is a larger change than the bug deserves. What changes is that the error exists, names
the file, and reaches the user through whichever of the two channels the caller has.

### One file, parsed once and applied only if it parses

```go
// mergeConfigFile reads one configuration file over cfg. It parses the document
// into a throwaway Config first, so a file that does not parse leaves cfg exactly
// as it was: the alternative is a configuration built from the prefix of a file
// that yaml managed to read before it gave up.
func mergeConfigFile(path string, cfg *Config) error
```

The parse happens twice, which for a file of this size is not worth avoiding: the first pass is the
validation, the second cannot fail.

### What each caller does

| Caller | Today | After |
|---|---|---|
| `media`, `offline`, `play`, `ttsbatch` | writes the error and exits 1, never reached | the same line, now reached: `Error loading configuration: <path>: yaml: ...` |
| `registry`, `provenance`, `styles` (five sites) | discards the error, uses the config | reports it on stderr and exits 1 |
| `gui.NewService`, `export.NewScriptCompiler` | discards the error | unchanged: the failure is in `Warnings()`, which the service logs as `config.problem` and the Settings payload returns |

The five command sites are the ones where a broken configuration changes the command's answer
(which registries to search, which publishers to trust, which style packs apply), so silently using
defaults there is the worst version of the bug.

## 5. Behaviour

- A malformed file now names itself, and the user can fix it: `<path>: yaml: line 3: ...`.
- A malformed file no longer leaks a prefix of itself into the configuration.
- The three commands that discarded the error now exit non-zero instead of answering from defaults.
- A well-formed configuration loads exactly as before, including the local override marking
  `isOverride`.
- `Warnings()` gains a line only when a file failed to parse, so the GUI's Settings warnings list
  and the `config.problem` log line carry it.

## 6. Testing

- **`pkg/config`**: a malformed user config returns an error naming the path and appears in
  `Warnings()`; a malformed local override is reported and does not set `isOverride`; a file whose
  first field parses and second does not leaves the first at its default, which is the
  half-application rule; a well-formed pair still layers.
- **`cmd/localrpg`**: the command paths keep their coverage; the config package's test is what
  proves the error exists, and the commands' existing `LOCALRPG_CONFIG_DIR` guard keeps a test run
  away from the developer's own configuration.
- `go vet ./...`, `mise run test`, `mise run lint`.

## 7. Rollout

One pull request, one commit. No migration: the change only makes a failure visible that was
silent, and a configuration that was readable stays readable.

## 8. Risks

- **A caller that ignores the error still proceeds on defaults.** That is the status quo for the
  GUI and the export compiler, and the warnings channel is what tells the user. Every command path
  that can act on it now does.
- **The double parse.** Two `yaml.Unmarshal` calls per file, on a file of a few hundred lines, once
  per process (and once per GUI settings read). Acceptable; the alternative is a merge that has to
  unwind a partial application.
- **A test that resolves the real configuration.** `cmd/localrpg`'s `TestMain` redirects
  `LOCALRPG_CONFIG_DIR` and refuses to run a test that escapes it; a new test in `pkg/config`
  builds the manager with explicit paths, as the existing ones do.
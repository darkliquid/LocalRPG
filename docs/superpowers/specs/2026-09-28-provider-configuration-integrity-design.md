# Provider Configuration Integrity Design

**Date:** 2026-09-28
**Status:** Proposed
**Scope:** Stop a failed provider build from silently becoming an echo bot, validate role configuration at load, correct provider descriptors that misreport their transport, retire the misleading `mock` type, cross-validate the two preset tables, and surface every problem to the user
**Related:** `pkg/harness` (`factory.go`, `router.go`), `pkg/config` (`types.go`, `manager.go`, `presets.go`), `pkg/provider` (descriptors, `all/` tests), `pkg/gui` (settings surface), `frontend/` (SettingsStudio); builds on `docs/superpowers/specs/2026-09-24-provider-capability-model-design.md` and `docs/superpowers/specs/2026-09-28-provider-key-identity-design.md`

## 1. Overview & Goals

Three defects let a broken provider configuration masquerade as a working one:

1. `RouterFromConfigWithLogger` discards a per-role build error with `continue`
   (`pkg/harness/factory.go:163-165`). If the failed role was `gm`, it then
   quietly installs `default-echo` for `gm` (`factory.go:186-189`). A typo in an
   endpoint or an unknown `type` therefore produces an echo bot, not an error.
2. `Config.Validate()` only checks `providers.prices` keys
   (`pkg/config/types.go:296-315`). Agent role entries are never validated, so an
   unknown `type`, a missing `endpoint`, or a missing `command` is accepted at
   load and only surfaces (if at all) at turn time.
3. Config presets exist twice: `config.AgentPresets`/`TTSPresets`/`STTPresets`/
   `ImagePresets` (`pkg/config/presets.go`) and each descriptor's `Presets`
   (e.g. `pkg/provider/openaichat/openaichat.go:21`). Only the descriptor copy is
   drift-checked (`pkg/provider/all/presets_test.go`), so the two can silently
   disagree.

Two metadata defects compound the confusion:

4. `tts:elevenlabs` declares `Source: "builtin"`
   (`pkg/provider/ttselevenlabs/ttselevenlabs.go:19`) while it is a metered cloud
   HTTP adapter. Any source-filtered catalogue or UI is wrong.
5. The `"mock"` provider type is advertised in `ProviderConfig.Type`
   (`pkg/harness/types.go:127`) and accepted by `NewModelProvider`
   (`factory.go:48`), but no mock implementation exists: an unknown builtin name
   falls through to `builtinEchoModelProvider` (`factory.go:51-54`). The type is
   a trap.

**Goals:**

- A configured role that cannot be built must produce a visible, attributable
  failure, never a silent echo.
- The distinction between "gm is intentionally unconfigured" and "gm is
  configured but broken" must be explicit.
- Role configuration is validated at load and save; problems are surfaced where
  the user edits them.
- Every descriptor's `Source` matches its actual transport, with a test that
  enforces it.
- The `mock` type is either real or gone.
- The two preset tables are cross-validated so they cannot drift.

**Non-Goals:**

- Adding new providers or changing provider key grammar (owned by the
  provider-key-identity spec).
- Auto-repairing configuration.
- Changing the echo provider's behaviour when it is genuinely selected.

**Success Criteria:**

- A config whose `gm` role has an unknown `type` fails the turn with a
  `provider_unavailable` failure naming the role and the build error, and does
  **not** echo.
- `Config.Validate()` reports an unknown type, a missing HTTP endpoint, a missing
  CLI command, and an unknown builtin name for every role entry.
- `test-provider` and the config-save response both carry validation problems.
- `provider.List()` contains no descriptor whose `Source` contradicts its
  transport, enforced by a test.
- `grep` for `"mock"` in provider type handling returns nothing.
- A test asserts every `config` preset has a matching, equal descriptor preset
  and vice versa.

## 2. Investigation Findings

- **Build errors are swallowed.** `factory.go:140-184`: the per-role loop calls
  `NewModelProviderWithLogger`; on error it `continue`s (`:163-165`). The only
  error the function can return is when `cfg` is nil. The echo default is then
  installed if `GetProviderForRole(RoleGM)` fails (`:186-189`), which conflates
  a missing gm with a broken gm.
- **Unknown types are already errors at the leaf.**
  `NewModelProvider` returns `fmt.Errorf("unknown model provider type: %s", …)`
  (`factory.go:55-57`); the error just never reaches a caller.
- **Validation is narrow.** `Config.Validate() []string`
  (`pkg/config/types.go:296`) only checks `providers.prices`. `ConfigManager.Load`
  stores the result in `m.warnings` (`pkg/config/manager.go:106`), exposed by
  `Warnings()` (`manager.go:32-38`). The GUI only logs them
  (`pkg/gui/service.go:137`); they are never shown to the user.
- **Router construction sites:** `pkg/gui/runtime.go:70`, `pkg/gui/service.go:1179`
  and `pkg/gui/service.go:1632`. The first two treat the error as fatal for the
  turn; the third ignores the router when construction fails.
- **Preset duplication.** `config.AgentPresets`/`TTSPresets`/`STTPresets`/
  `ImagePresets` (`pkg/config/presets.go:3-296`) map preset IDs to typed configs;
  descriptors carry the same presets as `map[string]interface{}`
  (`pkg/provider/descriptor.go:55-61`). `TestEachFamilyHasPresets` and
  `TestEveryPresetConfigUnmarshals` (`pkg/provider/all/presets_test.go:13,31`)
  check the descriptor copy only. `config.Get*Preset` accessors exist
  (`presets.go:297-313`).
- **Metadata drift.** `tts:elevenlabs` `Source: "builtin"`
  (`ttselevenlabs.go:19`) though `ttselevenlabs/client.go` performs HTTP. The
  config preset mirrors the same wrong framing (`Presets` id `elevenlabs`,
  `"type": "builtin", "builtin_name": "elevenlabs"`), and `Source` is what the
  docs catalogue prints (`pkg/gui/docs_catalogue_test.go:82`). Also
  `tts:native-os` describes `spd-say`/`say`/PowerShell but also invokes
  `espeak-ng` (`pkg/provider/ttsnativeos/native.go:37`).
- **The `mock` type.** `ProviderConfig.Type` comment lists `"mock"`
  (`pkg/harness/types.go:127`); `NewModelProvider` accepts `"mock"`
  (`factory.go:48`); `KeyFor` treats `"mock"` like `"builtin"`/`""`
  (`pkg/harness/exports.go:20`). An unmatched name returns the echo provider
  (`factory.go:51-54`). No mock implementation exists outside tests.
- **Default roles.** `DefaultConfig` sets `gm: cli echo`, `narrator: disabled`,
  and `extractor`/`completion: inherit gm` (`pkg/config/types.go:364-383`), so the
  builtin echo fallback is not what the shipped default uses.

## 3. Design

### 3.1 Fail-loud router build

Record failures on the router instead of dropping them, and make the echo
fallback intent-explicit.

```go
// pkg/harness
// RoleBuildError records a configured role whose provider could not be built.
type RoleBuildError struct {
    Role   string
    Type   string
    Name   string // builtin_name or command, when set
    Err    error
}

// BuildErrors returns the per-role build failures from the last configuration
// this router was built from. Empty when every configured role built.
func (r *Router) BuildErrors() []RoleBuildError
```

In `RouterFromConfigWithLogger`:

- On `NewModelProviderWithLogger` error, append a `RoleBuildError` and continue.
- Track whether a `gm` role was *configured* (present in `cfg.Agents.Roles` and
  not `inherit`). Install `default-echo` only when `gm` was **not** configured.
  When `gm` **was** configured and failed, leave the role unassigned so the turn
  fails loudly.
- Keep the signature `(*Router, error)`; return a nil error for role failures so
  callers keep the usable router and read `BuildErrors()`.

At turn preparation (`pkg/gui/runtime.go:70`, `pkg/gui/service.go:1179`), after
building the router, inspect `router.BuildErrors()` for the active role
(`gm` for turns, the extractor role for extraction). If present, fail the turn
with a `harness.GenerationFailure`:

```go
harness.GenerationFailure{
    Code:    harness.FailureProviderUnavailable,
    Message: fmt.Sprintf("gm provider failed to build: %v", buildErr.Err),
}
```

This turns the current silent echo into the same `model_missing`/failure surface
the frontend already renders.

### 3.2 Validate role configuration at load

Extend `Config.Validate()` to walk `Agents.Roles` (and the media families) and
report, per entry:

- `type` is one of the accepted values (`""`, `builtin`, `cli`, `http`,
  `gemini`, `disabled`, `inherit` for LLM; the media equivalents).
- `type: http`/`gemini` ⇒ non-empty `endpoint` (http) or a resolvable key
  requirement; `cli` ⇒ non-empty `command`; `builtin` ⇒ `builtin_name` resolves
  to a registered descriptor, otherwise report it.
- `inherit` on `gm` (the root role) is invalid.

Resolution uses `harness.KeyFor`/`media.TTSKeyFor`/`STTKeyFor`/`ImageKeyFor`
rather than re-implementing the mapping, so validation and construction cannot
disagree. Unknown types reuse the existing error text from `NewModelProvider`.

Warnings remain non-fatal: they are collected, not thrown, matching the existing
contract (`types.go:292-295`).

### 3.3 Surface warnings in the GUI

- Add `Service.ConfigWarnings() []string` returning
  `configMgr.Warnings()` plus any `router.BuildErrors()` from the current
  runtime.
- Include `warnings []string` in the config-save response DTO and in the
  bootstrap config DTO so SettingsStudio can show a dismissible banner after a
  save, with each problem naming the field path.
- Replace the log-only loop at `pkg/gui/service.go:137` with the same source.

### 3.4 Correct descriptor metadata, enforced

- Change `tts:elevenlabs` `Source` to `"http"` (or a new `"cloud"` value if a
  distinction from self-hosted HTTP is wanted; prefer reusing `"http"` to avoid
  schema churn).
- Correct the `tts:native-os` description to include `espeak-ng`.
- Add a test in `pkg/provider/all` that every descriptor's `Source` matches its
  transport: `builtin` for in-process adapters, `http` for `net/http` adapters,
  `cli` for `os/exec` adapters, with an allowlist for the few that legitimately
  mix (documented in the test).

### 3.5 Retire the `mock` type

Remove `"mock"` from the accepted types in `NewModelProvider` and from the
`ProviderConfig.Type` comment. The debug echo remains reachable through the
documented `cli echo` default and, if desired, described explicitly in the
catalogue. Update `KeyFor` to drop the `"mock"` case. Anyone currently writing
`type: mock` silently got echo; after this change they get a validation warning
telling them so.

### 3.6 Cross-validate the preset tables

Extend `pkg/provider/all/presets_test.go` (or add a `pkg/config` test that
imports `pkg/provider/all`) to assert, per family:

- Every `config.<Family>Presets` ID has a descriptor preset with the same ID.
- The descriptor preset's `Config` unmarshals into the typed config and equals
  the `config` preset value (compare after normalising empty-vs-unset).
- Every descriptor preset ID also appears in the `config` table, or is
  deliberately marked catalogue-only.

A single generated source of truth is the better long-term fix but is a larger
refactor; the cross-check closes the drift now and can be replaced later.

## 4. Interfaces

```go
// pkg/harness
type RoleBuildError struct { Role, Type, Name string; Err error }
func (r *Router) BuildErrors() []RoleBuildError

// pkg/config
func (c *Config) Validate() []string            // extended, same contract

// pkg/gui
func (s *Service) ConfigWarnings() []string
// ConfigDTO gains: Warnings []string `json:"warnings,omitempty"`
```

No new HTTP routes. `POST /api/settings/test-provider` keeps its shape; the
config-save response gains `warnings`.

## 5. Error Handling

| Situation | Today | After |
| --- | --- | --- |
| Configured `gm` fails to build | Echo bot, no signal | Turn fails `provider_unavailable`; warning on save |
| `gm` not configured at all | Echo bot | Unchanged (intentional debug default) |
| Unknown role `type` | Ignored at load | Validation warning; build error at runtime |
| `http` role without endpoint | Ignored at load | Validation warning |
| `cli` role without command | Echo at runtime | Validation warning |
| `type: mock` | Echo | Validation warning naming the retired type |
| Price key malformed | Warning (logged only) | Warning (shown) |

Warnings never block a save or a load; they inform.

## 6. Testing & Verification

Go (stdlib `testing`, `t.TempDir()`):

- `pkg/harness`: a router built from a config with an unbuildable `gm` returns a
  `BuildErrors` entry and does **not** assign the echo to `gm`; a config with no
  `gm` role installs the echo.
- `pkg/harness`: `NewModelProvider` returns an error for `type: "mock"` and for
  an unknown builtin name.
- `pkg/config`: `Validate()` reports the four role problems above and still
  reports price-key problems.
- `pkg/provider/all`: the `Source`-vs-transport test fails for a deliberately
  wrong descriptor (table-driven negative case) and passes for the catalogue.
- `pkg/provider/all`: the preset cross-check flags a mutated preset.
- `pkg/gui`: `ConfigWarnings()` includes a build error for a broken `gm`; the
  config-save response carries it.

Frontend: `tsc` only. The Settings banner is verified by inspection against the
new DTO field.

## 7. Compatibility & Rollout

- Returning `BuildErrors` on the router is additive and does not change existing
  call sites' control flow; only the two turn-preparation sites gain a check.
- `type: mock` removal is a hard cut for a type that never had an
  implementation; a validation warning eases the transition.
- Changing `tts:elevenlabs` `Source` changes generated documentation only; the
  docs regeneration task (`go test ./pkg/gui -update-docs`) must be run.
- Preset cross-validation may surface real existing disagreements; each is fixed
  in the same change or explicitly allowlisted with a reason.

## 8. Open Questions

- Should a failed `gm` block the turn outright, or degrade to a deterministic
  mention-only extraction path with a visible banner?
- Is `Source` the right discriminator, or should the transport be derived from
  descriptor metadata instead of hand-declared?
- Should warnings be per-field structured objects (so Settings can highlight
  inputs) rather than strings?
- Do we generate the preset tables from descriptors now, or keep the
  cross-check as the interim guard?

## 9. References

- Code: `pkg/harness/factory.go:44-189`, `pkg/harness/exports.go:9-33`,
  `pkg/harness/router.go`, `pkg/config/types.go:296`, `pkg/config/manager.go:32,106`,
  `pkg/config/presets.go`, `pkg/provider/all/presets_test.go`,
  `pkg/provider/ttselevenlabs/ttselevenlabs.go:19`,
  `pkg/provider/ttsnativeos/native.go:37`, `pkg/gui/service.go:137,1179,1632`.
- Specs: `2026-09-24-provider-capability-model-design.md`,
  `2026-09-28-provider-key-identity-design.md`,
  `2026-09-26-provider-error-surfacing-design.md`.

# Offline Preset Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#35 LF-4](https://github.com/darkliquid/LocalRPG/issues/35)
**Epic:** [#17 Zero-GPU and local-first offerings](https://github.com/darkliquid/LocalRPG/issues/17)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §2 (LF-4)
**Depends on:** [#27 MP-1](https://github.com/darkliquid/LocalRPG/issues/27), [#34 LF-3](https://github.com/darkliquid/LocalRPG/issues/34)
**Scope:** `pkg/config`, `pkg/provider`, `pkg/gui`, `cmd/localrpg`, `frontend`

---

## 1. Problem

A user who wants zero network calls must assemble the stack by hand: set the GM role to the oracle,
pick a local TTS, choose the procedural image generator, select a local embedding provider. Nothing
does this in one action, and nothing **verifies** it: after configuring, the user has no way to know
whether some provider still reaches the network.

The pieces exist (the oracle, `tts:native-os`, `tts:sherpa-onnx`, `image:procedural-art`,
`embedding:builtin`) and LF-3 classifies them, but there is no bundle and no check.

## 2. Goals

- One action that configures a **fully offline** stack: LLM, TTS, image, and embeddings.
- An **offline verification** that inspects the resolved providers and reports any that are not
  offline.
- The bundle is a preset, not a mode: it writes ordinary config, and the user can change any part.
- Available from the settings UI and the CLI.

## 3. Non-goals

- Bundling a model. The bundle uses the providers that need no model; a neural TTS or the ONNX
  embedding still downloads on demand (LF-1).
- Enforcing offline mode (no network calls anywhere). The verification reports; it does not block.
- The tier labels themselves (LF-3).

## 4. Design

### 4.1 The bundle

A preset that sets the resolved config to:

| Role/family | Provider |
| --- | --- |
| `gm` (LLM) | `narrative-oracle` |
| `narrator`/`extractor`/`completion` | `disabled` or the oracle |
| TTS | `native-os` (no model) or `sherpa-onnx` (a local model) |
| Image | `procedural-art` |
| Embeddings | `builtin` (or `onnx` once LF-1 lands) |

The TTS choice is a sub-option: `native-os` is truly model-free; `sherpa-onnx` needs the Kokoro
download but is offline after it. The preset offers both and says which needs a download.

### 4.2 Applying

The preset is a `config` transform, reusing the existing preset mechanism (descriptors carry presets,
`pkg/provider/descriptor.go:55-61`) plus a bundle that sets several roles at once. Applying it:

- sets the roles and media providers to the offline choices;
- leaves everything else (paths, preferences) untouched;
- reports what it changed.

Because MP-1 keeps the singleton fields as the default, the bundle writes the singleton fields; a
user's named providers are untouched.

### 4.3 Offline verification

```go
// OfflineReport lists configured providers that are not offline.
type OfflineReport struct {
	Offline bool
	Issues  []OfflineIssue // {Role, ProviderKey, Tier, Reason}
}

// VerifyOffline inspects every resolved provider against its descriptor.
func VerifyOffline(cfg *config.Config) OfflineReport
```

It resolves each role and media family to a provider key, looks up its descriptor, and checks
`FeatureOffline` and the tier (LF-3). A cloud or local-server provider is an issue, with its reason.
The report is shown after applying the bundle and available on demand (a "Check offline" action).

The check is a report, not an enforcement: a user may deliberately use a local server, which is not
offline but is local, and the report says so.

### 4.4 Surfaces

- **Settings UI**: an "Offline preset" action in the Providers section (MP-5) with a confirmation of
  what will change, and a "Check offline" action that shows the report.
- **CLI**: `localrpg config offline-preset` (apply) and `localrpg config check-offline` (report).

### 4.5 What "offline" means

The verification reports a provider as offline when its descriptor carries `FeatureOffline`. That is
the descriptor's honest claim (LF-3's drift guard keeps it consistent with the code). The report does
not attempt to intercept network calls; it classifies the configuration.

## 5. Behaviour

| Action | Result |
| --- | --- |
| apply the bundle | the four families are set to the offline providers |
| apply with `sherpa-onnx` | a note that a model download is required |
| check offline, all offline | a clean report |
| check offline with a cloud provider | an issue naming the role and provider |
| check offline with a local server | an issue, marked "local, not offline" |
| apply twice | idempotent |

## 6. Testing

- `pkg/config`/`pkg/gui`: applying the bundle sets the expected providers and leaves the rest; applying
  twice is idempotent.
- `pkg/provider`: `VerifyOffline` reports a cloud provider, a local-server provider, and a fully
  offline stack correctly.
- `cmd/localrpg`: the two verbs.
- A regression guard: applying the bundle does not alter paths or preferences.

## 7. Rollout

Additive: a preset, a check, and two surfaces. No default changes.

## 8. Risks

- **A false sense of security.** "Offline" here means "the configured providers claim to be offline",
  not "no packet leaves the machine". The report's wording must be precise, and LF-5's page explains
  the tiers.
- **Model downloads.** `sherpa-onnx` and `onnx` need a one-time download; the bundle says so, so the
  user is not surprised.
- **Overwriting a deliberate setup.** Applying the bundle replaces the four families' defaults; the
  confirmation lists what changes, and named providers (MP-1) are untouched.

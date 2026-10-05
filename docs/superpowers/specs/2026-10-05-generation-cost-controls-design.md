# Generation Cost Controls Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#83 WG-6](https://github.com/darkliquid/LocalRPG/issues/83)
**Epic:** [#25 AI world generation](https://github.com/darkliquid/LocalRPG/issues/25)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §8 (WG-6)
**Depends on:** [#78 WG-1](https://github.com/darkliquid/LocalRPG/issues/78)
**Scope:** `pkg/worldgen`, `pkg/gui`, `frontend`

---

## 1. Problem

World generation is several large model calls. WG-1 makes four calls, WG-2 one, WG-4 one per chunk
batch. With a metered provider, a user who presses "Generate" on a big brief can spend real money
with no warning: the endpoints stream progress but never say how much work is coming, and nothing
stops a runaway.

The usage ledger (`pkg/pricing`) records what was spent **after** the fact. Generation needs a
**before** control.

## 2. Goals

- Estimate the number of model calls (and, when priced, the cost) before generation starts.
- Require confirmation when the estimate exceeds a threshold.
- Enforce a **cap** on calls and abort past it.
- Fall back to the **deterministic oracle** (LF-2) when no provider is configured, so generation
  still works offline.
- Report the actual usage afterwards.

## 3. Non-goals

- A general budget system across all features; IMG-4 does images. This is generation.
- Caching generated worlds (a follow-up).
- Changing the pipeline's steps.

## 4. Design

### 4.1 The estimate

`pkg/worldgen` gains:

```go
// Estimate is the cost of a planned generation, before it runs.
type Estimate struct {
	Calls     int     // model calls
	Chunks    int     // for ingestion
	CostMicros int64  // 0 when the provider is unpriced
}

// EstimatePlan reports the calls a generation will make.
func EstimatePlan(kind string, brief Brief, chunks int, prices pricing.Table) Estimate
```

The estimate is a function of the pipeline:

- WG-1: one call per step (four), plus one per batch if the entity counts exceed a batch size.
- WG-2: one call per batch of the requested count.
- WG-4: one call per chunk batch plus the WG-1 steps.

`CostMicros` uses the pricing ledger when the `generator` role's provider is priced; otherwise zero,
and the UI shows "unpriced".

### 4.2 The confirmation

The generation endpoints accept a `dry_run` flag. A dry run returns the `Estimate` without calling the
model. The studio always dry-runs first and shows:

- the call count and, when priced, the estimated cost;
- a **Generate** button (and a stronger confirmation when the estimate exceeds a threshold).

This makes the cost visible before the spend, which is the whole point.

### 4.3 The cap

A generation carries a **call budget**: `config.Generation.MaxCalls` (default, for example, 20).
The pipeline increments a counter per call and returns an error naming the cap when it is exceeded.
The cap is per generation, not per process, so one runaway cannot starve the rest.

For ingestion, a **chunk budget** bounds the number of batches.

### 4.4 The offline fallback

When no `generator`/`gm` provider is configured, generation would error. Instead, when the resolved
provider is the deterministic oracle (LF-2) or the built-in narrative-oracle, the pipeline runs
against it, producing a **template** world (a simple outline, a handful of entities from the brief)
that is honest about being a fallback. The estimate for the oracle is zero calls and zero cost.

This makes the feature usable offline and makes LF-2's oracle a first-class generator, not only a
play-time narrator.

### 4.5 Reporting

After a generation, the actual call count and cost are recorded in the usage ledger under the
`generator` role (the ledger already keys by role and provider), so the Usage view shows what
generation cost. The draft's metadata carries the estimate and the actual.

## 5. Behaviour

| Situation | Result |
| --- | --- |
| dry run | the estimate, no calls |
| estimate over the threshold | a stronger confirmation |
| estimate under it | a normal Generate |
| calls exceed the cap | an error naming the cap; the draft is discarded |
| no provider configured | the oracle produces a template world |
| a priced provider | the estimate includes the cost |
| an unpriced provider | the estimate says "unpriced" |

## 6. Testing

- `pkg/worldgen`: `EstimatePlan` returns the expected call count for each kind and count; a cap is
  enforced; the oracle path produces a template world with zero calls.
- `pkg/gui`: a dry run makes no model call; a capped generation errors; the actual usage is recorded.
- `frontend`: the estimate is shown before Generate; a large estimate requires confirmation.
- A regression guard: a small generation under the cap and threshold behaves as before.

## 7. Rollout

Additive: a dry-run flag, a config cap, and a fallback. The default cap is generous enough not to
affect a normal generation.

## 8. Risks

- **A cap that blocks a legitimate generation.** The default is generous and configurable; the error
  names the cap so the user can raise it.
- **Estimate accuracy.** The estimate is a planning aid, not a bill. The actual usage is recorded and
  shown, so the two can be compared.
- **Oracle quality.** The template world is a fallback, not a substitute; it is labelled as such and
  is the same honesty LF-3's tiers establish.

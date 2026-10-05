# Provider Chains Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#30 MP-4](https://github.com/darkliquid/Projects/LocalRPG/issues/30)
**Epic:** [#16 Multiple providers of the same type](https://github.com/darkliquid/Projects/LocalRPG/issues/16)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §1 (MP-4)
**Depends on:** [#27 MP-1](https://github.com/darkliquid/Projects/LocalRPG/issues/27), [#28 MP-2](https://github.com/darkliquid/Projects/LocalRPG/issues/28), [#34 LF-3](https://github.com/darkliquid/Projects/LocalRPG/issues/34)
**Scope:** `pkg/harness`, `pkg/media`, `pkg/config`, `pkg/gui`

---

## 1. Problem

A role has exactly one **fallback**: `Router.fallbacks` maps a role to a single fallback provider id
(`pkg/harness/router.go:24-29,122-129`), and `GenerateForRole` tries the primary then the fallback
(`:178-222`). There is no way to say:

- "try these three in order";
- "use the cheapest that is configured";
- "prefer a local provider, fall back to cloud only if there is no local one";
- "use the provider tagged `fast` for this role".

With MP-1 (several named providers) and MP-3 (purposes), a user can configure several providers but
can only order two of them, once. The richness of MP-1 is unusable without a selection policy.

## 2. Goals

- A role or purpose declares an **ordered list** of provider instances, not one fallback.
- A **selection rule** chooses within the list: `first`, `cheapest`, `local-first`, or `by-tag`.
- The list is also the **fallback chain**: on a hard failure, the next instance is tried.
- The default is `first`, which is today's behaviour (primary, then one fallback).
- The selection and the fallbacks are traced, so a choice is explainable.

## 3. Non-goals

- The named providers (MP-1), the purposes (MP-3), or the instance ids (MP-2).
- Cost estimation (WG-6) or budgets; this uses the ledger's prices.
- A general constraint solver; the rules are the four above.

## 4. Design

### 4.1 The chain

A role's configuration gains an ordered chain:

```yaml
agents:
  roles:
    gm:
      chain: [good, cheap, local]     # provider instance names (MP-1/MP-2)
      select: cheapest                # first | cheapest | local-first | by-tag
      tag: fast                       # for select: by-tag
```

and a purpose (MP-3) may carry one too:

```yaml
media:
  purposes:
    narrator: { chain: [premium, default], select: first }
```

For the LLM side, the chain names **roles or providers**; for media, it names **provider instances**.
Both reduce to a list of resolvable provider keys plus a rule.

### 4.2 The rules

- **first**: the first configured instance that builds. This is today's primary/fallback.
- **cheapest**: order by the ledger's price for the instance's provider key and model, cheapest
  first; an unpriced instance sorts last (a priced one is a known quantity).
- **local-first**: order by the LF-3 tier (offline-basic and offline-neural first, then local-server,
  then cloud); within a tier, the declared order.
- **by-tag**: prefer instances whose provider descriptor carries the named tag or feature (for example
  `FeatureTools`, `FeatureVision`); within the preferred set, the declared order.

The rule **orders** the chain; the chain is then tried in order on a hard failure, so a rule and a
fallback are the same mechanism.

### 4.3 The router

`Router` gains:

```go
// SetChain assigns an ordered chain and a selection rule to a role.
func (r *Router) SetChain(role string, ids []string, rule string, tag string)

// GenerateForRole picks the first chain member that succeeds, in the rule's
// order, recording the attempts.
func (r *Router) GenerateForRole(ctx context.Context, role string, req Request) (Response, error)
```

`GenerateForRole` builds the ordered list (the chain, sorted by the rule), then tries each in order,
recording an `Attempt` per failure and returning the first success. A role with no chain uses the
existing primary/fallback, so the default is unchanged.

For media, a small `media.Chain` mirrors the router: `Select(chain, rule, tag) []TTSConfig` (or
`ImageConfig`), used by the registries (MP-1) and the purposes (MP-3).

### 4.4 Cost and tags

- **cheapest** reads the pricing ledger (`pricing.Resolve`) for each instance's key and model. An
  instance the ledger cannot price is unknown and sorts last.
- **by-tag** reads the descriptor's `Features` (LF-3's descriptors) and the tier; the tag is a feature
  name or a tier. A descriptor without the tag is not preferred.

### 4.5 Tracing

Each attempt is recorded (`harness.Attempt`), and the selected instance and the rule are traced
(`router.select`), so a trace says "cheapest chose `cheap`; `cheap` failed; tried `good`; succeeded".

### 4.6 Validation

A chain member must name a configured instance (MP-1/MP-2); an unknown name is a validation error. A
rule must be one of the four; an unknown rule is a validation error. `by-tag` requires a tag.

## 5. Behaviour

| Chain, rule | Result |
| --- | --- |
| `[a, b]`, `first` | a, then b on failure |
| `[a, b]`, `cheapest` (b priced lower) | b, then a on failure |
| `[cloud, local]`, `local-first` | local, then cloud |
| `[x, y]`, `by-tag: tools` (y has tools) | y, then x |
| `[]` | the existing primary/fallback |
| an unknown chain member | validation error |

## 6. Testing

- `pkg/harness`: `SetChain` orders by each rule; `GenerateForRole` tries in order and falls through on
  failure; an empty chain is the existing behaviour; an attempt is recorded per failure.
- `pkg/media`: `Select` orders a media chain by each rule.
- `pkg/config`: a chain and a rule round-trip; an unknown member or rule is a validation error.
- A regression guard: no chain behaves exactly as today.
- A property test: for any chain and rule, the order is a permutation of the chain.

## 7. Rollout

Additive: a chain and a rule are optional. A role with no chain keeps the primary/fallback, so every
existing configuration is unchanged.

## 8. Risks

- **Cheapest that is bad.** The cheapest provider may be poor. The rule is explicit, the trace shows
  the choice, and a user who wants quality uses `first` or orders the chain.
- **Tag vocabulary.** `by-tag` depends on descriptors' features; a tag that no provider has selects
  nothing and falls back to the declared order. The trace says so.
- **Cost of ordering.** `cheapest` reads the ledger, which is cheap (in-memory). No provider call is
  made to choose.

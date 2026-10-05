# Narrative Oracle Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#33 LF-2](https://github.com/darkliquid/LocalRPG/issues/33)
**Epic:** [#17 Zero-GPU and local-first offerings](https://github.com/darkliquid/LocalRPG/issues/17)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §2 (LF-2)
**Depends on:** [#37 SYS-1](https://github.com/darkliquid/LocalRPG/issues/37)
**Scope:** `pkg/provider/oracle`

---

## 1. Problem

`llm:narrative-oracle` is the only provider that needs no model, no server, and no network, so it is
the offline fallback and the zero-GPU path. It is also nearly unusable:

- `craftProse` (`pkg/provider/oracle/provider.go:65-132`) parses a mechanics tier, a player action,
  and `[[wikilinks]]`, seeds an RNG from the prompt's length, and picks from a handful of hardcoded
  openers.
- It ignores the player's stats, the location, the stakes, and the entities' names beyond their
  links.
- Its output is a template: two sentences that could describe any action in any place.

So the honest offline story is "the app runs with no model", but not "you can play offline". LF-3
labels the tier; LF-2 is what makes the tier worth having.

## 2. Goals

- The oracle reads the turn's state: the outcome tier, the player's declared stats, the stakes, the
  location, and the entities present.
- It assembles prose from a **richer grammar**: an opener by tier, a consequence by tier, an
  entity-aware line, and a location-aware line.
- It stays deterministic (same prompt, same output) and pure Go.
- It is honest: the output is template prose, and the tier label (LF-3) says so.

## 3. Non-goals

- Quality comparable to a model. The bar is "coherent and stateful", not "good".
- A new provider interface; the oracle remains a `ModelProvider`.
- The fallback policy (WG-6) and the tier label (LF-3).

## 4. Design

### 4.1 The inputs, from the prompt

The oracle is an LLM provider, so its only input is the assembled prompt. The engine's prompt already
carries what it needs, because SYS-1 and the context assembler put it there:

- the mechanics result: `[MECHANICS RESULT: …]` or `[ROLL RESULT: …]` with a tier/outcome;
- the player's stats: the `Player stats: …` line (`pkg/harness/mechanics_instructions.go:45-55`);
- the stakes: the `[PROPOSED CHECK: …]` or the roll's stakes;
- the entities and location: `[[wikilinks]]` and the scene scope's names.

The oracle parses these with small, tested regexes, as it already does for the tier and the action.
The parsing is a **contract** with the context assembler: a section rename breaks the oracle, so the
two share a test fixture.

### 4.2 The grammar

`craftProse` becomes a composer over four parts:

1. **Opener** by tier (strong/weak/miss, or success/fail), several variants each.
2. **Consequence** by tier, several variants, referencing the stakes when present.
3. **Cast** — when the scene names entities, a line that includes one by name (deterministically
   chosen), so the prose is about *these* people.
4. **Place** — when a location is known, a line that names it.

The parts are chosen with the seeded RNG and joined, so the output varies with the turn while staying
deterministic for a given prompt. A stats-aware touch: when the player's governing stat is high, the
strong openers lean confident; when low, the miss openers lean strained. This is a small mapping from
the stat value to a variant set, which is what makes it an *expert system* rather than a random
template.

### 4.3 Determinism

The seed is derived from the prompt (as today), so the same turn always yields the same prose. No
clock, no global RNG. A test asserts byte-identity for a repeated call.

### 4.4 Honesty

The oracle's descriptor keeps its tier (`offline-basic`, LF-3) and its label ("needs no model or
network"). The prose is template text; the tier badge and the caveat say so. The oracle must not
pretend to be a model.

### 4.5 The script/segments

The oracle returns prose. The turn stream expects either framed segments or prose the parser
segments. The oracle returns plain prose; the existing prose path (and the extractor) handle it, as
today. A follow-up could emit `> Name: "…"` speech for named entities, but that is a separate step;
this spec keeps prose and lets the parser segment it.

## 5. Behaviour

| Prompt | Output |
| --- | --- |
| a strong check with stats and a location | a confident opener, a consequence, a cast line, a place line |
| a miss with stakes | a strained opener, a stake-aware consequence |
| no stats | a generic opener |
| no entities | no cast line |
| the same prompt twice | identical prose |
| a prompt with no mechanics result | a neutral opener |

## 6. Testing

- `pkg/provider/oracle`: `craftProse` includes the location when present, an entity name when present,
  and a stake-aware consequence; the stat value selects a variant set; the output is deterministic;
  a prompt with no result is neutral.
- `pkg/provider/oracle`: a fixture prompt from the real assembler (shared with the context tests)
  parses into the expected parts.
- A regression guard: the provider still implements `ModelProvider` and streams the prose.

## 7. Rollout

A rewrite of the oracle's prose composer. Its descriptor, id, and streaming are unchanged. The output
changes (it is richer), which is the point.

## 8. Risks

- **Prompt coupling.** Parsing the prompt is brittle. The shared fixture and the small regexes keep it
  honest; a section rename is a test failure, not a silent degradation.
- **Expectation.** "Expert system" can oversell. The tier label and caveat are the guard; the output
  is still templates.
- **Scope.** It is tempting to keep adding grammar. Time-box it: four parts and a stat mapping is
  enough to be coherent; more is a model's job.

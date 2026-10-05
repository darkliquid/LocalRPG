# World Generation Pipeline Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#78 WG-1](https://github.com/darkliquid/LocalRPG/issues/78)
**Epic:** [#25 AI world generation](https://github.com/darkliquid/LocalRPG/issues/25)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §8 (WG-1)
**Depends on:** [#72 PKG-1](https://github.com/darkliquid/LocalRPG/issues/72)
**Scope:** new `pkg/worldgen`, `pkg/gui`, `pkg/harness`, `frontend`

---

## 1. Problem

World generation is field-scoped. `/api/generate-text` fills one field at a time
(`pkg/gui/text_generate.go:18-122`): a name, a description, a genre, an art style, a lore prompt. The
user assembles the world by pressing Generate on each field and then writes every entity by hand in
the studio. There is no "make me a world" and no "fill this world with people".

So the single most valuable AI feature the app could offer — turn a sentence into a playable world —
does not exist, and the pieces (a provider, a prompt builder, entity writing) all do.

## 2. Goals

- Generate a **draft world** from a short brief: identity, lore, factions, locations, and characters,
  cross-linked.
- Do it **step by step**, with progress, so a long generation is visible and interruptible.
- Produce a draft the user reviews before it becomes a world (WG-5); never write a live world.
- Reuse the existing provider routing, prompt assembly, and entity format.
- Bound the cost (WG-6).

## 3. Non-goals

- Enhancing an existing world (WG-3) and batch entity generation (WG-2).
- Ingestion from folders or URLs (WG-4).
- System generation (SG-*).
- Committing the draft; that is WG-5.

## 4. Design

### 4.1 The pipeline

A new `pkg/worldgen` package orchestrates steps, each a structured model call:

1. **Outline** — from the brief, produce `{name, genre, premise, themes, tone, art_style}`.
2. **Places and factions** — produce a list of `{locations[], factions[]}`, each with a name,
   one-line description, and tags.
3. **Characters** — produce a list of `{name, role, faction, home_location, description, tags,
   state}`, seeded with the outline and the places/factions.
4. **Cross-link** — a final pass that writes each entity's body with `[[wikilinks]]` to the others
   and assembles `world.yaml` and `prompts/lore.md`.

Each step is a separate call so a failure at step 3 does not lose steps 1-2, and each is resumable.

```go
// Brief is the user's starting point.
type Brief struct {
	Name     string   // optional; generated when empty
	Genre    string   // optional
	Premise  string   // the user's sentence
	Themes   []string
	Counts   Counts   // how many locations, factions, characters
}

// Draft is the generated world, before it is committed.
type Draft struct {
	World    core.WorldManifest
	Lore     string
	Entities []DraftEntity
}

// Generate runs the pipeline, reporting progress per step.
func Generate(ctx context.Context, gen Generator, brief Brief, onStep func(Step)) (Draft, error)
```

`Generator` is the model seam: a function that takes a prompt plus a JSON schema and returns
structured JSON, backed by the existing provider router (`harness.RouterFromConfig`) and, where the
provider supports it, native structured outputs; otherwise a parse-and-repair path (RB-1's
`jsonrepair`).

### 4.2 Structured outputs

Each step declares a JSON schema, so the model returns data rather than prose to be parsed. Where a
provider cannot do structured outputs, the prompt asks for JSON and `jsonrepair` recovers it. This
mirrors the superseded structured-GM idea, applied to generation rather than play.

### 4.3 Reuse

- **Provider**: `harness.RouterFromConfig` with a `generator` role (falling back to `gm`), reusing
  the existing role/fallback machinery.
- **Entity format**: `entity.ParseMarkdownEntity`/the frontmatter writer, so a draft entity is a real
  entity note.
- **Prompt builder**: the existing `buildTextGeneratorPrompt` shape, extended per step.
- **Retries**: `generation_attempts.go`'s role-fallback chain.

### 4.4 Progress and interruption

Generation streams NDJSON (like a turn): a `step` event per step (`outline`, `places`, `characters`,
`link`), each with a status, and a final `draft` event carrying the draft (or a reference to it). A
client can cancel, which aborts the in-flight call and discards the partial draft.

### 4.5 Where the draft lives

A draft is **not** written into `worlds/`. It is held in memory and, for a long generation or a
reload, persisted to `worlds/.drafts/<id>.yaml` (a dot-directory the syncer skips). WG-5 reviews it;
committing moves it into `worlds/<id>/`.

### 4.6 Surfaces

- **API**: `POST /api/world/generate` (brief → NDJSON stream of steps and the draft).
- **Studio**: a "Generate a world" flow in `WorldsStudio.tsx` (or the launcher's new-world path) that
  collects the brief, shows step progress, and hands the draft to WG-5's review.

## 5. Behaviour

| Input | Result |
| --- | --- |
| a premise, default counts | a draft with identity, lore, and the requested entities |
| a provider without structured outputs | the same draft via parse-and-repair |
| a step failure | the earlier steps are kept; the pipeline reports the failed step |
| cancellation | the in-flight call aborts; no draft is committed |
| no provider configured | an error naming the missing provider |

## 6. Testing

- `pkg/worldgen`: a stub `Generator` drives the pipeline; the draft has the requested counts; a
  failing step is reported and earlier steps survive; cancellation aborts.
- `pkg/worldgen`: a draft's entities parse with `entity.ParseMarkdownEntity` and their wikilinks
  resolve to other draft entities.
- `pkg/gui`: the endpoint streams a step per call and a final draft; a cancelled stream leaves no
  draft.
- A regression guard: a `Generator` returning malformed JSON is repaired by `jsonrepair`.

## 7. Rollout

Additive: a new package, an endpoint, and a studio flow. No existing behaviour changes. The
`generator` role defaults to `gm`.

## 8. Risks

- **Quality.** A generated world is only as good as the model. The review step (WG-5) is the control;
  generation without review produces clutter, which is why the pipeline never commits.
- **Cost.** Four-plus calls per world, each large. WG-6's controls (counts, a cap, a preview) are the
  mitigation; the counts in `Brief` are the primary lever.
- **Consistency.** Cross-links can reference an entity that was not generated. The link step
  validates every wikilink against the generated set and drops or stubs an unknown one.

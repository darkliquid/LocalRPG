# Genre-Aware Fallbacks Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#59 PH-4](https://github.com/darkliquid/Projects/LocalRPG/issues/59)
**Epic:** [#21 Placeholder and fallback variety](https://github.com/darkliquid/Projects/LocalRPG/issues/21)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §7 (PH-4)
**Scope:** `frontend`, `pkg/gui`, `tools/sitegen`

---

## 1. Problem

The app has genre knowledge and does not use it in its chrome. The launcher's procedural assets have
six palettes and a genre→icon map (`frontend/src/components/launcher/ProceduralAsset.tsx:23-118`), but
they are local to that component. The app root has a single CSS radial gradient
(`frontend/src/App.tsx:740`). Empty states and loading messages are generic strings.

So a cyberpunk campaign and a grim fantasy campaign look identical when nothing is loaded: the same
gradient, the same "Loading…". The genre the user chose is invisible until content exists.

## 2. Goals

- One **palette source**: genre → a small colour set, used by banners, the app background, empty
  states, and loading states.
- **Genre-aware copy**: empty-state text and a few loading messages that fit the genre.
- The app, the site, and the export read the **same source**, so they agree.
- Nothing changes once content exists; this is the empty and loading experience.

## 3. Non-goals

- The procedural art itself (PH-1) and portraits (PH-2); this is chrome and copy.
- Generated images.
- A theme system; one palette per genre is enough.

## 4. Design

### 4.1 The palette source

A shared module, `frontend/src/lib/genre.ts`:

```ts
// GenrePalette is the colour set for one genre.
export interface GenrePalette {
  id: string;
  label: string;
  from: string; // gradient start
  to: string;   // gradient end
  accent: string;
  icon: string; // a lucide icon name
}

// genrePalette resolves a genre (or a world's tags) to a palette.
export function genrePalette(genre?: string): GenrePalette;
```

It holds palettes for the genres the app ships (fantasy, cyberpunk, horror, sci-fi, western,
modern, historical, and a neutral default) and resolves an unknown or absent genre to the default.
The launcher's `ProceduralAsset` moves its palette table here and reads from it, so there is one
source.

### 4.2 Where it shows

- **App background**: the root gradient uses the current campaign's palette (from the world's genre)
  instead of a fixed one.
- **Banners**: a campaign or world with no banner gets a genre gradient with its icon, replacing the
  current fixed set.
- **Empty states**: the launcher, a campaign with no turns, the codex with no entities, and a search
  with no results each get a genre-flavoured line and the genre's accent, from a small copy table.
- **Loading states**: a few loading messages vary by genre ("Consulting the oracle…", "Jacking in…")
  rather than a single "Loading…".

### 4.3 The copy table

A small table of genre → {empty, loading} strings, with a neutral fallback. The copy is short and
tasteful; it is a fallback, not a personality. The table lives beside the palette so the two are one
concern.

### 4.4 Shared with the site and the export

- **Site**: the showcase site's generator (`tools/sitegen`) already copies the app's palette values;
  it reads the same genre palettes for its own background, so the site and the app match.
- **Export**: the export's gradient (the theatre's no-art background, `pkg/scene/theater.go`) uses the
  genre palette for the campaign's world, so an export's empty beats match the app.

Sharing across languages is by **value**: the palette table is small and duplicated in Go where the
export needs it, with a test asserting the two agree (the pattern the provider presets already use).

### 4.5 When content exists

Once a campaign has a banner, a scene image, or a portrait, the fallback is not shown. The genre
palette still tints the app background subtly, which is the one persistent effect; it is a tint, not a
theme.

## 5. Behaviour

| Situation | Result |
| --- | --- |
| a cyberpunk campaign, no banner | a cyan/magenta gradient with the cyberpunk icon |
| the same campaign, a banner set | the banner |
| a codex with no entities | a genre-flavoured empty line |
| loading a turn | a genre-flavoured message |
| an unknown genre | the neutral palette |
| the site and the app | the same palette |

## 6. Testing

- `frontend`: `genrePalette` resolves each known genre and defaults; the launcher, the app background,
  the empty states, and the loading states use it.
- `pkg/scene`: the export's no-art gradient uses the same palette values for a genre as the frontend.
- A parity test: the frontend and Go palette tables agree for every genre.
- A regression guard: a campaign with a banner renders the banner, not the fallback.

## 7. Rollout

Additive: a shared module and its consumers. The neutral default preserves the current look for a
genre-less world.

## 8. Risks

- **Palette drift between languages.** The frontend and Go tables must agree; the parity test is the
  guard, and the table is small.
- **Copy taste.** Genre loading messages can grate. Keep them few, short, and easily changed; the
  neutral fallback is always available.
- **Accessibility.** A gradient behind text must keep contrast; the palettes are chosen dark enough for
  the app's light text, and the test can assert a minimum contrast for each.

## 9. Open question

Whether the app background should be tinted persistently by genre, or only when empty. The spec
chooses a subtle persistent tint; if it proves distracting, it can be limited to the empty state
without changing the palette source.

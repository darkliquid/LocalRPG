# The Modal Primitive Design

**Date:** 2026-10-15
**Status:** Proposed
**Issue:** gh-188 (the bead LocalRPG-14a.8)
**Epic:** [#152](https://github.com/darkliquid/LocalRPG/issues/152)
**Proposal:** `docs/architecture/quality-review-2026-10-12.md` (FE-2)
**Scope:** `frontend/src`. No Go change.

## 1. Problem

**33 files carry their own overlay shell** — measured as `fixed inset-0` over `frontend/src` — and
the review's count of 31 copy-pasted overlays is close enough to that to be the same finding. Of those
surfaces, **3 declare `role="dialog"`** (`AboutModal`, `DocsModal`, `StoryTheater`), and none of the 33
traps focus, moves it in when it opens, restores it when it closes, or treats Escape the same way as
its neighbour.

What that costs is not the duplication. It is that a keyboard user can tab *behind* an open modal and
lose it, has no way back to the control that opened it, and cannot be sure Escape will work. Three
surfaces tell a screen reader they are dialogs; the other thirty do not, so the announcement depends on
which one opened.

## 2. Goals

- **One shell**, the primitive that already landed (`components/Modal.tsx`): `role="dialog"`,
  `aria-modal="true"`, `aria-labelledby` on its title, focus moved to the first control on open,
  focus returned to the opener on close, Tab cycling inside, and Escape closing unless the modal is one
  that must be answered.
- **Every blocking overlay uses it**, and the ones that are not blocking are named as such rather than
  left in the count.
- **The three existing dialogs** gain what the primitive has: `aria-modal`, a labelled title, and the
  focus behaviour.
- A keyboard user can always leave a modal, and always returns where they were.

## 3. Non-goals

- **A visual redesign.** The primitive's own chrome is the shell's; each converted surface keeps its
  content and its layout classes. Some headers and paddings will settle into the shared form, which is
  the point, but no colour, wording or control changes.
- **Drawers that are not modals.** A panel that slides in while the page behind stays usable is not a
  dialog: converting it would trap focus over a page the user is still working in. Each is classified
  before it is converted, and a genuinely non-blocking one stays out with a note saying why.
- **Focus order inside a studio**, which is what FE-6 is for.
- **The Go side.**

## 4. Design

### The primitive, as landed

Documented in `components/Modal.tsx`, with six tests. The two props that carry a decision:

- **`title`**, required: it labels the dialog, so a conversion cannot skip it.
- **`dismissable`**, default true: false for a modal that must be answered, where Escape and a backdrop
  click are refused and no close button is rendered. Deleting a campaign is the case that names it.

### The conversion, per file

| The surface | What it becomes |
|---|---|
| A blocking dialog with a heading | `<Modal open onClose title>` around the existing content, `className` carrying its width |
| A confirmation that must be answered | the same, with `dismissable={false}` |
| A drawer or panel the page stays usable behind | stays, with a comment saying it is not a modal |
| One of the three that already declares a dialog | keeps its markup if converting it would change its layout, but gains `aria-modal`, a labelled title, and the focus behaviour |

**Every converted file drops its own Escape and backdrop handler**, because the shell owns them; a file
that keeps one has two handlers and closes twice.

### The batches

By area, one pull request each, so each is visually checkable: the launcher's modals, the entity and
codex dialogs, the studios' dialogs and drawers, the settings and provider modals, then the docs,
theatre and export surfaces.

## 5. Behaviour

- A modal announces itself and its title; Escape closes it; Tab stays inside it; focus returns to the
  opener.
- Focus can no longer reach the page behind an open modal.
- A conversion's own handlers are removed rather than left beside the shell's.
- Visual change is limited to the shared header and padding, and a batch that shows more than that is
  a bug in the conversion.

## 6. Testing

- **The primitive**: its six tests, already landed.
- **Per converted surface**: one test that it renders as a dialog with its title and closes on Escape —
  the assertions that were missing from all 33.
- **A keyboard test** for a modal whose focus behaviour decides whether the user can proceed (a
  confirmation, an import).
- `npx tsc --noEmit`, `npx vitest run --coverage`, `mise run lint:frontend`, then `mise run test`.

## 7. Rollout

The primitive first (landed), this spec and plan now, then one batch per area with the bead open across
them and its comment recording what has landed.

## 8. Risks

- **A conversion can change a layout subtly.** Batching by area, with a visual check per batch, is the
  control; a batch that moves more than a header is wrong.
- **Two Escape handlers** is the failure mode of a half-conversion, and it closes a modal twice —
  which looks like nothing happened. Each converted file's own handler is removed in the same change.
- **A nested modal** (one opened from another) needs the inner one to take focus when it mounts; the
  primitive's effect does that on `open`, and its title id is per instance, so nesting survives — but
  it is worth a test the first time a surface nests.
- **Thirty-three files is a large sweep.** None of it is clever, which is why it is batched rather than
  done in one pull request nobody can review.

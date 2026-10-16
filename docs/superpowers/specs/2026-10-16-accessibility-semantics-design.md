# Accessibility Semantics Design

**Date:** 2026-10-16
**Status:** Proposed
**Issue:** gh-184 (the bead LocalRPG-14a.4)
**Epic:** [#152](https://github.com/darkliquid/LocalRPG/issues/152)
**Proposal:** `docs/architecture/quality-review-2026-10-12.md` (FE-6, part of F-8)
**Scope:** `frontend/src`. No Go change.
**Depends on:** FE-2 (the modal primitive, closed) and TC-5 (the axe assertion, closed)

## 1. Problem

Three classes of thing exist on screen and nowhere else.

**Progress that is drawn but not stated.** Nine bars are plain `<div style={{width}}>` elements with no
role and no values, against one (`ImportProgressPanel`) that declares `role="progressbar"` and its
numbers. Measured today over `frontend/src`:

| The bar | What is beside it |
|---|---|
| `theater/TheaterTransport.tsx:55` | nothing: the rail is the only statement of the position |
| `TTSBatchPanel.tsx:372` | a `title` on the track, which no assistive technology reads on a `div` |
| `CharacterSheetDrawer.tsx:57` | "Health" and "12 / 20" above it |
| `CharacterSheetDrawer.tsx:77` | "3 / 5" below it |
| `LivingWorldDrawer.tsx:103`, `:131` | "3/6" and "2/4" above each |
| `providers/TTSProviderEditor.tsx:340` | "Downloading 30%" beside it |
| `ModelDownloadModal.tsx:140` | "30%" and "1.2 / 320 MB" above it |
| `ExportModal.tsx:300` | "45%" above it, and a made-up 10% width while the phase reports no percentage |

**A turn that arrives in silence.** `App.tsx` streams a turn's narration into the chronicle, and nothing
tells assistive technology that a turn is being written or that one has landed. Four banners — the
provider funds error, the content warnings, the rate-limit notice and the turn error — are rendered with
no role, so each appears and disappears unannounced.

**Names that live only in a tooltip.** Of the 83 buttons in `frontend/src` whose only child is an icon,
59 carry an explicit name, **17 are named only by `title`**, and **7 have no name at all**: five dismiss
buttons and two closes (`App.tsx:1236`, `:1251`, `AddEntityModal.tsx:54`, `OfflinePreset.tsx:128`,
`:197`, `launcher/CampaignGallery.tsx:251`, `launcher/CampaignSettingsModal.tsx:238`). A `title` is the
last resort in the accessible-name computation and a tooltip needs a pointer, so an icon-only button
that relies on one is named to nobody on a keyboard or a touch screen.

## 2. Goals

- **Every drawn bar is either stated or hidden.** A bar that is the only statement of its value becomes
  a `role="progressbar"` with a name and its numbers. A bar that duplicates a value already written
  beside it is decoration, and is marked `aria-hidden` so it is not read twice.
- **A turn announces itself once, at each end.** A live region says a turn is being written, and says
  which turn landed, without narrating the prose into the reader's ear token by token.
- **The four banners have roles.** An error that needs the player to act is an alert; a notice that can
  wait is a status.
- **An icon-only control carries its own name**, whatever the pointer is doing.
- **The three rules are enforced, not remembered**: the progress rule by a component, the name rule by a
  source-scanning test, and the announcement rule by the axe assertions TC-5 landed.

## 3. Non-goals

- **Announcing the prose.** A screen reader reading a streaming turn token by token is unusable; the
  milestone is the announcement, and the chronicle stays readable where it is.
- **Progress as a live region.** A bar that updates every few hundred milliseconds, announced each time,
  is noise. Where a value genuinely needs announcing it is a sentence, not a bar.
- **Visual change.** The bars keep their colours, heights and widths; only their semantics change.
- **Colour contrast.** jsdom cannot measure it, so the axe assertion cannot gate it, and no rule here
  depends on it.
- **The Go side.**

## 4. Design

### A bar is stated or it is decoration

`frontend/src/components/ui/ProgressBar.tsx` is the one way to state a value:

- `value`, `max` (default 100) and **`label`**, which is required for the same reason `Modal`'s `title`
  is: a progressbar with no name is a number with no meaning.
- The value is clamped into `0..max`, so a caller cannot render a bar wider than its track or a negative
  `aria-valuenow`.
- It renders `role="progressbar"` with `aria-valuenow`, `aria-valuemin` and `aria-valuemax`, and the
  fill's width derived from the same clamped value, so the drawn width and the stated number cannot
  disagree.
- `className` and `barClassName` carry the track and fill styling, because the studios, the theatre and
  the settings each have their own palette and none of them should have to give it up.

Two bars use it, because two are the only statement of their value: `TheaterTransport`'s playback rail
("Playback position") and `TTSBatchPanel`'s job bar ("Clips stored"). **The other seven are marked
`aria-hidden="true"`**, with a comment naming the text that already carries the number.

`ExportModal` is the case that decides the rule: while the export reports no percentage the bar draws a
fixed 10% as a shape, and a progressbar claiming a value there would be a lie. It is decoration, and
hiding it is the honest answer.

### A turn announces its two ends

`frontend/src/components/ui/LiveStatus.tsx` renders a `role="status"` paragraph, visually hidden,
always mounted — a live region that is inserted at the same moment its text arrives is announced by few
screen readers, so it is present and empty and then given a sentence:

- the turn starts (`beginTurnStream` sets `turnInFlight`) → "Narrating the turn…";
- a turn lands (`handleTurnEvent`'s `turn` event appends to the chronicle) → "Turn 12 ready".

Both sentences are set where the event is known: `beginTurnStream` sets the first, and the `turn` branch
of `handleTurnEvent` sets the second from the turn number it just appended. An effect on the chronicle
would have needed a first-run guard, because its first value is the campaign's history loading rather
than news; setting the sentence at the two places that already know removes the question.

The chronicle's container also takes `aria-busy={turnInFlight}`, which is the standard way to say "this
region is being updated" without announcing the update.

### The banners

| The banner | Role | Why |
|---|---|---|
| the provider funds error | `alert` | the player has to add funds before anything else proceeds |
| the turn error | `alert` | the turn they submitted did not happen |
| the rate-limit notice | `status` | a wait, not a failure |
| the content warnings | `status` | information about the campaign, polite rather than interrupting |

The failure region TC-5's predecessor landed (`FailuresContext`) already announces what it reports; the
four banners are separate because each is dismissible and local to the chronicle.

### The names

The 7 unnamed buttons gain an `aria-label` that says what they do ("Dismiss", "Close", "Clear the
search"). The 17 named only by `title` gain an `aria-label` beside the title, which stays for the
tooltip.

**The rule is enforced by a test, not by memory.** `frontend/src/test/iconButtons.test.ts` parses every
`.tsx` under `src/` with the TypeScript compiler, finds each `<button>` whose subtree renders no text
and contains no expression (an expression might render text, so it is left alone), and fails if such a
button has no `aria-label` or `aria-labelledby`. That is the same shape as the revive gate: the check
exists, and reaching zero promotes it from a report to a gate.

## 5. Behaviour

- A screen-reader user hears that a turn is being written, and which turn arrived, once each.
- The bars that state a value expose it; the bars that repeat a value are not read twice.
- An error the player must act on interrupts; a notice does not.
- An icon-only button announces what it does, with no pointer involved.
- Nothing changes visually.

## 6. Testing

- **`ProgressBar`**: unit tests for the stated value, the clamp at both ends, a `max` other than 100,
  and the required label reaching `aria-labelledby`/`aria-label`.
- **`LiveStatus`**: a unit test that an empty region is present, and that it takes its sentence.
- **`iconButtons.test.ts`**: the source scan, which currently reports 7 unnamed buttons and must report
  none.
- **The banner roles and the turn announcement**: asserted where a test can render the surface; the
  launcher and system-studio tests already render App-adjacent trees, and the announcement is asserted
  through a small extraction if App itself is not testable.
- **The existing axe assertions** in the dialog tests continue to run, and a surface that loses a name
  fails there.
- `npx tsc --noEmit`, `npx vitest run --coverage`, `mise run lint:frontend`, `mise run test`,
  `mise run lint`.

## 7. Rollout

One pull request, three commits, in this order:

1. `ProgressBar` and the nine bars.
2. `LiveStatus`, the turn announcement, `aria-busy` and the four banner roles.
3. The 24 names, and the scanning test that keeps them.

## 8. Risks

- **A progressbar that repeats adjacent text is worse than a hidden bar**: two announcements of one
  fact, one of them interrupting. The rule above decides it per bar, and each `aria-hidden` names the
  text it defers to.
- **A live region that mounts with its text is not announced.** The region is rendered empty and
  always, which is the whole reason it is a component rather than a conditional paragraph.
- **A chatty announcement is the failure mode.** Two sentences per turn, and only for a turn the player
  asked for.
- **The scanning test can be wrong about a button.** It looks only at buttons whose subtree has no text
  and no expression, so a button rendering a label from a variable is left alone rather than
  misreported; a false negative is acceptable, a false positive that forces a meaningless label is not.

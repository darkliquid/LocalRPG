# Splitting the Two Studios Design

**Date:** 2026-10-16
**Status:** Proposed
**Issue:** gh-182 (the bead LocalRPG-14a.6)
**Epic:** [#152](https://github.com/darkliquid/LocalRPG/issues/152)
**Proposal:** `docs/architecture/quality-review-2026-10-12.md` (F-3)
**Scope:** `frontend/src/components`. No Go change, no behaviour change.

## 1. Problem

`SystemsStudio.tsx` is **1,584 lines** and `WorldsStudio.tsx` **1,625**. Each is one component with
**42 and 45 `useState` hooks** and a `return` that opens at line 572 and 703 and runs to the end of the
file: roughly a thousand lines of JSX in a single expression, containing the list column, the header,
five and four tab bodies, and two or three dialogs.

Measured regions, which are the same in both files because one was written from the other:

| Region | SystemsStudio | WorldsStudio |
|---|---|---|
| Master list column | 574-700 | 705-815 |
| Header and sub-tabs | 703-876 | 816-949 |
| Toast | 877-961 | 950-967 |
| Tab 1: manifest / lore and artwork form | 962-1292 | 968-1323 |
| Tab 2: rules prompt / lore prompt editor | 1293-1326 | 1324-1358 |
| Tab 2b: mechanics editor | 1327-1339 | (none) |
| Tab 3: script editor / starter entities | 1340-1361 | 1359-1478 |
| Tab 5: tests | 1362-1440 | (none) |
| Dialogs | 1441-1584 | 1479-1625 |

The cost is not the line count. It is that a reader cannot see a tab's shape without scrolling past
four others, that a change to one tab is a diff inside a thousand-line expression, and that no part of
this UI can be rendered or tested without mounting the whole studio and its twenty-odd API calls.

## 2. Goals

- **A container per studio** that owns the state, the API calls and the handlers, and renders the
  layout; and **a view per region** that renders one part of it.
- **Every view takes focused props**, not the container's whole state: a view that reads one boolean
  takes that boolean. A single `model` prop would move the god object one level down and let any view
  read anything.
- **The moves are verbatim.** A refactor that changes logic is a bug, and the diff should be readable
  as a move.
- **Every view is renderable on its own**, with a small fixture, and each studio gains a smoke test per
  tab, so the split has a safety net where there was none.
- **The container keeps the file path and the export name**, so the lazy imports in `LauncherHub.tsx`
  and the direct import in `App.tsx` do not change.

## 3. Non-goals

- **A state layer.** Extracting the state into contexts or a reducer is FE-3, and it is a different
  change: this one moves rendering, not ownership. The container still holds every hook it holds now.
- **Reducing the API calls or the derived state.** The duplicate `characterPortraits` effects and the
  thirty-prop `ChronicleView` are F-2's findings, in `App.tsx`.
- **Any visual or behavioural change.** No class, no handler, no condition changes.
- **Sharing code between the two studios.** They look alike and are not the same: one edits a system
  with a script and a test suite, the other a world with starter entities. A shared abstraction would
  need a parameter for every difference.

## 4. Design

### The layout of the files

```text
components/SystemsStudio.tsx          the container (path and export unchanged)
components/studios/system/SystemList.tsx          the master column
components/studios/system/SystemHeader.tsx        the header, sub-tabs and toast
components/studios/system/SystemManifestTab.tsx   tab 1
components/studios/system/SystemRulesTab.tsx      tab 2
components/studios/system/SystemMechanicsTab.tsx  tab 2b
components/studios/system/SystemScriptTab.tsx     tab 3
components/studios/system/SystemTestsTab.tsx      tab 5
components/studios/world/WorldList.tsx
components/studios/world/WorldHeader.tsx
components/studios/world/WorldLoreTab.tsx         tab 1, including the artwork block
components/studios/world/WorldPromptTab.tsx       tab 2
components/studios/world/WorldEntitiesTab.tsx     tab 3
```

The dialogs stay in the container: each is a modal whose open flag and callbacks are the container's,
and they are already small after FE-2 gave them the modal shell.

### What a view takes

Each view declares its own props interface from what it actually reads. `SystemList` is the clearest
example: the systems, the selection, the draft, and the four callbacks that change them. A tab takes
the form values it edits and the setter for each, which is verbose and is the point: the compiler then
says exactly what a tab depends on, and a test can render one tab without the other four.

Form state is the case that decides the shape. The container holds twenty-odd `useState` values that
belong to one form, and passing twenty pairs to a tab is unreadable. So each form view takes its values
in one object **of its own fields**, not the container's state object:

```tsx
export interface ManifestFormValues {
  name: string;
  slugID: string;
  version: string;
  description: string;
  rulesPrompt: string;
  script: string;
  mechanics: MechanicsSpec;
  creationPreamble: string;
  creationFields: CharacterCreationField[];
}

export interface SystemManifestTabProps {
  values: ManifestFormValues;
  onChange: <K extends keyof ManifestFormValues>(key: K, value: ManifestFormValues[K]) => void;
  ...
}
```

That means the container gains one `changeManifestField` helper, which is the only new logic this
change adds, and it is a switch over a key rather than a behaviour change.

Hmm. Twenty setters turned into one keyed setter is a change in how the container works. It is worth
it for readability, but it is not a verbatim move, so the spec records it as the one deviation: the
helper is mechanical (a `switch` returning the same setters), and every call site passes the same key
it used before.

The alternative, twenty individual props, keeps the container untouched and makes each tab's signature
unreadable. The keyed setter is chosen, and this paragraph is the reason.

### Verification

The risk in a move this size is silence: most of both studios has no test, so a dropped prop or an
inverted condition would only show when someone clicked. Three things bound it:

1. **`tsc` with `strict` and `noUnusedLocals`**, which fails on a prop the view declares and the
   container forgets, and on one passed and never read.
2. **A smoke test per tab**, which does not exist today: render the studio with its APIs mocked, open
   each tab, and assert the one thing that tab is for (the manifest form's name field, the rules
   editor, the tests panel, the entity list). A tab that throws on mount, or that the container stops
   rendering, fails.
3. **The e2e suite**, which already opens the studios in a browser
   (`TestDeletingAWorldEntityKeepsTheStudioUsable`, `TestSettingsStudioRendersEveryTab`, and the
   launcher test).

## 5. Behaviour

- The studios render identically and behave identically.
- A tab can be rendered in a test without the other tabs, which is new.
- The container is the only file that holds state or calls the API; a view calls nothing.

## 6. Testing

- **`SystemList`** and **`WorldList`**: rendered from a fixture, asserting a row's name and that
  clicking it calls the selection callback.
- **One tab per studio** at minimum, chosen by the API call it needs; the rest are covered by the
  container's smoke tests.
- **A smoke test per studio** that opens every tab in turn and asserts a landmark of each.
- `npx tsc --noEmit`, `npx eslint src`, `npx prettier --check .`, `npx vitest run --coverage`,
  `npm run build`, then `mise run test` and `mise run lint`.

## 7. Rollout

One pull request, one commit per extracted view, so each is a readable move on its own: the systems
studio first (it has one extra tab), then the worlds studio.

## 8. Risks

- **The moves are large and mostly untested.** The smoke tests are written before the extraction, not
  after, so each move is checked against a test that already passed.
- **A prop can be silently dropped where the types agree.** `strict` catches a missing prop, and does
  not catch a prop that exists on both sides but is no longer used the same way. Reading each extracted
  region as it moves, and keeping the move verbatim, is the control.
- **The keyed setter is a change, not a move.** It is mechanical and each call site keeps the key it
  had; the form behaviour is asserted by the smoke tests, which type into the fields.
- **Two studios that look alike invite a shared abstraction.** They are kept separate deliberately: a
  shared studio would need a parameter for every difference between them.

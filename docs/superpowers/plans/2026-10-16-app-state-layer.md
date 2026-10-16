# A State Layer for App Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The settings, the portraits and the playback each get one owner, and `App.tsx` becomes a shell that renders a campaign screen from `CampaignScreen.tsx`.
**Architecture:** `SettingsContext` and `CampaignAudioProvider` own what `App.tsx` holds today for those two subjects; `useCharacterPortraits` owns the one derivation the app was doing twice; the campaign screen's chrome moves into a view.
**Tech Stack:** React 19 (context, hooks), TypeScript 5.9 (strict, noUnusedLocals), Vitest + React Testing Library.
**Spec:** `docs/superpowers/specs/2026-10-16-app-state-layer-design.md`
**Bead:** LocalRPG-14a.7 (FE-3)
**Branch:** `feat/fe-3-state-layer`

## File Map

| File | Why |
|---|---|
| `frontend/src/context/SettingsContext.tsx` | new: the one owner of the config |
| `frontend/src/context/SettingsContext.test.tsx` | new: one fetch, and a save applied without another |
| `frontend/src/hooks/useCharacterPortraits.ts` | new: both portrait sources and the merge rule |
| `frontend/src/hooks/useCharacterPortraits.test.ts` | new: a source each, and the merge rule |
| `frontend/src/context/CampaignAudioContext.tsx` | new: the playback state and the actions over it |
| `frontend/src/context/CampaignAudioContext.test.tsx` | new: heard turns and stop |
| `frontend/src/components/CampaignScreen.tsx` | new: the campaign screen's chrome |
| `frontend/src/components/CampaignScreen.test.tsx` | new: the smoke test, written before the split |
| `frontend/src/App.tsx` | becomes the shell: the switch, the providers, the modals |
| `frontend/src/components/ChronicleView.tsx`, `StoryTheater.tsx` | read playback from the context instead of props |
| `frontend/src/components/SettingsStudio.tsx` | reads the config from the context, applies a save back |
| `AGENTS.md` | the state layer's shape and what a provider owns |

## Global Constraints

- **No behaviour change.** Every switch, auto-play rule and interval behaves as it does today.
- **A provider owns its subject end to end**, including the setters the turn stream needs.
- **A consumer reads the context rather than receiving the prop**, which is the point of the change.
- **The smoke test for the campaign screen lands before the split**, as it did for the studios.
- **After every step**: `npx tsc --noEmit`, `npx eslint src` (0 errors), and the tests that cover the
  step.

## Task 1: The settings context

- [ ] `SettingsContext.tsx`: `SettingsProvider` fetches once on mount, holds `AppConfig | null`,
      exposes `config`, `isLoading`, `reload()` and `applyConfig(config)`.
- [ ] `useSettings()` throws outside a provider, like `useFailures` does inside one.
- [ ] Tests: the provider fetches once for two consumers; `applyConfig` replaces the copy without a
      second request; `reload` asks again.
- [ ] `App.tsx` mounts the provider, drops its own `config` state and its fetch, and reads `config`
      from the context.
- [ ] `SettingsStudio` reads the config from the context instead of fetching, and applies the saved
      result back with `applyConfig`; `App`'s `onSaved` re-fetch goes.

## Task 2: The portraits hook

- [ ] `useCharacterPortraits.ts`: both effects and `mergePortraits` move in, with the rule written down.
- [ ] Tests: a chronicle segment produces a portrait; an entity with `has_portrait` produces one; a
      generated portrait does not overwrite a custom one; a custom one does overwrite a generated one.
- [ ] `App.tsx` uses the hook; the two effects and the inline merge go.

## Task 3: The audio provider

- [ ] `CampaignAudioContext.tsx`: the provider owns `serverAudio`, `playbackOwner`, `turnAudioStatus`,
      `segmentAudioStatus`, `segmentAudioProgress`, `playbackLedger`, `streamedKeys`, `audioProgress`,
      `sceneImagePending` and the streamed-speech hook, and exposes the actions over them.
- [ ] Tests: marking a turn heard lists its keys; stopping clears what is playing; the owner rule picks
      the server when it is playing.
- [ ] `App.tsx` mounts the provider and calls its actions from the turn stream.
- [ ] `ChronicleView` loses its eleven audio props and reads `useCampaignAudio`; `StoryTheater` loses
      its nine.

## Task 4: The campaign screen

- [ ] `CampaignScreen.test.tsx`: render the screen against a fixture and assert the chronicle, the
      action console and the drawer pills, against `App.tsx` as it stands.
- [ ] `CampaignScreen.tsx`: the header, the drawer pills, the chronicle container, the console and the
      report strip move in; the props are what the screen reads that is not in a context.
- [ ] `App.tsx` renders it; measure both files and record the result.

## Task 5: Gates and the record

- [ ] `AGENTS.md`: what a provider owns, where the three live, and why the turn stream stayed in the
      shell.
- [ ] `npx prettier --write`, `npx tsc --noEmit`, `npx eslint src`, `npm run build`.
- [ ] `npx vitest run --coverage` against the floors, `mise run test`, `mise run lint`.
- [ ] Open the pull request, wait for CI, close the bead with the evidence.

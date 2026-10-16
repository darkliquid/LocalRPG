# The Campaign Provider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One provider owns the active campaign: its data, its turn stream and its playback. `App.tsx` becomes a shell that renders `CampaignScreen.tsx`, and the chronicle and the theatre read playback from the context instead of from eleven and nine props.
**Architecture:** `context/CampaignContext.tsx` is mounted in `main.tsx`, above the component that chooses a campaign, and takes `{ children }`; the settings come from `SettingsContext`, the failures from `FailuresContext`. The shell keeps the routing between the launcher and the screen, the UI flags and the modals.
**Tech Stack:** React 19 (context, hooks), TypeScript 5.9 (strict, noUnusedLocals), Vitest + React Testing Library.
**Spec:** `docs/superpowers/specs/2026-10-16-app-state-layer-design.md` (the decision), this plan (the rollout)
**Bead:** LocalRPG-5ti, which completes FE-3 (`LocalRPG-14a.7`)
**Branch:** `feat/campaign-provider`

## File Map

| File | Why |
|---|---|
| `frontend/src/context/CampaignContext.tsx` | new: the campaign's data, its turn stream and its playback |
| `frontend/src/context/CampaignContext.test.tsx` | new: the turn stream against a mocked client |
| `frontend/src/components/CampaignScreen.tsx` | new: the campaign screen's chrome |
| `frontend/src/components/CampaignScreen.test.tsx` | new: the smoke test, written before the split |
| `frontend/src/App.tsx` | the shell: the switch, the flags, the modals, the menu bar |
| `frontend/src/main.tsx` | mounts the provider |
| `frontend/src/components/ChronicleView.tsx` | reads playback from the context |
| `frontend/src/components/StoryTheater.tsx` | reads playback from the context |
| `frontend/src/components/ActionConsole.tsx` | reads what it needs from the context |
| `AGENTS.md` | what the campaign provider owns and what the shell keeps |

## Global Constraints

- **No behaviour change.** Every effect, interval, storage key and auto-play rule behaves as it does today.
- **The move is verbatim**: the stream's logic, the load effects and the audio callbacks move, they are not rewritten.
- **The provider is mounted above the component that chooses a campaign**, which is why it owns the selection rather than receiving it.
- **A consumer reads the context instead of the prop**, which is the whole point.
- **The smoke test for the campaign screen lands before the split**, as it did for the studios and for the settings.

## Task 1: The provider and its state

- [ ] `CampaignContext.tsx`: move the campaign's data (`activeGameID` and its storage, `gameState`,
      `chronicle`, `graph`, `entities`, `recap`, `selectedEntity`, `addressed`, `campaignStatus`,
      `campaignError`, `limits`, `rateLimitUntil`, `fundsError`).
- [ ] Move the load effects verbatim: the corpus refresh, the chronicle, the findings, the campaign
      verification and the rate-limit poll.
- [ ] Move `selectGame`, `handleReturnToLauncher` and the storage keys.
- [ ] Tests: the provider loads a campaign's chronicle and entities against a mocked client; a
      dangling id is reported as unavailable rather than trusted.

## Task 2: The turn stream

- [ ] Move `turnInFlight`, `turnStatus`, `pendingAction`, `streamedSegments`, `toolActivity`,
      `turnError`, `pendingCheck`, the stream processor ref and the abort ref.
- [ ] Move `handleTurnEvent`, `beginTurnStream`, `handleActionSubmit`, `handleStopTurn`,
      `handleResolveCheck`, `handleRenegotiate`, `handleBeginStory`, `handleBeginWithAction` verbatim.
- [ ] Tests: a streamed turn appends to the chronicle; a rejected turn reports and does not append;
      stopping a turn clears what is in flight.

## Task 3: The playback

- [ ] Move the audio state and `handlePlayTurnAudio` / `handleStopAudio` verbatim, with
      `audioPollingRef`.
- [ ] `autoPlay` and `volume` are derived from the settings context.
- [ ] Tests: marking a turn heard lists its keys; stopping closes every subscription and idles the
      statuses.

## Task 4: The consumers

- [ ] `ChronicleView` reads `useCampaign()`; its eleven playback props go.
- [ ] `StoryTheater` reads `useCampaign()`; its nine go.
- [ ] `ActionConsole` takes what it needs from the context.
- [ ] The existing tests for each keep passing; a test that renders one of them outside a provider is
      updated to render it inside one.

## Task 5: The screen and the shell

- [ ] `CampaignScreen.test.tsx`: render the screen against the provider and assert the chronicle, the
      console and the drawer pills, against the code as it stands.
- [ ] `CampaignScreen.tsx`: the header, the drawer pills, the chronicle container, the action console
      and the report strip move in.
- [ ] `App.tsx` keeps the launcher/campaign switch, the UI flags, the modals and the menu bar, and
      measures what is left.
- [ ] `main.tsx` mounts `CampaignProvider` inside `SettingsProvider`.

## Task 6: Gates and the record

- [ ] `AGENTS.md`: what the campaign provider owns, what the shell keeps, and the rule that a provider
      is mounted above the component that chooses its subject.
- [ ] `npx prettier --write`, `npx tsc --noEmit`, `npx eslint src`, `npm run build`.
- [ ] `npx vitest run --coverage` against the floors, `mise run test`, `mise run lint`.
- [ ] Open the pull request, wait for CI, close this bead and FE-3 with the evidence.

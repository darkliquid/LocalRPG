# A State Layer for App Design

**Date:** 2026-10-16
**Status:** Proposed
**Issue:** gh-181 (the bead LocalRPG-14a.7)
**Epic:** [#152](https://github.com/darkliquid/LocalRPG/issues/152)
**Proposal:** `docs/architecture/quality-review-2026-10-12.md` (F-2, FE-3)
**Scope:** `frontend/src`. No Go change, no behaviour change.

## 1. Problem

`App.tsx` is **1,532 lines with 48 `useState` hooks**, three `useRef` and six polling effects. It owns
turn streaming, audio playback, the settings, and every drawer's open flag, and it renders the whole
campaign screen as one expression. Measured today:

- `createContext`/`useContext` appear **once** in the frontend, in `context/FailuresContext.tsx`, and
  `useReducer` appears nowhere.
- **`ChronicleView` takes 27 props** and **`StoryTheater` 19**, and eleven of the chronicle's are audio
  playback: the same values, passed again through the same prop list, because both need them.
- **The character portraits are derived twice**, by two effects that build the same record and call the
  same merge: one from the chronicle's segments (`App.tsx:345-380`), one from the entity list
  (`App.tsx:381-409`).
- **The config is held twice.** `App.tsx:197` fetches the settings into its own state, and
  `SettingsStudio.tsx:158` fetches its own copy to edit; a save updates the studio's copy and then
  `App.tsx:1394` re-fetches to update the app's.

## 2. Goals

- **One owner for the settings.** `SettingsContext` holds the config; the studio edits a copy of it and
  applies the saved result back. The second fetch and the re-fetch after save both go.
- **One derivation for the portraits.** A `useCharacterPortraits` hook owns both sources and the merge
  rule, so the ordering rule it encodes ("a custom portrait wins over a generated one, and once it is
  custom it stays") exists once.
- **One owner for playback.** `CampaignAudioProvider` holds the audio state the chronicle, the theatre
  and the action console all read, and exposes it through `useCampaignAudio`, so eleven props stop
  being threaded through two components that do not otherwise share anything.
- **`App.tsx` becomes a shell.** The campaign screen moves into `CampaignScreen.tsx`; App keeps the
  routing between the launcher and the campaign, the settings provider, and the modals whose flags it
  owns.
- **No behaviour change.** Every switch, every auto-play rule and every polling interval behaves as it
  does today.

## 3. Non-goals

- **`useReducer` for everything.** A reducer earns its keep where a state has transitions that must be
  named and tested together (the turn stream is the candidate); converting forty independent booleans
  into one reducer would be a rewrite with no reader benefit. The state layer here is about
  *ownership*, which is what the prop lists are asking for.
- **Moving the turn stream into a provider.** It is the largest and most entangled piece of `App.tsx`,
  and its callbacks are what the audio provider needs rather than the reverse. The stream stays in the
  shell this change, and the next one can take it.
- **The launcher.** `LauncherHub` already owns its own state and its own views.
- **`SettingsStudio`'s internals.** It keeps its editing copy, which is what a form is; only the source
  of that copy and the destination of a save change.

## 4. Design

### The settings

```text
context/SettingsContext.tsx
  SettingsProvider          fetches once, holds AppConfig | null, exposes reload() and applyConfig()
  useSettings()             { config, isLoading, reload, applyConfig }
```

`SettingsStudio` reads the config from the context instead of fetching it, and after a successful save
calls `applyConfig(res.config)`. `App` stops passing an `onSaved` that re-fetches, and reads `config`
from the context for the two things it needs it for (the TTS preferences and the master volume).

A save that the server accepted is the freshest copy there is, so applying it is both cheaper and more
correct than asking for the settings again.

### The portraits

```text
hooks/useCharacterPortraits.ts
  useCharacterPortraits({ chronicle, entities, activeGameID }): Record<string, Portrait>
```

Both effects move in, along with `mergePortraits`, which is the rule they share. The hook keeps the
two dependency arrays as they are: the chronicle effect reacts to the chronicle, the entity effect to
the entity list, and neither runs when the other does.

### The playback

```text
context/CampaignAudioContext.tsx
  CampaignAudioProvider     owns the playback state and the actions over it
  useCampaignAudio()        one object, read where it is needed
```

The provider owns what `App.tsx` holds today for playback: `serverAudio`, `playbackOwner`,
`turnAudioStatus`, `segmentAudioStatus`, `segmentAudioProgress`, `playbackLedger`, `streamedKeys`,
`audioProgress`, `sceneImagePending`, the streamed-speech hook, and the actions
(`playTurnAudio`, `stopAudio`, `markHeard`, `noteTurnAudio`, `noteSegmentAudio`, `noteProgress`,
`noteSceneImage`). The turn stream calls the actions; the chronicle, the theatre and the action console
read the values.

That is what makes the prop lists shrink: `ChronicleView` loses eleven props and `StoryTheater` nine,
because they read what they need from the context rather than receiving it.

**`sceneImagePending` is not audio**, and it moves because it is written by the same stream event
handler and read by the same components; splitting it out would leave two providers doing one job. The
name of the provider says audio, so the field carries a comment saying what it is.

### Why the playback provider did not land with the other two

The audio state is **written by the turn stream**, and the turn stream lives in `App.tsx` with the
campaign's own state: `handleTurnEvent` sets the audio progress, the streamed keys and the ledger, and
`beginTurnStream` resets them. A provider that owns the audio state therefore has to wrap the code that
writes it, and that code is the campaign's, not the audio's.

That leaves two shapes, and the first one is a trap:

- **Two providers, audio inside App.** `App` cannot consume a provider it renders, so the stream's
  writes would have to be passed *into* the provider as callbacks, which is a prop list wearing a
  context's clothes.
- **One campaign provider**, owning the campaign's data, the turn stream and the playback together,
  mounted above `App` and consumed by the screen. This is what the review meant by a campaign
  context, and it is the shape the two landed providers do not need because neither of them is written
  by the stream.

So the state layer is **one campaign provider, not three**, and it carries the App split with it: while
the stream lives in `App.tsx`, no context can own what the stream writes. The settings and the portraits
landed first because they are not written by the stream, and they are what the provider will read.

The provider is mounted in `main.tsx` and is told which campaign is active by the shell, because the
campaign is chosen below it; every action that needs the campaign is either bound to it by the shell or
takes it as an argument.

### App becomes a shell

`CampaignScreen.tsx` takes the campaign screen's chrome: the header, the drawer pills, the chronicle
container, the console and the report strip. `App.tsx` keeps the launcher/campaign switch, the
providers, the modals and the menu bar. What remains in `App.tsx` is the routing and the flags that
decide it.

## 5. Behaviour

- The settings are fetched once per app load, not twice.
- A saved setting reaches the app without a second request.
- The portrait merge rule is applied in one place; its behaviour is unchanged.
- Every audio control behaves as it does today, including the rules that decide whether the browser or
  the server owns playback.
- `App.tsx` is a shell; `CampaignScreen.tsx` is the campaign screen.

## 6. Testing

- **`SettingsContext`**: a test that the provider fetches once and that `applyConfig` replaces the copy
  without another request.
- **`useCharacterPortraits`**: a test per source, and one for the merge rule (a custom portrait wins,
  and a later generated one does not downgrade it).
- **`CampaignAudioProvider`**: a test that a marked turn is listed as heard and that stopping clears
  what is playing.
- **The app's own behaviour**: the existing e2e suite covers the launcher and the studios; the
  campaign screen has no test, so a smoke test that renders `CampaignScreen` against a fixture and
  asserts the chronicle, the console and the drawer pills is written **before** the split.
- `npx tsc --noEmit`, `npx eslint src`, `npx prettier --check .`, `npx vitest run --coverage`,
  `npm run build`, `mise run test`, `mise run lint`.

## 7. Rollout

One pull request, one commit per step: the settings context, the portraits hook, the audio provider
with its consumers, then the App split with its smoke test first.

## 8. Risks

- **The audio state is entangled with the turn stream**, which stays in App. Every action the stream
  calls is exposed by the provider, and a missed one fails `tsc`.
- **A context makes a component harder to render alone.** The provider is exported, so a test renders
  what it needs inside it; that is the same shape as `FailuresProvider`.
- **Two providers around one screen** could re-render more than two props did. Both hold what changes
  while a turn streams, which is already the case, and the values are memoised at the provider.
- **The App split is the largest diff** and the campaign screen has no unit test today, so the smoke
  test lands first, as it did for the studios.

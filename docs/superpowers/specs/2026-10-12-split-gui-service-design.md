# Split the GUI service by domain Design

**Date:** 2026-10-12
**Status:** Proposed
**Issue:** [#167](https://github.com/darkliquid/LocalRPG/issues/167)
**Epic:** [#151](https://github.com/darkliquid/LocalRPG/issues/151)
**Proposal:** `docs/architecture/quality-review-2026-10-12.md` (RD-1, finding R-1)
**Scope:** `pkg/gui/service.go`, and the nine files it becomes. No signature changes.

## 1. Problem

`pkg/gui/service.go` is **6,575 lines** and holds **235 top-level declarations**: 170 methods
on `*Service`, 36 package-level helpers, 6 types and the sentinel errors. `Service` itself
carries about 55 fields spanning persistence, AI orchestration, audio playback, media
caching and the content registry.

It is not one concern. It is the whole application backend plus the state a GUI needs to
drive it, and it is the first file a newcomer opens. Finding "where a turn starts" means
scanning 170 methods of unrelated work, and every feature change touches the same file, so
conflicts between branches are structural rather than incidental.

## 2. Goals

- `service.go` becomes the struct, its construction, its lifecycle and the accessors that
  everything else needs, and nothing else.
- Each remaining file is named for one domain, so the file list alone explains the package.
- No exported signature changes, no behaviour change, and no test changes: this is a move.

## 3. Non-goals

- **Splitting `pkg/gui` into packages.** The review notes that most of this is not a GUI
  concern, and that is true, but moving code between packages changes import cycles and the
  `Service` surface. This proposal moves it between *files*, which is reviewable as a
  permutation, and leaves the package question to a later proposal.
- **Reducing `Service`'s 55 fields or its 170 methods.** A smaller surface is a design
  question. This one only makes the existing surface locatable.
- **`server.go` (2,405 lines) and `types.go` (1,279).** RD-2 owns the routes and FE-5 owns
  the wire types.

## 4. Design

Ten files, one of which is the existing `service.go` reduced to the struct and its
lifecycle. The assignment is exhaustive: every one of the 170 methods appears below exactly
once, and a test asserts that.

| File | Methods |
|---|---|
| `service.go` | 24 |
| `games.go` | 26 |
| `entities.go` | 18 |
| `worlds.go` | 12 |
| `systems.go` | 10 |
| `turns.go` | 15 |
| `audio.go` | 40 |
| `scene.go` | 7 |
| `models.go` | 6 |
| `registry.go` | 12 |

**The rule is "what does this method operate on".** `GetWorldEntity` is a world operation
even though it reads an entity; `GetGameAsset` is a campaign operation even though it
serves a file. Where a method genuinely spans two domains, it goes to the one that owns the
data it reads and writes, and the ambiguity is recorded rather than left to the reader.

Sentinel errors move with the domain that returns them (`ErrWorldNotFound` to `worlds.go`
and so on), and the package-level helpers move with their only caller. `Service`,
`TurnSession`, `ImportContentOptions` and the request DTOs stay in `service.go`, since the
type is what the other files are methods on.

## 5. Behaviour

None. A move between files in one package cannot change behaviour, and the compiler is the
proof: if a helper were left behind, or a file missed its imports, the package would not
build. The one visible difference is `git log` no longer telling you which concern a change
touched, which the new file names fix for the future.

## 6. Testing

- `go build ./...` and `go vet ./...`, which catch a missed file or a stray import.
- The existing suite, unchanged: 63 packages, and `pkg/gui` has the largest test file in the
  repository (`service_test.go`, 57KB), so a mistake in the move is very likely to surface.
- **A test that the assignment is a permutation**, since that is the property this proposal
  actually establishes: it reads the method set of `*Service` by reflection and asserts each
  name appears in the file this spec assigns it to. That is what stops the split decaying
  back into one file, and it is the same instinct as the generated-doc guards.
- `gofmt -l`, `staticcheck`, `mise run lint`.

## 7. Rollout

One commit per file, in the order above, so each is reviewable on its own and a mistake is
bisectable. `service.go` is reduced last, when the others already hold their methods.

## 8. Risks

- **A large rename-shaped diff.** Mitigated by moving whole methods with no edits, one file
  at a time, and by the fact that Go's compiler rejects anything incomplete.
- **The new file list is a judgement call**, and reasonable people would draw the boundaries
  differently: `audio.go` takes 40 methods and `scene.go` 7. The test makes the current
  choice explicit and cheap to change, which is the point of asserting it rather than
  documenting it.
- **`pkg/gui` remains one package**, so this does not fix the import graph, only the
  readable surface. That is deliberate and stated as a non-goal.
- **Churn while other wave-7 work lands.** Wave 7's other items (RD-3, RD-11) touch
  different files, and this one is best landed first, which is why it is first.

## Appendix: method assignment

| Method | File |
|---|---|
| `ApplyOfflinePreset` | `service.go` |
| `CheckOffline` | `service.go` |
| `ClearTrace` | `service.go` |
| `Close` | `service.go` |
| `Config` | `service.go` |
| `GetResolver` | `service.go` |
| `GetSettings` | `service.go` |
| `OpenURL` | `service.go` |
| `SaveSettings` | `service.go` |
| `SetLogger` | `service.go` |
| `SetURLOpener` | `service.go` |
| `SetVersion` | `service.go` |
| `TraceEvents` | `service.go` |
| `appVersion` | `service.go` |
| `applyResolvedPaths` | `service.go` |
| `goBackground` | `service.go` |
| `imageRegistry` | `service.go` |
| `invalidateRegistries` | `service.go` |
| `setStyleWarnings` | `service.go` |
| `store` | `service.go` |
| `storeOrNil` | `service.go` |
| `styleWarnings` | `service.go` |
| `tracePath` | `service.go` |
| `ttsRegistry` | `service.go` |
| `CreateGame` | `games.go` |
| `DeleteGame` | `games.go` |
| `DeleteGameAsset` | `games.go` |
| `GameFolders` | `games.go` |
| `GenerateGameAsset` | `games.go` |
| `GetChronicle` | `games.go` |
| `GetGameAsset` | `games.go` |
| `GetGameState` | `games.go` |
| `GetLocationArt` | `games.go` |
| `GetRecap` | `games.go` |
| `GetTurnContext` | `games.go` |
| `GetWorkingSet` | `games.go` |
| `ListGames` | `games.go` |
| `RestartGame` | `games.go` |
| `SaveGameAsset` | `games.go` |
| `UpdateGameSettings` | `games.go` |
| `assignPlayerVoice` | `games.go` |
| `campaignArtPrompt` | `games.go` |
| `entityState` | `games.go` |
| `forgetGame` | `games.go` |
| `gameAssetSource` | `games.go` |
| `gameAssetURL` | `games.go` |
| `gameCorpus` | `games.go` |
| `gameLock` | `games.go` |
| `protagonistArtSummary` | `games.go` |
| `worldArtStyle` | `games.go` |
| `CreateFolder` | `entities.go` |
| `DeleteFolder` | `entities.go` |
| `DeleteGameEntity` | `entities.go` |
| `GetEntity` | `entities.go` |
| `GetGraph` | `entities.go` |
| `ListEntities` | `entities.go` |
| `ListEntityMemories` | `entities.go` |
| `ListFolders` | `entities.go` |
| `MergeEntities` | `entities.go` |
| `MoveFolder` | `entities.go` |
| `SaveEntity` | `entities.go` |
| `SaveEntityInFolder` | `entities.go` |
| `WorldFolders` | `entities.go` |
| `assertIDIsFree` | `entities.go` |
| `findEntityNote` | `entities.go` |
| `hasCustomPortrait` | `entities.go` |
| `rewriteInboundLinks` | `entities.go` |
| `scanAndEnrichCharacters` | `entities.go` |
| `CreateWorld` | `worlds.go` |
| `DeleteWorld` | `worlds.go` |
| `DeleteWorldEntity` | `worlds.go` |
| `GenerateWorldAsset` | `worlds.go` |
| `GetWorld` | `worlds.go` |
| `GetWorldAsset` | `worlds.go` |
| `GetWorldEntity` | `worlds.go` |
| `ListWorlds` | `worlds.go` |
| `SaveWorldAsset` | `worlds.go` |
| `SaveWorldEntity` | `worlds.go` |
| `UpdateWorld` | `worlds.go` |
| `writeWorld` | `worlds.go` |
| `DeleteSystem` | `systems.go` |
| `DeleteSystemScenario` | `systems.go` |
| `GetSystem` | `systems.go` |
| `ListReferenceSystems` | `systems.go` |
| `ListSystems` | `systems.go` |
| `SaveSystem` | `systems.go` |
| `SaveSystemScenario` | `systems.go` |
| `SystemScenarios` | `systems.go` |
| `TestSystem` | `systems.go` |
| `systemDraftsDir` | `systems.go` |
| `BeginResolveCheck` | `turns.go` |
| `BeginTurn` | `turns.go` |
| `GetEntityTurns` | `turns.go` |
| `ResolveCheck` | `turns.go` |
| `SummaryPending` | `turns.go` |
| `chronicler` | `turns.go` |
| `engagementFor` | `turns.go` |
| `ensureEmbeddingWorker` | `turns.go` |
| `ensureIndexed` | `turns.go` |
| `findTurn` | `turns.go` |
| `logParityMismatch` | `turns.go` |
| `pendingCheckDTO` | `turns.go` |
| `prepareTurn` | `turns.go` |
| `summariseBehind` | `turns.go` |
| `turnDTO` | `turns.go` |
| `AllTTSBatchJobs` | `audio.go` |
| `AudioAvailable` | `audio.go` |
| `AudioPlaying` | `audio.go` |
| `CancelTTSBatch` | `audio.go` |
| `ClearTTSBatch` | `audio.go` |
| `ClipPath` | `audio.go` |
| `CountUncachedBeats` | `audio.go` |
| `DeleteTTSBatch` | `audio.go` |
| `GetCharacterPortrait` | `audio.go` |
| `GetSegmentClips` | `audio.go` |
| `PlaySegmentAudio` | `audio.go` |
| `PlayTurnAudio` | `audio.go` |
| `RegenerateCharacterPortrait` | `audio.go` |
| `ResumePendingBatches` | `audio.go` |
| `ResumeTTSBatch` | `audio.go` |
| `StartTTSBatch` | `audio.go` |
| `StopAudio` | `audio.go` |
| `SubscribeAudioStatus` | `audio.go` |
| `TTSBatchJobs` | `audio.go` |
| `addPortraitListener` | `audio.go` |
| `audioPipeline` | `audio.go` |
| `audioPlayer` | `audio.go` |
| `broadcastAudioFailure` | `audio.go` |
| `broadcastAudioStatus` | `audio.go` |
| `broadcastPortraitReady` | `audio.go` |
| `cancelBatchJobs` | `audio.go` |
| `clipPath` | `audio.go` |
| `clipPlanFor` | `audio.go` |
| `emitTurnClips` | `audio.go` |
| `groupingEnabled` | `audio.go` |
| `ledgerHolds` | `audio.go` |
| `liveGrouping` | `audio.go` |
| `narratorVoiceFor` | `audio.go` |
| `recordTTSUsage` | `audio.go` |
| `setAudioCurrent` | `audio.go` |
| `streamerRuns` | `audio.go` |
| `synthesizeSegment` | `audio.go` |
| `synthesizeTurnGroups` | `audio.go` |
| `turnClipStream` | `audio.go` |
| `voiceFor` | `audio.go` |
| `GenerateTurnSceneImage` | `scene.go` |
| `GetTurnSceneImage` | `scene.go` |
| `broadcastSceneImagePending` | `scene.go` |
| `broadcastSceneImageReady` | `scene.go` |
| `configureSceneBudget` | `scene.go` |
| `saveImageBudget` | `scene.go` |
| `sceneArtResolver` | `scene.go` |
| `DownloadModel` | `models.go` |
| `GetModelsStatus` | `models.go` |
| `SubscribeModelEvents` | `models.go` |
| `TestProvider` | `models.go` |
| `TranscribeAudio` | `models.go` |
| `UnsubscribeModelEvents` | `models.go` |
| `AddRegistrySource` | `registry.go` |
| `ExportContent` | `registry.go` |
| `GenerateAssetPreview` | `registry.go` |
| `HandleRegistryInstall` | `registry.go` |
| `HandleRegistrySearch` | `registry.go` |
| `HandleRegistrySources` | `registry.go` |
| `HandleRegistryUpdates` | `registry.go` |
| `ImportContent` | `registry.go` |
| `ImportContentWithOptions` | `registry.go` |
| `RegistrySources` | `registry.go` |
| `RemoveRegistrySource` | `registry.go` |
| `registryClient` | `registry.go` |

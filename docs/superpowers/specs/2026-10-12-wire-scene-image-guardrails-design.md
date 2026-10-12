# Wire the code that nothing calls Design

**Date:** 2026-10-12
**Status:** Proposed
**Issue:** [#216](https://github.com/darkliquid/LocalRPG/issues/216)
**Epic:** [#151](https://github.com/darkliquid/LocalRPG/issues/151)
**Proposal:** `docs/architecture/quality-review-2026-10-12.md` (R-10, and the ten U1000
findings TQ-4 could not delete)
**Scope:** `pkg/gui/service.go`, `pkg/engine` (read-only), `pkg/provider/imagehttp`, and
the three genuinely dead symbols the same sweep found.

## 1. Problem

`staticcheck` reports ten unused symbols and **none of them is obsolete code**. They fall
into three groups, and only the first is a missing call.

**The scene-image guardrails are wired to nothing.** `configureSceneBudget`
(`pkg/gui/service.go:2575`) is the only caller of the scene worker's `SetBudget`,
`SetApproval`, `SetPrice`, `SetProceduralFallback` and `SetOnSkipped`, and nothing calls
it. The worker is built at `pkg/gui/service.go:2222`, given only `SetOnReady`, and handed
to the orchestrator:

```go
sceneWorker := engine.NewSceneWorker(s.resolver, imgClient)
sceneWorker.SetOnReady(func(gID string, turnNum int, relPath string) {
    s.broadcastSceneImageReady(gID, turnNum, relPath)
})
orchestrator.SetSceneWorker(sceneWorker)
```

So in production the worker plans every scene with `w.budget` at its zero value and
`w.approval` empty. The consequences, read from `scenePlan` at
`pkg/engine/scene_worker.go:452`:

| Guardrail | Consequence of it being unset |
|---|---|
| `SetBudget` | The campaign's `image_budget` is **never enforced**. `ImageBudgetFromManifest` is never consulted, so a metered provider bills without a ceiling. |
| `SetPrice` | Images are generated but never attributed a cost, so the spend ledger under-reports them even for a campaign that set `max_micros`. |
| `SetProceduralFallback` | A beat that the budget would have covered with free procedural art is instead skipped, so "a beat is never imageless because of a limit" does not hold. |
| `SetOnSkipped` | The `scene_image_pending` event never fires, so a client is never told an illustration was skipped. `broadcastSceneImagePending` is dead as a result. |
| `SetApproval` | A campaign that set `image_approval: ask` is silently ignored, so its provider bills without the consent it asked for. |

`imageProviderMetered` and `imagePrice` are unreachable for the same reason: they exist
only to serve `configureSceneBudget`.

**The STT registry is a promise with no caller.** `Service.sttRegistry()`
(`pkg/gui/service.go:249`) builds a `media.STTRegistry` lazily and has no caller, so
`sttReg` is never non-nil and `invalidateRegistries` invalidates nothing. Its siblings
`ttsRegistry` and `imageRegistry` are both used. Meanwhile `TranscribeAudio`
(`pkg/gui/service.go:5518`) resolves its client directly through
`media.NewSTTClientWithSharedKey`, exactly as the TTS side resolves non-named clients
through `ttsClientFor`. So there is no `sttClientFor` twin and no route by which a named
STT instance could be selected: the registry is half of a feature that was never built.

**ComfyUI's usage recorder records nothing.** `comfyUIImageClient.setLastUsage`
(`pkg/provider/imagehttp/client.go:77`) has no caller, so the field it writes is always
zero and `LastUsage()` always returns the zero value. That client's own comment already
says why: "ComfyUI advertises no token or character count."

## 2. Goals

- A campaign's `image_budget` is enforced, its spend is attributed when the ledger can
  price the provider, and a beat the budget stops is still illustrated by the free
  procedural fallback.
- A skipped or approval-gated scene generation is reported on the turn's event stream
  rather than dropped in silence.
- Every remaining `U1000` finding is resolved, so TQ-4's gate can pass.
- No existing campaign changes behaviour.

## 3. Non-goals

- **The approval-request user interface.** `ask` mode needs a player to see a request and
  approve it. Nothing in `frontend/src` handles `scene_image_pending`, so this proposal
  makes `ask` *honest and visible* rather than *usable*: the event reaches the stream and
  a client may act on it, but no client does yet. A follow-up proposal owns the UI.
- **Named-instance STT.** Building it means `sttClientFor`, a config surface and a route;
  that is a feature, not a wiring fix. This proposal removes the unused half instead.
- Changing `ImageBudget`, `ImageApprovalMode` or the worker's plan logic. They are
  correct as they stand; the defect is that nothing configures them.

## 4. Design

**Wire the guardrails at the creation site.** One call, in the block that already
configures the portrait worker the same way:

```go
sceneWorker := engine.NewSceneWorker(s.resolver, imgClient)
sceneWorker.SetOnReady(...)
s.configureSceneBudget(sceneWorker, gameID, cfg)
```

This is behaviour-preserving, and the types are what make it so: `ImageBudget`'s zero
value is documented as *unlimited* ("a campaign that never sets one is unchanged"), and
`ImageApprovalMode` defaults to `auto` even when the manifest is absent or malformed. A
campaign that has never touched these settings therefore gets exactly the generation it
gets today.

**Make the approval gate visible.** `configureSceneBudget` sets `SetOnSkipped` but not
`SetOnPending`, so `ask` mode would return `scenePlan{pending: true}` and report nothing.
It gains a `SetOnPending` that emits the same `scene_image_pending` event, so both
outcomes that withhold an image are announced:

```go
worker.SetOnPending(func(gID string, turnNumber int, job engine.SceneJob) {
    s.broadcastSceneImagePending(gID, turnNumber, "awaiting_approval")
})
```

Both notifiers are already nil-safe (`pkg/engine/scene_worker.go:502`), so a caller that
sets neither is unaffected.

**Delete the three groups that are not features.** Each deletion is the honest reading of
the code, not a sweep for its own sake:

| Symbol | Why deletion is right |
|---|---|
| `Service.sttRegistry`, `sttReg`, `sttRegMu`, its `invalidateRegistries` clause | No caller, and no caller is possible: `TranscribeAudio` resolves directly. Delete the whole group so no half-built path remains. |
| `comfyUIImageClient.setLastUsage` and the field it writes | Nothing records a usage ComfyUI cannot report. `LastUsage()` keeps returning the zero value, which is what its comment already promises. |
| `TurnOrchestrator.generate` | A one-line wrapper over `generateRequest`, with no caller. |
| `export.ScriptCompiler.rootDir` | A field no method reads. |
| `harness.speechFormattingInstruction` | Unused constant whose comment claims backwards compatibility in a 0.x project; `FormatSpeechFormattingInstructions` superseded it. Already named in R-10. |

## 5. Behaviour

| Situation | Before | After |
|---|---|---|
| Campaign with no budget set | Images generate without a ceiling | Unchanged: a zero `ImageBudget` is unlimited |
| Campaign with `image_budget.max_images` | Ceiling ignored | Ceiling enforced; further beats fall back to procedural art |
| Campaign with `max_micros`, priced provider | Spend never attributed | Each image charges the ledger |
| Budget exhausted | Scene skipped, nobody told | Free procedural fallback draws the beat |
| Campaign with `image_approval: ask`, metered provider | Auto-generated anyway | Not generated; a `scene_image_pending` event reports it |
| `TranscribeAudio` | Unchanged | Unchanged; the unused registry is gone |

## 6. Testing

The worker, the budget and the approval mode are already covered, and none of those tests
can see this defect: the bug was a **missing call**, so every function involved behaved
correctly in isolation. A hand-written test is the wrong instrument, and reaching the
orchestrator's scene worker from outside `pkg/engine` would mean adding an accessor that
exists only for the test.

**The regression guard is `staticcheck`'s `U1000`**, and it is a strong one precisely
because it found the bug: `configureSceneBudget` and its helpers are no longer unused, so
deleting the call that wires them makes them unused again and fails the check. The same
holds one step out: `broadcastSceneImagePending` is unexported and becomes unreachable if
neither `SetOnSkipped` nor `SetOnPending` is wired, so the pending wiring is guarded too.
TQ-4 puts that check in `mise run lint` and CI, which is what makes it automatic rather
than a note in a document.

So the verification is:

- `staticcheck ./...` reports **none** of the ten `U1000` findings this proposal owns.
  Confirmed by count: the tree goes from ten to five, and the five that remain are TQ-4's,
  which its own branch deletes.
- The existing `pkg/engine` tests for `ImageBudget`, `ImageApprovalMode` and `scenePlan`
  still pass, so the wiring did not change the logic it feeds.
- `go build`, `go vet`, `gofmt -l`, the full suite and `mise run lint` all pass.
- **A play-a-turn test that asserts an image was skipped, or that procedural art was drawn
  in its place, is deliberately not written here.** It needs a mock image provider and a
  driven turn, and it belongs with the scene-image coverage work where the fixture would be
  reused; inventing a narrower fixture for this one assertion would test the wiring through
  a keyhole and leave the real flow untested.

## 7. Rollout

No migration. Campaigns that never set a budget or an approval mode are unaffected by
construction. A campaign that *did* set `ask` will stop auto-generating, which is what it
asked for; the changelog and the storage article should say so, because until now the
setting did nothing.

## 8. Risks

- **`ask` mode becomes reachable with no UI to answer it.** A campaign in `ask` mode with
  a metered provider now stops generating images and reports each skip. That is the
  documented meaning of the setting, and it is preferable to billing without consent, but
  it is a behaviour change for anyone who set it and saw it ignored. The spec records it,
  and the follow-up UI proposal closes the loop.
- **More `game.yaml` writes.** With a budget set, each generated image is saved back so the
  allowance survives a reload. That is what `SetBudget`'s `save` callback is for, and it
  only happens for campaigns that set a budget.
- **Deleting the STT registry forecloses named STT instances**, since the getter is the way
  in. If that feature is wanted, the deletion is a one-commit revert; leaving unused code
  in place to avoid that is not a reason to keep it.

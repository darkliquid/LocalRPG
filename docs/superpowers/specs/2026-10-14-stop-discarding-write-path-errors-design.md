# Design Spec: Stop Discarding Errors in the Write Path (RD-6)

## Overview
In the current implementation, errors in critical write and state-deserialization paths are silently swallowed (`_ =`), leading to undetectable data loss, corrupt state propagation, and masked database faults:
1. `pkg/storage/store.go`: Corrupt frontmatter JSON stored in SQLite silently results in an unpopulated entity with default/empty metadata and voice.
2. `pkg/engine/timeline.go`: Failed file removal or database deletion during entity de-merge/retirement is swallowed.
3. `pkg/engine/orchestrator.go`: Failed voice assignment saves and working set replacement writes are discarded, and check resolution marshal errors silently emit empty tool replies.
4. `pkg/storage/sync.go`: Any error from `GetEntity` is treated as "entity does not exist" rather than checking for `sql.ErrNoRows`, masking real SQLite failures.
5. `pkg/engine/portrait_worker.go`, `pkg/storage/ttsjobs.go`, `pkg/storage/embedding_worker.go`: Write path results and transactions are unverified.

This design establishes robust error propagation and handling across all of these load-bearing paths while preserving benign error discards (with explicit comments).

## Proposed Changes

### 1. `pkg/storage/store.go`
- Change `unmarshalEntityMeta(ent *entity.Entity, fmJSON string) error`:
  - If `strings.TrimSpace(fmJSON) == ""` return `nil`.
  - Return formatted error if `json.Unmarshal` into `meta` fails.
  - Return formatted error if `json.Marshal(voiceData)` or `json.Unmarshal(voiceBytes, &v)` fails.
- In `GetEntity(id string)`:
  - If `unmarshalEntityMeta` returns an error, return `nil, fmt.Errorf("unmarshal entity %q metadata: %w", id, err)`.
- In `ListEntities()`:
  - If `json.Unmarshal([]byte(fmJSON), &meta)` fails for a non-empty `fmJSON`, return `fmt.Errorf("unmarshal entity %q metadata: %w", summary.ID, err)`.

### 2. `pkg/storage/sync.go`
- In `syncNote(root, path string)`:
  - Inspect `existingErr` from `s.store.GetEntity(ent.ID)`.
  - If `existingErr != nil && !errors.Is(existingErr, sql.ErrNoRows)`: return `syncUnchanged, fmt.Errorf("check existing entity %q: %w", ent.ID, existingErr)`.
  - Distinguish added vs updated with `errors.Is(existingErr, sql.ErrNoRows)`.

### 3. `pkg/engine/timeline.go`
- Update `retirePreviousEntity(...) error`:
  - Return error if `pathutil.ResolveSafeChild` fails.
  - Return error if `os.Remove(prevPath)` returns an error other than `os.ErrNotExist`.
  - Return error if `t.store.DeleteEntity(...)` returns an error.
- Update callers:
  - `foldPreviousIdentity(...) error`
  - `stagePersona(...) error`
  - `stageEntities(...) (map[string]*entity.Entity, error)`
- In `RewindToTurn`:
  - Return error if `t.store.ReplaceWorkingSet(...)` fails.

### 4. `pkg/engine/orchestrator.go`
- In `ProcessActionStream`:
  - Check error on `o.timeline.SaveEntity(ent)` for assigned voice profiles: return `fmt.Errorf("save assigned voice for entity %q: %w", slugID, err)`.
  - Check error on `o.store.ReplaceWorkingSet(workingSet.ToStorageRecords())`: return `fmt.Errorf("replace working set: %w", err)`.
- In `runGenerationLoop`:
  - Check `json.Marshal(resolved)`: on failure, append error tool message to model and log/continue rather than sending empty content.
  - On `onChunk` invocations during session start/continue, return `errStreamListener{err: err}` if `onChunk` fails.

### 5. `pkg/engine/portrait_worker.go`
- In `applyPortrait`: check error on `storage.NewSyncer(w.store).SyncFile(notePath)`.

### 6. `pkg/storage/ttsjobs.go` & `pkg/storage/embedding_worker.go`
- `ttsjobs.go`: check `res.RowsAffected()`.
- `embedding_worker.go`: check `tx.Commit()`.

## Testing & Verification
- Unit test in `pkg/storage/store_test.go`: test `GetEntity` with malformed frontmatter JSON returns an error.
- Unit test in `pkg/storage/sync_test.go`: test database error propagation in `Syncer`.
- Unit test in `pkg/engine/timeline_test.go`: test error propagation when retiring an entity fails.
- All existing tests in `pkg/storage`, `pkg/engine`, `pkg/rules`, `pkg/harness`, `pkg/gui` must pass.
- `mise run test:backend` and `mise run lint` must pass.

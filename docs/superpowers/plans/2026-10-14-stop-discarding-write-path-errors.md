# Implementation Plan: Stop Discarding Errors in the Write Path (RD-6)

Addresses bead `LocalRPG-92l.8` (RD-6: Stop discarding errors in the write path, issue #172).

## Proposed Tasks

### Task 1: Store & Syncer Error Propagation (`pkg/storage`)
- In `pkg/storage/store.go`:
  - Change `unmarshalEntityMeta(ent *entity.Entity, fmJSON string) error`.
  - Handle malformed `fmJSON` and voice serialization errors.
  - In `GetEntity`: propagate `unmarshalEntityMeta` error.
  - In `ListEntities`: return error if unmarshaling `fmJSON` fails.
- In `pkg/storage/sync.go`:
  - Check `errors.Is(existingErr, sql.ErrNoRows)` in `syncNote`.
  - Propagate database errors.
- In `pkg/storage/ttsjobs.go`:
  - Check `RowsAffected` error in `ClearTTSJobsForGame`.
- In `pkg/storage/embedding_worker.go`:
  - Check error on `tx.Commit()`.
- Add unit tests in `pkg/storage/store_test.go` and `pkg/storage/sync_test.go`.

### Task 2: Timeline De-merge & Rewind Error Handling (`pkg/engine`)
- In `pkg/engine/timeline.go`:
  - Update `retirePreviousEntity(...) error` to return errors from path resolution, `os.Remove`, and `t.store.DeleteEntity`.
  - Propagate errors in `foldPreviousIdentity(...) error`, `stagePersona(...) error`, and `stageEntities(...) (map[string]*entity.Entity, error)`.
  - Check error in `RewindToTurn` on `ReplaceWorkingSet`.
- In `pkg/engine/portrait_worker.go`:
  - Propagate `SyncFile` error in `applyPortrait`.
- Add unit tests in `pkg/engine/timeline_test.go`.

### Task 3: Orchestrator Write Paths & Tool Call Error Handling (`pkg/engine`)
- In `pkg/engine/orchestrator.go`:
  - Check error on `o.timeline.SaveEntity(ent)` for assigned voices.
  - Check error on `o.store.ReplaceWorkingSet(...)`.
  - Check `json.Marshal(resolved)` in tool loop and return error response to model instead of empty string.
  - Propagate `onChunk` listener errors in session continuation paths.
- Add/update unit tests in `pkg/engine/orchestrator_failure_test.go`.

### Task 4: Verification & Integration
- Run `go test ./pkg/storage/... ./pkg/engine/...`.
- Run `mise run test:backend`.
- Run `mise run lint`.

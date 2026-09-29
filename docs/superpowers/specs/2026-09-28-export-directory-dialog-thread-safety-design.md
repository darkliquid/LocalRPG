# Export Directory Dialog Thread Safety Design

**Date:** 2026-09-28
**Status:** Implemented (2026-09-28); manual desktop verification outstanding
**Scope:** Make the Wails directory picker safe to call from an HTTP handler goroutine, or move it behind a service binding
**Related:** `cmd/localrpg/gui.go`, `pkg/gui/export.go`, `pkg/gui/server.go`, `frontend/src/components/ExportModal.tsx`; follows `docs/superpowers/specs/2026-09-28-story-export-reachability-design.md`

## 1. Overview & Goals

The export destination picker is installed as a callback on `Service`
(`Service.SetDirectoryPicker`) and invoked from `handleExportRoutes` when the
frontend calls `POST /api/export/choose-directory`. That request is served on an
HTTP handler goroutine, and the callback opens a native Wails dialog
(`app.Dialog.OpenFile()...PromptForSingleSelection()`,
`cmd/localrpg/gui.go`). Wails dialogs must run on the application's main thread;
calling one from an arbitrary goroutine is at best undefined and at worst a
deadlock that freezes the export dialog and the request.

**Goals:**

- Make opening the native picker safe from any goroutine.
- Keep the browser/socket fallback (a path text field) unchanged.
- Keep the change small: no new frontend dependency, no protocol change.

**Non-Goals:**

- Changing the picker's default directory or filter behaviour.
- Removing the HTTP fallback path.
- A general "native dialog" API for other features.

**Success Criteria:**

- The picker opens reliably when `POST /api/export/choose-directory` is served,
  with no main-thread assertion or deadlock.
- When the app is not running (browser/socket mode) the endpoint still returns
  501 and the UI falls back to the text field.
- The picker callback remains testable without a Wails app (a stub is injected).

## 2. Investigation Findings

- The picker is installed in the native-window branch only
  (`cmd/localrpg/gui.go`, the `application.New(...)` branch): it calls
  `app.Dialog.OpenFile().CanChooseDirectories(true)...SetDirectory(defaultDir)`
  and then `PromptForSingleSelection()`. A dismissed dialog is reported as a
  cancel (`"", nil`).
- The endpoint is `POST /api/export/choose-directory` →
  `Service.ChooseExportDirectory` (`pkg/gui/export.go`), which calls the picker.
  The route is registered and span-named in `pkg/gui/server.go`.
- Wails v3 beta.24 exposes a main-thread marshalling helper:
  `application.InvokeSyncWithResultAndError[T](fn func() (T, error)) (T, error)`
  (and `application.InvokeSyncWithError`). This runs the closure on the app's
  main thread and returns its result, which is exactly the guarantee the dialog
  needs.
- `App.RegisterService` binds a Go service for the frontend to call, but the
  frontend has no `@wailsio/runtime` dependency and does not use `window._wails`,
  so a service binding would require adding that runtime to the SPA and branching
  every call site. The HTTP endpoint is the app's only transport today.
- The frontend already handles the 501 fallback
  (`APIClient.chooseExportDirectory` throws `HTTPError(501)`, and
  `ExportModal` then relies on the text field).

## 3. Design

### 3.1 Marshal the dialog onto the main thread

Wrap only the dialog call, so the callback keeps its signature and testability:

```go
svc.SetDirectoryPicker(func(defaultDir string) (string, error) {
    return application.InvokeSyncWithResultAndError(func() (string, error) {
        dialog := app.Dialog.OpenFile().
            CanChooseDirectories(true).
            CanChooseFiles(false).
            CanCreateDirectories(true).
            SetTitle("Choose an export destination")
        if defaultDir != "" {
            dialog = dialog.SetDirectory(defaultDir)
        }
        chosen, err := dialog.PromptForSingleSelection()
        if err != nil {
            // A dismissed dialog is a cancel, not a failure.
            return "", nil
        }
        return chosen, nil
    })
})
```

`InvokeSyncWithResultAndError` returns an error only when the main thread could
not run the closure (for example the app is shutting down), which
`ChooseExportDirectory` surfaces as a normal error and the UI falls back to the
text field.

### 3.2 Keep the fallback and the seam

- `Service.ChooseExportDirectory` is unchanged: nil picker → `ErrNoNativeDialog`
  → HTTP 501.
- The picker remains an injected `func(defaultDir string) (string, error)`, so
  tests keep stubbing it (`TestChooseExportDirectoryUsesThePicker`).
- No route, DTO, or frontend change.

### 3.3 Rejected alternative

A registered Wails service (`app.RegisterService`) with a generated binding is
the more "native" design, but it requires the `@wailsio/runtime` package in the
SPA and a transport branch in `client.ts` for every call. That is a large change
for one dialog; it is recorded under Open Questions as the path if more native
capabilities are wanted later.

## 4. Interfaces

No public interface changes. The only change is the body of the picker closure
installed in `cmd/localrpg/gui.go`.

## 5. Error Handling

| Situation | Behaviour |
| --- | --- |
| Dialog dismissed | Callback returns `"", nil`; UI keeps the current path |
| Main thread unavailable / app shutting down | Callback returns an error; endpoint 500; UI shows it |
| Browser/socket mode | No picker installed; endpoint 501; UI uses the text field |
| Default directory missing | `SetDirectory` is skipped; the OS picks its own default |

## 6. Testing & Verification

- `pkg/gui`: `ChooseExportDirectory` with a stub picker is unchanged and still
  covered.
- Manual: in the Wails window, open the export dialog and pick a folder; confirm
  the app does not freeze, and that cancelling leaves the field unchanged.
- Manual: over `--port`, confirm the Browse button is absent and the text field
  works.
- A build check that the native branch compiles (`go build ./cmd/localrpg`).

## 7. Compatibility & Rollout

- Behaviour-preserving for the frontend and API.
- `InvokeSyncWithResultAndError` is available in the pinned Wails version; no
  dependency change.
- Fully revertible: the closure body is the only edit.

## 8. Open Questions

- Should the app adopt `@wailsio/runtime` and move every native capability
  (dialogs, reveal-in-file-manager, clipboard) behind service bindings?
- If the export modal later offers a "reveal in file manager" action, does it
  share this marshalling need?
- Is there a Wails-sanctioned way to detect "main thread unavailable" so the
  endpoint can answer 501 rather than 500?

## 9. References

- Code: `cmd/localrpg/gui.go` (native window branch),
  `pkg/gui/export.go` (`SetDirectoryPicker`, `ChooseExportDirectory`),
  `pkg/gui/server.go` (`handleExportRoutes`), `frontend/src/components/ExportModal.tsx`,
  `frontend/src/api/client.ts` (`chooseExportDirectory`).
- Specs: `2026-09-28-story-export-reachability-design.md`.
- API: `application.InvokeSyncWithResultAndError`, `App.Dialog.OpenFile`.

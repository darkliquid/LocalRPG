# Explain Panics in JavaScript Host Bindings Design

**Date:** 2026-10-14
**Status:** Proposed
**Issue:** [#173](https://github.com/darkliquid/LocalRPG/issues/173)
**Epic:** [#151](https://github.com/darkliquid/LocalRPG/issues/151)
**Proposal:** `docs/architecture/quality-review-2026-10-12.md` (RD-7, finding R-8)
**Scope:** `pkg/rules/js_engine.go`. No behaviour change.

## 1. Problem

There are twelve `panic(j.vm.ToValue(...))` calls in `pkg/rules/js_engine.go` across the host API bindings:
- `bindStateAPI`: `roll`, `getStat`, `setStat` (3 panics)
- `bindLocationAPI`: `getLocation`, `setLocation` (2 panics)
- `bindAdvancementAPI`: `grantXP` (1 panic)
- `bindHookRegistration`: `onAction`, `onTurnEnd`, `onWorldTick`, `onTurnBegin`, `onCheck`, `onHealthZero` (6 panics)

In Goja (the embedded JavaScript runtime), Go functions registered as callbacks throw JavaScript exceptions by panicking with a `goja.Value` (or error). Goja intercepts these panics internally within `vm.RunProgram`, `vm.RunString`, or function invocation wrappers, converting them to catchable JS exceptions (`try ... catch`) or returning them to Go callers as `*goja.Exception` errors.

To a Go developer reading the codebase, twelve bare `panic` calls in normal control flow look like fatal unhandled crashes rather than deliberate JS exception propagation. Nothing in `js_engine.go` explains this pattern.

## 2. Goals

- Encapsulate the twelve bare `panic` calls behind a dedicated `throwJS(format string, args ...any)` helper on `*JSEngine`.
- Document the Goja exception mechanism clearly on `throwJS` and `bindHostAPI`.
- Verify with unit tests that `throwJS` exceptions are catchable inside JavaScript `try ... catch` blocks and surface as standard errors to Go callers when uncaught.
- Zero behaviour changes: keep exact exception messages and return types.

## 3. Non-goals

- Altering Goja exception handling semantics or changing how JS scripts handle host errors.
- Modifying error strings returned by hooks or host functions.

## 4. Design

### Helper: `(*JSEngine).throwJS`

Add a private helper method to `*JSEngine`:

```go
// throwJS raises a JavaScript exception within the running Goja runtime.
//
// In Goja, host Go callbacks throw JavaScript exceptions by panicking with a
// goja.Value (or error). Goja catches this panic internally within VM execution
// (such as vm.RunString or function invocations), wraps it as a *goja.Exception,
// and returns it to the host caller as an error or passes it to a JS try/catch
// block. This helper encapsulates that convention so host bindings do not use
// unadorned panic calls.
func (j *JSEngine) throwJS(format string, args ...any) {
	if len(args) == 0 {
		panic(j.vm.ToValue(format))
	}
	panic(j.vm.ToValue(fmt.Sprintf(format, args...)))
}
```

### Call-Site Migration

Replace each of the 12 `panic(j.vm.ToValue(...))` call sites:
- `panic(j.vm.ToValue(fmt.Sprintf("roll error: %v", err)))` -> `j.throwJS("roll error: %v", err)`
- `panic(j.vm.ToValue(fmt.Sprintf("getStat error: %v", err)))` -> `j.throwJS("getStat error: %v", err)`
- `panic(j.vm.ToValue(fmt.Sprintf("setStat error: %v", err)))` -> `j.throwJS("setStat error: %v", err)`
- `panic(j.vm.ToValue(fmt.Sprintf("getLocation error: %v", err)))` -> `j.throwJS("getLocation error: %v", err)`
- `panic(j.vm.ToValue(fmt.Sprintf("setLocation error: %v", err)))` -> `j.throwJS("setLocation error: %v", err)`
- `panic(j.vm.ToValue(fmt.Sprintf("grantXP error: %v", err)))` -> `j.throwJS("grantXP error: %v", err)`
- `panic(j.vm.ToValue("onAction handler must be a function"))` -> `j.throwJS("onAction handler must be a function")`
- `panic(j.vm.ToValue("onTurnEnd handler must be a function"))` -> `j.throwJS("onTurnEnd handler must be a function")`
- `panic(j.vm.ToValue("onWorldTick handler must be a function"))` -> `j.throwJS("onWorldTick handler must be a function")`
- `panic(j.vm.ToValue("onTurnBegin handler must be a function"))` -> `j.throwJS("onTurnBegin handler must be a function")`
- `panic(j.vm.ToValue("onCheck handler must be a function"))` -> `j.throwJS("onCheck handler must be a function")`
- `panic(j.vm.ToValue("onHealthZero handler must be a function"))` -> `j.throwJS("onHealthZero handler must be a function")`

### Documentation

Add a package-level and function-level explanation on `bindHostAPI`:
```go
// bindHostAPI registers the host bridge functions and hook callbacks in the VM.
//
// Host callbacks raise JavaScript exceptions by calling j.throwJS(...). In Goja,
// host functions throw JS exceptions by panicking with a goja.Value, which Goja
// catches and surfaces as a *goja.Exception in JS or wrapped in an error returned
// to the Go caller.
```

## 5. Testing

- Existing tests in `pkg/rules` continue to pass without modification.
- Add unit test in `pkg/rules/js_engine_test.go`:
  - Test that calling a host method that fails (e.g. invalid roll notation) inside a JS `try ... catch` block is successfully caught and inspected in JS.
  - Test that uncaught host exceptions surface as expected Go errors from `ExecuteAction` / `vm.RunString`.

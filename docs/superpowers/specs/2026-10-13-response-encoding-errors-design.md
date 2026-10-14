# Response encoding errors Design

**Date:** 2026-10-13
**Status:** Proposed
**Issue:** [#213](https://github.com/darkliquid/LocalRPG/issues/213)
**Epic:** [#154](https://github.com/darkliquid/LocalRPG/issues/154)
**Proposal:** `docs/architecture/quality-review-2026-10-12.md` (SEC-2, finding S-3)
**Scope:** `pkg/gui/server.go`, `pkg/gui/registry.go`, `pkg/debugger/server.go`. No route, status or body changes for a response that encodes.

## 1. Problem

`writeJSON` (`pkg/gui/server.go:370`) marshals straight into the `ResponseWriter` and throws the result away:

```go
func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}
```

The header and the status are written before the encoder runs, and `Encoder.Encode` flushes as it goes, so a value that cannot be marshalled produces a **200 whose body stops mid-object**. The client receives a success it cannot parse, and the server records nothing. Eight more sites spell the same idiom out inline, most of them writing a `201` or `202` first. `writeJSONStatus` has the same shape with an explicit status, and `writeSaveError` writes a `422` before encoding.

This is not hypothetical for this code. Response DTOs are assembled from entity frontmatter, provider payloads and model status, and one unencodable field is enough: a `NaN` in a float that came from a provider's metrics, a field added to a DTO that is a `func` or a channel, or a map with a non-string key. The failure mode is the worst kind, because a truncated body looks like a transport problem to the client and like a client problem to the server.

## 2. Goals

- A response that cannot be encoded is a `500` the client can parse, never a `200` it cannot.
- One helper per HTTP package decides that, so the inline copies stop drifting.
- Byte-identical bodies for every response that does encode.

## 3. Non-goals

- **The NDJSON streams.** `handleTurnSubmit`, `handleResolveCheck`, the world and system generators and the entity-import progress callback all set `200` and then write events through a closure whose error is returned to the caller and acted on where it can still change something. Once the first event is out, the status is committed and an encode failure cannot be reported in the response; what is left discarded is the write of the final event, after an error the caller already knows about. Section 5 records it rather than pretending it is fixed.
- **Logging the failure server-side.** `writeJSON` takes no request and the package has no logger, only `Service.logger`; threading one through a hundred call sites is a backdoor dependency, and the repo's convention of keeping internals out of a response is what the envelope is for. The detail belongs with SEC-1, which owns the error mapper.
- **`w.Write` results that are not JSON** - the debug dashboard's HTML, audio bytes, the streaming frames. Different failure, different handling, and not what the finding is about.

## 4. Design

Marshal first, write second. The status is ours until the first byte, so marshalling before the header is what makes a `500` still possible:

```go
func writeJSON(w http.ResponseWriter, data any) {
	body, err := json.Marshal(data)
	if err != nil {
		writeEncodeFailure(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(append(body, '\n'))
}
```

- **The bytes do not change.** `Encoder.Encode` writes the value and a newline, HTML escaping included, which is what `json.Marshal` does; the newline is added explicitly so a client or test that compares bodies sees the same response.
- **`writeJSONStatus`** marshals before its `WriteHeader`, so an explicit status is only sent for a body that exists.
- **`writeSaveError`** marshals before its `422`, for the same reason. It keeps its shape: a frontmatter failure still carries its line and column.
- **`writeEncodeFailure`** answers `500` through `writeJSONError`, so the client parses the envelope it already parses. The envelope is a map of strings and cannot fail the way the response just did; the message is fixed, because the marshalling error names internal types.
- **The eight inline sites** become calls to those helpers. Several are `201`/`202` followed by an encode, which is exactly `writeJSONStatus`, so the duplication is what made them ignore the result in the first place.
- **`pkg/debugger`** gets a local helper of the same shape for its three handlers. It has its own server and its own error text, and it cannot share `pkg/gui`'s without importing the GUI package for a twelve-line function.

## 5. Behaviour

| | Before | After |
|---|---|---|
| A response that encodes | 200, bytes | 200, the same bytes |
| A response that cannot encode | 200, body truncated mid-object, nothing recorded | 500, `application/json`, the standard error envelope |
| An explicit status (`201`, `202`, `422`) | written, then possibly a truncated body | written only for a body that exists |
| A `202` that carried no `Content-Type` | status, body, and no content type | the same status and body, now declared `application/json` |

**Memory.** The body is buffered before it is written, so a large response (the entity list, a chronicle, a usage report) is held once for the duration of the write. These are JSON payloads of kilobytes, and the copy is the price of being able to answer a fault at all.

**Still discarded, deliberately.** In the stream handlers, the final `_ = writeEvent(event)` / `_ = emit(...)` after a failure: the response is committed, `200` and `ndjson` have gone out, and the caller has already had the error that matters.

## 6. Testing

- **A response that cannot be encoded is a 500 with the envelope.** A handler is driven with a DTO that holds a value `json.Marshal` rejects, and the test asserts the status is `500`, the content type is JSON, and the body parses as the envelope. Without the fix this test fails on the status, which is the point.
- **A response that encodes is byte-identical**, newline included, so the change is provably invisible to every client.
- **The suite covers the rest.** Every route test drives a handler through `httptest`, so all of them exercise the rewritten helper on the way through.
- `gofmt`, `staticcheck`, `mise run test`, `mise run lint`.

## 7. Rollout

One commit. The helper changes first, then the eight inline sites, then the debugger's three; nothing about the ordering is load-bearing, because the compiler sees each site.

## 8. Risks

- **A failure that used to look like a transport error now looks like a server error**, which is a visible change for a client that hit it. That is the point of the proposal, and it is the honest description of what happened.
- **Buffering a large response** doubles the peak for that response, briefly. Bounded by the endpoints' payload sizes, which are kilobytes.
- **A `500` from `writeEncodeFailure` cannot itself be written if the handler already wrote a status.** It cannot in these sites, because every one goes through a helper that marshals first; the streaming sites, which do write a status first, are out of scope by design.

# Playback Ledger Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#68 RB-3](https://github.com/darkliquid/LocalRPG/issues/68)
**Epic:** [#23 Malformed output and playback integrity](https://github.com/darkliquid/LocalRPG/issues/23)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §5 (RB-3)
**Depends on:** [#67 RB-2](https://github.com/darkliquid/LocalRPG/issues/67)
**Scope:** `pkg/gui`, `frontend`

---

## 1. Problem

RB-2 gives the server an authoritative record of what it has sent, and skips a group whose segments
were all heard. Two gaps remain, both on the **client**:

1. **A partially heard clip is replayed from the start.** The streamed-speech hook marks a key heard
   only on `onended` (`frontend/src/hooks/useStreamedSpeech.ts:34-45`), so a clip cut short when the
   turn lands is played again in full by the chronicle. RB-2 suppresses the *server* repeat, but the
   client's own handover still restarts a partial clip.
2. **The browser and device paths are independent.** `useStreamedSpeech` is gated only on an `enabled`
   flag (`frontend/src/hooks/useStreamedSpeech.ts:11-12`), so a misconfiguration can play a clip in
   the browser **and** on the device.

Both are the same missing thing: one shared record of what the player has actually heard, with how
much of it, owned by one place.

## 2. Goals

- **One** heard registry, agreed between server and client, keyed by clip.
- A partially played clip records a **resume offset**, so the remainder plays rather than the whole
  clip again.
- **One owner** of playback at a time: the browser or the device, never both.
- The registry survives the turn handover and a client reconnect within a session.
- No clip plays twice; no clip is skipped.

## 3. Non-goals

- The server-side skip (RB-2) and the record repair (RB-1).
- A cross-session history of what was heard; the ledger is per session.
- Changing when audio is synthesized.

## 4. Design

### 4.1 The ledger

A ledger keyed by clip, valued by how much has been heard:

```go
// Ledger records what the player has heard, so no clip plays twice.
type Ledger struct {
	mu      sync.Mutex
	entries map[string]Entry
}

// Entry is a clip's heard state.
type Entry struct {
	PlayedMS int  // how much of the clip has played
	TotalMS  int  // the clip's length, when known
	Complete bool // heard to the end
}
```

The server owns the **authoritative** ledger for a turn (RB-2's heard set, extended to offsets). The
client keeps a mirror, and the two reconcile at the handover and on reconnect.

### 4.2 Resume instead of replay

When the client is about to play a clip it has partially heard, it seeks to `PlayedMS` and plays the
remainder, rather than from zero. `useStreamedSpeech` records `PlayedMS` on a pause/abort (the clip's
current `currentTime`) rather than discarding the key; the chronicle's play path reads the entry and
seeks.

The entry is marked `Complete` on `onended`, so a later play is a no-op. This replaces the current
"only on ended" rule with "how far".

### 4.3 One owner

The ledger carries the **owner**: `device` or `browser`. When the server begins device playback
(`PlayTurnAudio`), it sets the owner to `device` and broadcasts it; the client's browser player
disables itself while the owner is `device`, and vice versa. The `enabled` flag becomes a derived
value of the owner, not an independent switch.

This is a small protocol addition on the existing playback events: an `owner` field alongside the
completion event (TH-1) and the audio-progress events.

### 4.4 Reconciliation

At the turn handover (streaming stops, finalise begins), the client sends its ledger's offsets for the
turn's clips; the server merges them with its own (the later offset wins) and uses the result for the
finalise pass. On a reconnect, the client resends its ledger and the server merges again.

Because the offsets are per clip and monotonic, the merge is a max, which is idempotent and safe to
repeat.

### 4.5 Storage and lifetime

The ledger is in memory per session, keyed by clip. It is not persisted: a new session starts empty,
which is correct (a fresh session has heard nothing). The server's ledger for a turn is discarded when
the turn's audio is done.

## 5. Behaviour

| Situation | Result |
| --- | --- |
| a clip heard to the end | never plays again |
| a clip heard halfway, then the turn lands | the remainder plays from the offset |
| the same clip requested twice | the second is a no-op |
| device playback active | the browser player is disabled |
| browser playback active | the device is not started |
| a reconnect mid-turn | the client resends offsets; the merge continues |
| a fresh session | an empty ledger |

## 6. Testing

- `pkg/gui`: the ledger merges offsets by max; a complete clip is a no-op; the owner flips and the
  other path is disabled.
- `frontend`: a partial play records an offset; a replay seeks to it; `onended` marks complete; the
  owner disables the browser player.
- An end-to-end test: a turn whose audio is streamed then finalised plays each clip once, resuming a
  partial clip rather than restarting it.
- A regression guard: a clip played once in a simple turn is unchanged.

## 7. Rollout

Additive: a ledger, an owner field, and a resume path. A client that does not send offsets still works
(the server's RB-2 skip applies).

## 8. Risks

- **Offset accuracy.** `currentTime` at a pause is approximate; seeking to it is within a few tens of
  milliseconds, which is inaudible. Good enough.
- **Owner flapping.** A device that reports idle and a browser that starts could flap. The owner is set
  by the server, not inferred from idle, so it changes only on an explicit handover.
- **Reconnect complexity.** The merge is a max and idempotent, so a duplicate resend is harmless.

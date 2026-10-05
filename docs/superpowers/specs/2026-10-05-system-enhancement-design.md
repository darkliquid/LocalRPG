# System Enhancement Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#87 SG-4](https://github.com/darkliquid/LocalRPG/issues/87)
**Epic:** [#26 AI system generation](https://github.com/darkliquid/LocalRPG/issues/26)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §9 (SG-4)
**Depends on:** [#85 SG-2](https://github.com/darkliquid/LocalRPG/issues/85), [#86 SG-3](https://github.com/darkliquid/LocalRPG/issues/86)
**Scope:** `pkg/sysgen`, `pkg/gui`, `frontend`

---

## 1. Problem

SG-1 creates a system and SG-2 makes its schema reliable, but neither touches a system the user
already has. Two everyday needs are unmet:

- **Enhance**: "this d20 system has no skills; add some", "give it an advancement track". The
  mechanics editor (SYS-5) lets the user add them by hand, but the model that could propose fitting
  ones is never asked.
- **Explain**: a user who did not write a system (or forgot it) has no way to get a plain description
  of what its mechanics do. The rules prose is what the GM reads, not the user.

## 2. Goals

- Propose **additions** to an existing system (stats, skills, checks/profiles, advancement) as an
  accept/reject diff.
- Produce a plain **explanation** of a system's mechanics, for the author or the table.
- Validate every addition with the smoke gate (SG-3) before it is offered.
- Never modify an existing declaration without an explicit accept.

## 3. Non-goals

- Creating a system from nothing (SG-1).
- Rewriting a system; additions only (like WG-3 for worlds).
- The editor (SYS-5) and the templates (SG-2); this uses them.

## 4. Design

### 4.1 Enhancement proposals

```go
// Proposal is one proposed addition to a system.
type Proposal struct {
	Kind    string // "stat" | "skill" | "profile" | "advancement"
	Title   string
	Reason  string
	Stat    *core.StatSpec
	Skill   *core.SkillSpec
	Profile *ProfileAddition
	Advancement *core.AdvancementSpec
}

func Propose(ctx context.Context, gen Generator, sys System, instruction string, kinds []string) ([]Proposal, error)
```

The model reads the current system (its manifest, mechanics, script, and rules) and proposes
additions, each with a reason. It does **not** restate what exists; a proposal is additive.

The additions are assembled into the system's `MechanicsSpec` by **appending** to the relevant list,
so an existing stat or skill is never altered. A duplicate id is refused (the same rule the editor
uses).

### 4.2 Validation

The enhanced system (the original plus the accepted additions) runs the smoke gate (SG-3) before it
is offered. An addition that breaks the system (for example a skill naming a stat the model did not
also add) is caught and shown, and the proposal is marked invalid.

This is why SG-2's templates matter: a proposed `profile` is built from a template, so it is valid by
construction, and only the cross-references need checking.

### 4.3 Explanation

```go
// Explain returns a plain-language description of a system's mechanics.
func Explain(ctx context.Context, gen Generator, sys System) (string, error)
```

The model reads the mechanics and the rules prose and writes a short explanation: how a check
resolves, what the stats mean, how advancement works, and what the hooks do. It is prose for a human,
not a machine; it is shown in the studio and can be copied into a README.

### 4.4 Surfaces

- **API**: `POST /api/system/{id}/enhance` (proposals), `POST /api/system/{id}/enhance/apply`, and
  `POST /api/system/{id}/explain`.
- **Studio**: an Enhance action with a diff, and an Explain action that shows the text.

## 5. Behaviour

| Input | Result |
| --- | --- |
| "add skills" | skill proposals, each linked to a declared stat |
| "add an advancement track" | an advancement proposal |
| an addition that breaks the system | the gate fails; the proposal is marked invalid |
| accept some | only the accepted additions are written |
| explain | a plain description of the mechanics |
| no provider | the oracle produces a minimal explanation, no proposals |

## 6. Testing

- `pkg/sysgen`: a stub generator proposes additions of each kind; applying appends without altering
  existing entries; a duplicate id is refused; a proposal that breaks the gate is marked invalid.
- `pkg/gui`: enhance writes nothing; apply writes only accepted; explain returns text; the smoke gate
  runs on the enhanced system.
- `frontend`: the diff renders proposals; apply sends the accepted set; explain shows the text.
- A regression guard: applying no proposals leaves the system unchanged.

## 7. Rollout

Additive: new endpoints and studio actions. No existing system changes without an accept.

## 8. Risks

- **Redundant proposals.** A model may propose what exists. The prompt forbids it and the duplicate-id
  check catches it; the review makes it obvious.
- **Explanation drift.** The explanation describes the mechanics as written; if the mechanics change,
  it is stale. It is a snapshot, not a live view; the studio can regenerate it.
- **Additive only.** A user who wants to change an existing stat uses the editor, not this feature.
  That is deliberate: a diff over additions is safe; a diff over changes is a merge.

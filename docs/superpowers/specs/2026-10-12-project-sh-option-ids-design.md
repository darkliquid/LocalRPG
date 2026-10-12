# Stop project.sh clearing board fields when a wave or area is added Design

**Date:** 2026-10-12
**Status:** Implemented
**Issue:** [#160](https://github.com/darkliquid/LocalRPG/issues/160)
**Epic:** [#150](https://github.com/darkliquid/LocalRPG/issues/150)
**Proposal:** `docs/architecture/quality-review-2026-10-12.md` (TQ-5, findings C-4 and C-5)
**Scope:** `scripts/project.sh` only.

## 1. Problem

Two defects in the project-board tooling, both found while registering the quality-phase
proposals, and both triggered in the process.

**Field values were destroyed by adding a wave.** `ensure_option` rebuilds a
single-select field's whole option list and sends it to `updateProjectV2Field`. It sent
each existing option as `{name, description, color}` with no `id`. The GraphQL input type
`ProjectV2SingleSelectFieldOptionInput` does have `id` (confirmed by schema
introspection), and without it GitHub treats every option as new, deletes the originals,
and clears that field on every item that used one. Adding five waves and six areas wiped
Wave and Area from all twelve items that had them (#122 to #133).

**An option name containing a comma was truncated.** `resolve` packed each field's
options into one comma-joined column and the readers split it on `,`, so
`Wave 10 - Quality: dependencies, docs and polish` was read as two options and
`wave_option 10` returned `Wave 10 - Quality: dependencies`. The epic that used it failed
to create with `could not add to milestone 'Wave 10 - Quality: dependencies': not found`.

A third defect surfaced during the same work: a repository project can be configured to
auto-add new issues, so `add-proposal`'s explicit `gh project item-add` races that
workflow and fails with `Content already exists in this project` whenever it loses. 15 of
58 proposals landed without their fields for this reason.

## 2. Goals

- Adding a wave or an area leaves every existing item's values untouched.
- An option name may contain any character that cannot appear in a field delimiter.
- Adding an issue to the project succeeds whether or not auto-add is enabled.
- `mise run project:*` keeps working unchanged for every other verb.

## 3. Non-goals

- A general refactor of `project.sh`. The edits are the three defects and nothing else.
- Unit tests for the shell script. There is no test harness for it today and building one
  is a larger change than the fix; the verification below is the evidence instead.
- Changing the project's schema, options or vocabulary.

## 4. Design

**Carry each option's identity.** `PROJECT_QUERY` now selects `color description` as well
as `id name`, and the option rows carry the id and colour so `ensure_option` can send
every existing option back with its own identity:

```text
{id: "<id>", name: "<name>", description: "", color: <COLOR>}
```

**Emit one row per option.** `FIELDS_TSV` becomes a tagged table instead of a packed
column, which removes the comma ambiguity entirely and is what makes the id available:

```text
field   <field name>   <field id>
option  <field name>   <option id>  <option name>  <colour>  <description>
```

`field_id`, `option_names`, `has_option` and `option_id` read the rows they want. Option
names may now contain any character except a tab, which is the same constraint the TSV
transport already imposed on field names.

**Treat "already in this project" as success.** A new `ensure_item` helper tries the
explicit add first; on that specific error it reads the item back, retrying briefly
because the auto-add can land a moment later. Genuine failures still abort.

## 5. Behaviour

- `add-wave` / `add-area`: existing options keep their ids, so no item loses a value.
- `wave_option 10` returns the full name; a comma in an option name is inert.
- `add-proposal` / `add-epic`: identical output either way; with auto-add enabled it no
  longer fails, and without it the explicit add still succeeds.
- Any other error from `item-add` aborts with the message rather than being swallowed.

## 6. Testing

Verified against the live project, since the defects are in live API behaviour:

1. Schema introspection confirmed `ProjectV2SingleSelectFieldOptionInput` accepts `id`.
2. After the fix, `option_names Wave` lists ten waves with no truncation and
   `wave_option 10` returns the full name of the wave-ten option.
3. The twelve damaged items were restored and re-listed, showing Wave 5 and their areas
   back in place.
4. `add-proposal` for SEC-2 succeeded through the new `ensure_item` path, and `ensure_item`
   was then used to attach #211, which the racy version had left off the project entirely.
5. The 15 items that had landed without fields were repaired and the board re-listed: all
   64 items (6 epics, 58 proposals) now carry Status, Area, Wave, Effort and Kind.

## 7. Rollout

Already applied. The fix and the description live in one commit; no migration is needed
because the repaired state matches the state before the damage.

## 8. Risks

- **Silent data loss recurs if a future edit drops the `id`.** The comment above
  `ensure_option` states why it is there, and `FIELDS_TSV`'s shape is documented at
  `resolve`, so the next reader has the reason inline.
- **The script still has no tests.** This is the real lesson of the finding: board-write
  tooling was untested. Callers are now defensive, but a test harness for `project.sh`
  (it already supports `PROJECT_SH_SOURCE_ONLY` for exactly this) is worth a proposal of
  its own.
- **Auto-add remains enabled**, so `item-add` will keep racing. `ensure_item` makes that
  harmless; turning auto-add off would be the alternative and would leave the script
  dependent on it being off.

# Stop project.sh clearing board fields when a wave or area is added Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `add-wave`, `add-area`, `add-epic` and `add-proposal` safe to run against a live board.
**Architecture:** Carry each option's id through the field query, emit one TSV row per option, and treat "already in this project" as success.
**Tech Stack:** Bash, `gh` (GraphQL and `gh project`).
**Spec:** `docs/superpowers/specs/2026-10-12-project-sh-option-ids-design.md`

## Global Constraints

- Three defects only; no general refactor of the script.
- Every other `project:*` verb must behave exactly as before.
- Fail loudly on any error that is not the known auto-add race.

---

### Task 1: Confirm the mechanism

- [x] **Step 1: Inspect the GraphQL input type**

Run: `gh auth` then a schema query for `ProjectV2SingleSelectFieldOptionInput`.
Result: it has `id`, `name`, `color` and `description`. Omitting `id` is what made GitHub
replace every option and clear the field.

---

### Task 2: Carry option identity

**Files:**
- Modify: `scripts/project.sh`

- [x] **Step 1: Widen the query**

Select `color description` alongside `id name` on each option.

- [x] **Step 2: Emit tagged rows from `resolve`**

Emit `field` rows and `option` rows rather than one packed comma-joined column, and
document the shape above `resolve`.

- [x] **Step 3: Update the four readers**

`field_id`, `option_names`, `has_option`, `option_id`.

- [x] **Step 4: Send ids back from `ensure_option`**

Build `{id, name, description, color}` for each existing option, and document why.

- [x] **Step 5: Verify parsing**

Run: `PROJECT_SH_SOURCE_ONLY=1 bash -c 'source scripts/project.sh; resolve; option_names Wave; wave_option 10'`
Expected: ten waves, and the full name of wave 10 including its comma.

---

### Task 3: Add `ensure_item`

**Files:**
- Modify: `scripts/project.sh`

- [x] **Step 1: Write the helper**

Try `item-add`; on `already exists in this project`, read the item back with retries;
abort on anything else.

- [x] **Step 2: Use it in `add-epic` and `add-proposal`**

- [x] **Step 3: Verify**

Run: `mise run project:add-proposal` for SEC-2, then confirm it carries all five fields
with `mise run project:show`.

---

### Task 4: Repair the board

- [x] **Step 1: Restore the twelve damaged items**

Set Wave 5 and their areas from a listing taken before the damage.

- [x] **Step 2: Repair the fifteen field-less proposals**

- [x] **Step 3: Attach #211, which the racy add had left off the project**

- [x] **Step 4: Verify the whole board**

Run: `mise run project:list`
Expected: 64 items (6 epics, 58 proposals), every one with Status, Area, Wave, Effort and
Kind.

---

### Task 5: Record the finding

**Files:**
- Modify: `docs/architecture/quality-review-2026-10-12.md`

- [x] **Step 1: Add findings C-4 and C-5** and index them.

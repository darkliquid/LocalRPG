#!/usr/bin/env bash
#
# project.sh manages the development project: its waves, its epics, and the
# proposal issues under them, together with their GitHub Project fields.
#
# The project is identified by PROJECT_ID (see scripts/project.conf) and is
# long-lived: its name can change and its waves, areas, epics, and proposals can
# keep growing. Every field and option is resolved by name at run time, so no
# numeric IDs live here. Run `scripts/project.sh help` for the command list.
#
set -euo pipefail

CONF="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/project.conf"
[ -f "$CONF" ] || { printf 'project: missing %s\n' "$CONF" >&2; exit 1; }
# shellcheck source=scripts/project.conf
. "$CONF"

: "${OWNER:?set OWNER in scripts/project.conf}"
: "${REPO:?set REPO in scripts/project.conf}"
: "${PROJECT_ID:?set PROJECT_ID in scripts/project.conf}"

# Project commands need the `project` scope. An ambient GITHUB_TOKEN (set in CI
# or some shells) may lack it while the keyring login has it, so project calls
# run without the ambient token. On a normal machine the variable is unset and
# this is a no-op.
ghp() { env -u GITHUB_TOKEN gh "$@"; }

die() { printf 'project: %s\n' "$*" >&2; exit 1; }

# --- resolution -------------------------------------------------------------
# PROJECT_NUMBER, PROJECT_TITLE, and FIELDS_TSV are filled by resolve().

PROJECT_NUMBER=""
PROJECT_TITLE=""
FIELDS_TSV=""

# resolve reads the project and its fields and options in one query. It does not
# use `gh project field-list`, which asks for the same data but is throttled far
# more aggressively, and this is the exact shape the rest of the script wants.
PROJECT_QUERY='query($id:ID!){ node(id:$id){ ... on ProjectV2 {
  number title
  fields(first:50){ nodes {
    ... on ProjectV2FieldCommon { id name }
    ... on ProjectV2SingleSelectField { id name options { id name } }
  } }
} } }'

resolve() {
  local out
  out="$(ghp api graphql -f query="$PROJECT_QUERY" -f id="$PROJECT_ID" \
    --jq '.data.node | ("\(.number)\t\(.title)"), (.fields.nodes[] | [.name, .id, ((.options // []) | map("\(.name)=\(.id)") | join(","))] | @tsv)')"
  [ -n "$out" ] || die "project $PROJECT_ID not found (check PROJECT_ID in scripts/project.conf)"
  PROJECT_NUMBER="$(printf '%s\n' "$out" | head -1 | cut -f1)"
  PROJECT_TITLE="$(printf '%s\n' "$out" | head -1 | cut -f2)"
  FIELDS_TSV="$(printf '%s\n' "$out" | tail -n +2)"
}

field_id() { # <field name>
  printf '%s\n' "$FIELDS_TSV" | awk -F'\t' -v f="$1" '$1==f { print $2; exit }'
}

option_names() { # <field name>, one per line
  printf '%s\n' "$FIELDS_TSV" | awk -F'\t' -v f="$1" '
    $1==f { n=split($3,p,","); for(i=1;i<=n;i++){ split(p[i],kv,"="); if(kv[1]!="") print kv[1] } exit }'
}

options_join() { option_names "$1" | awk 'NR>1{printf ", "} {printf "%s", $0}'; }

has_option() { # <field> <option name>
  printf '%s\n' "$FIELDS_TSV" | awk -F'\t' -v f="$1" -v o="$2" '
    $1==f { n=split($3,p,","); for(i=1;i<=n;i++){ split(p[i],kv,"="); if(kv[1]==o) found=1 } }
    END { exit found ? 0 : 1 }'
}

option_id() { # <field> <option name>
  local id
  id="$(printf '%s\n' "$FIELDS_TSV" | awk -F'\t' -v f="$1" -v o="$2" '
    $1==f { n=split($3,p,","); for(i=1;i<=n;i++){ split(p[i],kv,"="); if(kv[1]==o){ print kv[2]; exit } } }')"
  [ -n "$id" ] || die "no option '$2' on the $1 field"
  printf '%s' "$id"
}

# --- vocabulary -------------------------------------------------------------

normalize() { printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | tr ' _' '-'; }

match_option() { # <field> <input>: print the option name, or fail
  local want opt
  want="$(normalize "$2")"
  while IFS= read -r opt; do
    [ -n "$opt" ] || continue
    if [ "$(normalize "$opt")" = "$want" ]; then printf '%s' "$opt"; return 0; fi
  done <<<"$(option_names "$1")"
  return 1
}

status_option() { match_option Status "$1" || die "unknown status: $1 (options: $(options_join Status))"; }
kind_option()   { match_option Kind   "$1" || die "unknown kind: $1 (options: $(options_join Kind))"; }
effort_option() { match_option Effort "$1" || die "unknown effort: $1 (options: $(options_join Effort))"; }
area_option()   { match_option Area   "$1" || die "unknown area: $1 (options: $(options_join Area)); add one with 'mise run project:add-area $1'"; }

wave_option() { # <number>: the "Wave <n> - ..." option name
  local n="$1" name
  name="$(option_names Wave | awk -v n="$n" '$0 == "Wave " n || index($0, "Wave " n " ") == 1 { print; exit }')"
  [ -n "$name" ] || die "no wave $n on the project (add one with 'mise run project:add-wave $n \"...\"')"
  printf '%s' "$name"
}

# kind_label maps a kind to its label, reusing the built-in ones where they exist.
kind_label() {
  case "$1" in
    enhancement | documentation) printf '%s' "$1" ;;
    *) printf 'type/%s' "$1" ;;
  esac
}

palette() { # a GraphQL single-select colour for an index
  local colors=(GRAY BLUE PURPLE YELLOW ORANGE GREEN RED PINK)
  printf '%s' "${colors[$(($1 % ${#colors[@]}))]}"
}

# --- project plumbing -------------------------------------------------------

# dump prints every project item as one TSV row:
#   number  status  wave  area  effort  kind  title  milestone  labels
dump() {
  ghp project item-list "$PROJECT_NUMBER" --owner "$OWNER" --limit 500 --format json \
    --jq '.items[] | [ (.content.number|tostring), .status, .wave, .area, .effort,
                       .kind, .content.title, (.milestone.title // ""),
                       ((.labels // []) | join(",")) ] | @tsv'
}

item_id() { # <issue>
  local n="$1" id
  id="$(ghp project item-list "$PROJECT_NUMBER" --owner "$OWNER" --limit 500 --format json \
        --jq ".items[] | select(.content.number==$n) | .id")"
  [ -n "$id" ] || die "issue #$n is not on the project"
  printf '%s' "$id"
}

set_option() { # <issue> <field-id> <option-id>
  local id
  id="$(item_id "$1")"
  ghp project item-edit --project-id "$PROJECT_ID" --id "$id" \
    --field-id "$2" --single-select-option-id "$3" >/dev/null
}

body_of() { gh issue view "$1" --repo "$REPO" --json body --jq .body; }

epic_of() { printf '%s' "$1" | sed -n 's/^Part of epic #\([0-9][0-9]*\).*/\1/p' | head -1; }

dod_block() { # the definition-of-done checklist, from the DOD config
  local entry key prefix suffix
  for entry in "${DOD[@]}"; do
    IFS='|' read -r key prefix suffix <<<"$entry"
    printf -- '- [ ] %s%s\n' "$prefix" "$suffix"
  done
}

join_by() { # <separator> <item>...
  local sep="$1" out="" x
  shift
  for x in "$@"; do out="${out:+$out$sep}$x"; done
  printf '%s' "$out"
}

# --- growth helpers ---------------------------------------------------------

ensure_option() { # <field> <name>
  local field="$1" name="$2" opts="" i=0 opt
  if has_option "$field" "$name"; then printf "option '%s' already on %s\n" "$name" "$field"; return; fi
  while IFS= read -r opt; do
    [ -n "$opt" ] || continue
    opts="$opts${opts:+, }{name:\"$opt\", description:\"\", color:$(palette "$i")}"; i=$((i+1))
  done <<<"$(option_names "$field")"
  opts="$opts${opts:+, }{name:\"$name\", description:\"\", color:$(palette "$i")}"
  ghp api graphql -f query="mutation { updateProjectV2Field(input: { fieldId: \"$(field_id "$field")\", singleSelectOptions: [$opts] }) { projectV2Field { ... on ProjectV2SingleSelectField { id } } } }" >/dev/null
  printf "option '%s' added to %s\n" "$name" "$field"
  resolve
}

ensure_field() { # <name> <comma-joined options>
  if [ -n "$(field_id "$1")" ]; then printf 'field %s already exists\n' "$1"; return; fi
  ghp project field-create "$PROJECT_NUMBER" --owner "$OWNER" --name "$1" \
    --data-type SINGLE_SELECT --single-select-options "$2" >/dev/null
  printf 'field %s created\n' "$1"
}

ensure_label() { # <name> <color> <description>
  if gh label list --repo "$REPO" --limit 200 --json name --jq '.[].name' | grep -qxF "$1"; then
    printf 'label %s already exists\n' "$1"; return
  fi
  gh label create "$1" --repo "$REPO" --color "$2" --description "$3" >/dev/null
  printf 'label %s created\n' "$1"
}

ensure_milestone() { # <title>
  if gh api "repos/$REPO/milestones?state=all&per_page=100" --jq '.[].title' | grep -qxF "$1"; then
    printf 'milestone %s already exists\n' "$1"; return
  fi
  gh api "repos/$REPO/milestones" -f title="$1" >/dev/null
  printf 'milestone %s created\n' "$1"
}

# --- commands ---------------------------------------------------------------

cmd_list() {
  local status="" wave="" area="" kind=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --status) if [ -n "$2" ]; then status="$(status_option "$2")"; fi; shift 2 ;;
      --wave)   if [ -n "$2" ]; then wave="$(wave_option "$2")"; fi; shift 2 ;;
      --area)   if [ -n "$2" ]; then area="$(area_option "$2")"; fi; shift 2 ;;
      --kind)   if [ -n "$2" ]; then kind="$(kind_option "$2")"; fi; shift 2 ;;
      *) die "list: unknown option $1" ;;
    esac
  done
  dump | sort -t$'\t' -k1,1n | awk -F'\t' -v st="$status" -v wv="$wave" -v ar="$area" -v kd="$kind" '
    BEGIN { printf "%-7s %-12s %-6s %-18s %-6s %-14s %s\n",
                  "ISSUE", "STATUS", "WAVE", "AREA", "EFFORT", "KIND", "TITLE" }
    (st=="" || $2==st) && (wv=="" || $3==wv) && (ar=="" || $4==ar) && (kd=="" || $6==kd) {
      w=$3; sub(/^Wave /, "", w); sub(/ .*/, "", w)
      printf "%-7s %-12s %-6s %-18s %-6s %-14s %s\n", "#"$1, $2, w, $4, $5, $6, $7
    }'
}

cmd_show() {
  [ $# -eq 1 ] || die "usage: show <issue>"
  local n="$1" body epic spec plan deps block
  block="$(ghp project item-list "$PROJECT_NUMBER" --owner "$OWNER" --limit 500 --format json \
    --jq ".items[] | select(.content.number==$n) |
      \"#\(.content.number)  \(.content.title)\n\" +
      \"  status:    \(.status)\n\" +
      \"  wave:      \(.wave)\n\" +
      \"  area:      \(.area)\n\" +
      \"  effort:    \(.effort)\n\" +
      \"  kind:      \(.kind)\n\" +
      \"  milestone: \(.milestone.title // \"-\")\n\" +
      \"  labels:    \((.labels // []) | join(\", \"))\n\" +
      \"  url:       \(.content.url)\"")"
  [ -n "$block" ] || die "issue #$n is not on the project"
  printf '%s\n' "$block"
  body="$(body_of "$n")"
  epic="$(epic_of "$body")"
  spec="$(printf '%s' "$body" | sed -n 's/^- \[.\] Spec written: `\(.*\)`.*/\1/p' | head -1)"
  plan="$(printf '%s' "$body" | sed -n 's/^- \[.\] Plan written: `\(.*\)`.*/\1/p' | head -1)"
  deps="$(printf '%s' "$body" | sed -n 's/^\*\*Depends on:\*\* *//p' | head -1)"
  [ -n "$epic" ] && printf '  epic:      #%s\n' "$epic"
  [ -n "$spec" ] && printf '  spec:      %s\n' "$spec"
  [ -n "$plan" ] && printf '  plan:      %s\n' "$plan"
  [ -n "$deps" ] && printf '  depends:   %s\n' "$deps"
  printf '  dod:       %s\n' "$(printf '%s' "$body" | awk '/^- \[/ {printf "%s ", ($0 ~ /^- \[x\]/ ? "[x]" : "[ ]")}')"
}

cmd_next() {
  local wave=""
  if [ $# -gt 0 ] && [ -n "$1" ]; then wave="$(wave_option "$1")"; fi
  local row
  row="$(dump | awk -F'\t' -v wv="$wave" '
    $6!="epic" && (wv=="" || $3==wv) {
      if ($2=="In Progress") print 0 "\t" $0
      else if ($2=="Planned") print 1 "\t" $0
    }' | sort -t$'\t' -k1,1n -k4,4 -k2,2n | head -1)"
  [ -n "$row" ] || { printf 'nothing ready\n'; return 0; }
  printf '%s' "$row" | awk -F'\t' '{ printf "next: #%s %s  (%s, %s, %s)\n", $2, $8, $3, $4, $5 }'
}

cmd_status() {
  [ $# -eq 2 ] || die "usage: status <issue> <status>"
  local opt
  opt="$(status_option "$2")"
  set_option "$1" "$(field_id Status)" "$(option_id Status "$opt")"
  printf '#%s -> %s\n' "$1" "$opt"
}

cmd_check() {
  local n="" box="" uncheck=0
  while [ $# -gt 0 ]; do
    case "$1" in
      --uncheck) uncheck=1; shift ;;
      *) if [ -z "$n" ]; then n="$1"; elif [ -z "$box" ]; then box="$1"; else die "check: unexpected $1"; fi; shift ;;
    esac
  done
  [ -n "$n" ] && [ -n "$box" ] || die "usage: check <issue> <box> [--uncheck]"
  local entry key prefix suffix found=0 keys=""
  for entry in "${DOD[@]}"; do
    IFS='|' read -r key prefix suffix <<<"$entry"
    keys="$keys${keys:+, }$key"
    if [ "$key" = "$box" ]; then found=1; break; fi
  done
  [ "$found" -eq 1 ] || die "unknown box: $box (keys: $keys)"
  local body new mark
  mark=$([ "$uncheck" -eq 1 ] && printf ' ' || printf 'x')
  body="$(body_of "$n")"
  new="$(printf '%s' "$body" | sed "s/^- \[[ x]\] $prefix/- [$mark] $prefix/")"
  [ "$new" != "$body" ] || die "box '$box' is already set on #$n"
  gh issue edit "$n" --repo "$REPO" --body "$new" >/dev/null
  printf '#%s %s -> [%s]\n' "$n" "$box" "$mark"
}

cmd_scaffold() {
  local n="" slug=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --slug) slug="$2"; shift 2 ;;
      *) if [ -z "$n" ]; then n="$1"; else die "scaffold: unexpected $1"; fi; shift ;;
    esac
  done
  [ -n "$n" ] || die "usage: scaffold <issue> [--slug slug]"
  local title url body epic date spec plan
  title="$(gh issue view "$n" --repo "$REPO" --json title --jq .title)"
  url="https://github.com/$REPO/issues/$n"
  body="$(body_of "$n")"
  epic="$(epic_of "$body")"
  [ -n "$slug" ] || slug="$(printf '%s' "$title" \
    | sed 's/^[A-Za-z][A-Za-z]*-[0-9][0-9]*: *//; s/ *(.*//' \
    | tr 'A-Z' 'a-z' | sed 's/[^a-z0-9][^a-z0-9]*/-/g; s/^-//; s/-$//')"
  date="$(date +%Y-%m-%d)"
  spec="docs/superpowers/specs/${date}-${slug}-design.md"
  plan="docs/superpowers/plans/${date}-${slug}.md"
  [ -e "$spec" ] && die "$spec already exists"
  [ -e "$plan" ] && die "$plan already exists"

  local epic_link="-"
  [ -n "$epic" ] && epic_link="[#$epic](https://github.com/$REPO/issues/$epic)"

  cat > "$spec" <<EOF
# ${title#*: } Design

**Date:** $date
**Status:** Proposed
**Issue:** [#$n]($url)
**Epic:** $epic_link
**Proposal:** \`$REFERENCE\`
**Scope:**

## 1. Problem

## 2. Goals

## 3. Non-goals

## 4. Design

## 5. Behaviour

## 6. Testing

## 7. Rollout

## 8. Risks
EOF

  cat > "$plan" <<EOF
# ${title#*: } Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (\`- [ ]\`) syntax for tracking.

**Goal:**
**Architecture:**
**Tech Stack:**
**Spec:** \`$spec\`

## Global Constraints

---

### Task 1:

**Files:**
- Create:
- Modify:
- Test:

- [ ] **Step 1: Write the failing test**
- [ ] **Step 2: Run it to verify it fails**
- [ ] **Step 3: Write the minimal implementation**
- [ ] **Step 4: Run it to verify it passes**
- [ ] **Step 5: Commit**
EOF

  printf 'created %s\ncreated %s\n' "$spec" "$plan"
  printf 'next: fill them in, then\n'
  printf '  mise run project:check %s spec && mise run project:check %s plan\n' "$n" "$n"
  printf '  mise run project:status %s planned\n' "$n"
}

cmd_add_wave() {
  [ $# -eq 2 ] || die "usage: add-wave <number> <title>"
  local n="$1" title="$2"
  local name="Wave $n - $title"
  ensure_milestone "$name"
  ensure_option Wave "$name"
  ensure_label "wave/$n" 0e8a16 "Wave $n"
  printf 'wave %s ready: %s\n' "$n" "$name"
}

cmd_add_area() {
  [ $# -eq 1 ] || die "usage: add-area <slug>"
  ensure_label "area/$1" 1d76db "Area: $1"
  ensure_option Area "$1"
  printf 'area %s ready\n' "$1"
}

cmd_add_epic() {
  local area="" title="" wave="" summary=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --area)    area="$2"; shift 2 ;;
      --title)   title="$2"; shift 2 ;;
      --wave)    wave="$2"; shift 2 ;;
      --summary) summary="$2"; shift 2 ;;
      *) die "add-epic: unknown option $1" ;;
    esac
  done
  [ -n "$area" ] && [ -n "$title" ] && [ -n "$wave" ] && [ -n "$summary" ] \
    || die "usage: add-epic --area A --title T --wave N --summary S"
  area_option "$area" >/dev/null
  local wname; wname="$(wave_option "$wave")"

  local body url num item
  body="$(printf '%s\n\nPart of the next-phase work.\n\n**Reference:** `%s`\n\n## Proposals\n' "$summary" "$REFERENCE")"
  url="$(gh issue create --repo "$REPO" --title "Epic: $title" --body "$body" \
        --label "area/$area,wave/$wave,type/epic" --milestone "$wname")"
  num="${url##*/}"

  item="$(ghp project item-add "$PROJECT_NUMBER" --owner "$OWNER" --url "$url" --format json --jq .id)"
  ghp project item-edit --project-id "$PROJECT_ID" --id "$item" --field-id "$(field_id Status)" --single-select-option-id "$(option_id Status "$(status_option proposal)")" >/dev/null
  ghp project item-edit --project-id "$PROJECT_ID" --id "$item" --field-id "$(field_id Area)"   --single-select-option-id "$(option_id Area "$area")" >/dev/null
  ghp project item-edit --project-id "$PROJECT_ID" --id "$item" --field-id "$(field_id Wave)"   --single-select-option-id "$(option_id Wave "$wname")" >/dev/null
  ghp project item-edit --project-id "$PROJECT_ID" --id "$item" --field-id "$(field_id Kind)"   --single-select-option-id "$(option_id Kind "$(kind_option epic)")" >/dev/null
  printf 'epic #%s  %s\n' "$num" "$url"
}

cmd_add_proposal() {
  local epic="" id="" title="" area="" wave="" effort="" kind="" summary="" deps="none"
  while [ $# -gt 0 ]; do
    case "$1" in
      --epic)    epic="$2"; shift 2 ;;
      --id)      id="$2"; shift 2 ;;
      --title)   title="$2"; shift 2 ;;
      --area)    area="$2"; shift 2 ;;
      --wave)    wave="$2"; shift 2 ;;
      --effort)  effort="$2"; shift 2 ;;
      --kind)    kind="$2"; shift 2 ;;
      --summary) summary="$2"; shift 2 ;;
      --deps)    deps="$2"; shift 2 ;;
      *) die "add-proposal: unknown option $1" ;;
    esac
  done
  [ -n "$epic" ] && [ -n "$id" ] && [ -n "$title" ] && [ -n "$area" ] \
    && [ -n "$wave" ] && [ -n "$effort" ] && [ -n "$kind" ] && [ -n "$summary" ] \
    || die "usage: add-proposal --epic N --id ID --title T --area A --wave N --effort S|M|L --kind K --summary S [--deps D]"
  area_option "$area" >/dev/null
  local kopt eopt wname
  kopt="$(kind_option "$kind")"
  eopt="$(effort_option "$effort")"
  wname="$(wave_option "$wave")"

  local body url num item child
  body="$(printf '%s\n\nPart of epic #%s.\n\n**Reference:** `%s` (%s)\n**Depends on:** %s\n\n### Definition of done\n%s\n' \
    "$summary" "$epic" "$REFERENCE" "$id" "$deps" "$(dod_block)")"
  url="$(gh issue create --repo "$REPO" --title "$id: $title" --body "$body" \
        --label "area/$area,wave/$wave,effort/$eopt,$(kind_label "$kopt")" --milestone "$wname")"
  num="${url##*/}"

  child="$(gh api "repos/$REPO/issues/$num" --jq .id)"
  ghp api --method POST "repos/$REPO/issues/$epic/sub_issues" -F sub_issue_id="$child" >/dev/null

  item="$(ghp project item-add "$PROJECT_NUMBER" --owner "$OWNER" --url "$url" --format json --jq .id)"
  ghp project item-edit --project-id "$PROJECT_ID" --id "$item" --field-id "$(field_id Status)" --single-select-option-id "$(option_id Status "$(status_option proposal)")" >/dev/null
  ghp project item-edit --project-id "$PROJECT_ID" --id "$item" --field-id "$(field_id Area)"   --single-select-option-id "$(option_id Area "$area")" >/dev/null
  ghp project item-edit --project-id "$PROJECT_ID" --id "$item" --field-id "$(field_id Wave)"   --single-select-option-id "$(option_id Wave "$wname")" >/dev/null
  ghp project item-edit --project-id "$PROJECT_ID" --id "$item" --field-id "$(field_id Effort)" --single-select-option-id "$(option_id Effort "$eopt")" >/dev/null
  ghp project item-edit --project-id "$PROJECT_ID" --id "$item" --field-id "$(field_id Kind)"   --single-select-option-id "$(option_id Kind "$kopt")" >/dev/null
  printf 'proposal #%s  %s\n' "$num" "$url"
}

cmd_link() {
  [ $# -eq 2 ] || die "usage: link <epic> <issue>"
  local child
  child="$(gh api "repos/$REPO/issues/$2" --jq .id)"
  ghp api --method POST "repos/$REPO/issues/$1/sub_issues" -F sub_issue_id="$child" >/dev/null
  printf 'linked #%s under epic #%s\n' "$2" "$1"
}

# cmd_new creates a fresh project with the standard fields, labels, and one seed
# wave and area, then writes its ID into scripts/project.conf. Most users never
# need it: the project is long-lived and grows with add-wave and add-area.
cmd_new() {
  local title=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --title) title="$2"; shift 2 ;;
      *) die "new: unknown option $1" ;;
    esac
  done
  [ -n "$title" ] || die 'usage: new --title "Project title"'

  local created
  created="$(ghp project create --owner "$OWNER" --title "$title" --format json --jq '"\(.number)\t\(.id)"')"
  IFS=$'\t' read -r PROJECT_NUMBER PROJECT_ID <<<"$created"
  [ -n "$PROJECT_ID" ] || die "could not create the project"
  printf 'created project #%s (%s)\n' "$PROJECT_NUMBER" "$title"

  resolve

  # The default Status field's options become the status vocabulary.
  local fid opts="" i=0 s
  fid="$(field_id Status)"
  [ -n "$fid" ] || die "the new project has no Status field"
  for s in "${STATUSES[@]}"; do
    opts="$opts${opts:+, }{name:\"$s\", color:$(palette "$i")}"; i=$((i+1))
  done
  ghp api graphql -f query="mutation { updateProjectV2Field(input: { fieldId: \"$fid\", singleSelectOptions: [$opts] }) { projectV2Field { ... on ProjectV2SingleSelectField { id } } } }" >/dev/null
  printf 'statuses set: %s\n' "$(join_by ', ' "${STATUSES[@]}")"

  ensure_field Wave 'Wave 1 - Foundations'
  ensure_field Area 'general'
  ensure_field Effort "$(join_by , "${EFFORTS[@]}")"
  ensure_field Kind "$(join_by , "${KINDS[@]}")"
  resolve

  ensure_label 'area/general' 1d76db 'Area: general'
  ensure_label 'wave/1' 0e8a16 'Wave 1'
  local e k
  for e in "${EFFORTS[@]}"; do ensure_label "effort/$e" c2e0c6 "Effort $e"; done
  for k in "${KINDS[@]}"; do ensure_label "$(kind_label "$k")" a2eeff "$k"; done
  ensure_milestone 'Wave 1 - Foundations'

  local tmp
  tmp="$(mktemp)"
  sed "s|^PROJECT_ID=.*|PROJECT_ID=\"$PROJECT_ID\"|" "$CONF" >"$tmp" && mv "$tmp" "$CONF"
  printf 'wrote PROJECT_ID=%s to %s\n' "$PROJECT_ID" "$CONF"
}

cmd_help() {
  local keys=() entry key
  for entry in "${DOD[@]}"; do IFS='|' read -r key _ <<<"$entry"; keys+=("$key"); done
  cat <<HELP
project.sh - manage the development project, its epics, and their proposals.

The project is identified by PROJECT_ID in scripts/project.conf and is
long-lived: rename it and keep adding waves, areas, epics, and proposals.

  list    [--status S] [--wave N] [--area A] [--kind K]   list project items
  show    <issue>                                          one item in full
  next    [--wave N]                                       the next ready item
  status  <issue> <status>                                 set the board status
  check   <issue> <box> [--uncheck]                        tick a DoD box
  scaffold <issue> [--slug s]                              create the spec and plan
  add-wave <number> <title>                                add a wave and its milestone
  add-area <slug>                                          add a theme area
  add-epic --area A --title T --wave N --summary S
  add-proposal --epic N --id ID --title T --area A --wave N \\
               --effort S|M|L --kind K --summary S [--deps D]
  link    <epic> <issue>                                   attach a sub-issue
  new     --title T                                        create a fresh project

statuses: $(join_by ', ' "${STATUSES[@]}")
kinds:    $(join_by ', ' "${KINDS[@]}")
efforts:  $(join_by ', ' "${EFFORTS[@]}")
boxes:    $(join_by ', ' "${keys[@]}")
HELP
}

# --- dispatch ---------------------------------------------------------------

# Sourcing the script defines its helpers without running a command, so the
# field and option parsing can be exercised without touching the API.
[ -n "${PROJECT_SH_SOURCE_ONLY:-}" ] && return 0

[ $# -gt 0 ] || { cmd_help; exit 1; }
cmd="$1"; shift
case "$cmd" in
  help | -h | --help) cmd_help; exit 0 ;;
  new) cmd_new "$@"; exit 0 ;;
esac

resolve
case "$cmd" in
  list)         cmd_list "$@" ;;
  show)         cmd_show "$@" ;;
  next)         cmd_next "$@" ;;
  status)       cmd_status "$@" ;;
  check)        cmd_check "$@" ;;
  scaffold)     cmd_scaffold "$@" ;;
  add-wave)     cmd_add_wave "$@" ;;
  add-area)     cmd_add_area "$@" ;;
  add-epic)     cmd_add_epic "$@" ;;
  add-proposal) cmd_add_proposal "$@" ;;
  link)         cmd_link "$@" ;;
  *) die "unknown command: $cmd (run 'project.sh help')" ;;
esac

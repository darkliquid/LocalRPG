#!/usr/bin/env bash
#
# Compare two coverage profiles for the question a behaviour-preserving refactor
# actually has to answer: did a path that used to execute stop executing?
#
#   scripts/coverage-diff.sh BEFORE.prof AFTER.prof
#
# Generate the profiles the same way, from the revision you are comparing:
#
#   git checkout main        && go test -count=1 -covermode=atomic -coverprofile=/tmp/before.prof ./pkg/engine/
#   git checkout my-branch   && go test -count=1 -covermode=atomic -coverprofile=/tmp/after.prof  ./pkg/engine/
#   scripts/coverage-diff.sh /tmp/before.prof /tmp/after.prof
#
# Why not the percentage. A profile entry is `file:range numStmt count`, and
# numStmt is the statement count of the whole basic block that range belongs to,
# not of the lines it spans. cmd/cover emits one counter per *code range* in a
# block, and a comment or a blank line inside a block splits it into another
# range that repeats the whole count. Two functions with identical statements
# therefore score differently once their comments move, and a small range with a
# large count is normal rather than a corrupted profile. The totals and the
# `go tool cover -func` percentages inherit that weighting, so a change in them
# is not evidence that a path was enabled or disabled.
#
# What is evidence. A function that was fully covered and is no longer has a
# block that stopped executing, and no comment, blank line or expression layout
# can cause that. That is what this compares, by file and function name, so a
# function that moved within its file is still the same function. Exit status is
# 1 when one regressed.
set -euo pipefail

before="${1:?usage: coverage-diff.sh BEFORE.prof AFTER.prof}"
after="${2:?usage: coverage-diff.sh BEFORE.prof AFTER.prof}"

for profile in "$before" "$after"; do
    [ -f "$profile" ] || {
        printf 'coverage-diff: %s does not exist\n' "$profile" >&2
        exit 2
    }
done

# fullyCovered prints one `file<TAB>function` per function the profile scores at
# 100%: the functions where every block ran. The percentage is the last field,
# because the tool pads the name with tabs.
fullyCovered() {
    go tool cover -func="$1" |
        awk -F'\t' -v OFS='\t' '$NF == "100.0%" && $1 != "total:" {
            sub(/:[0-9]+:$/, "", $1)
            gsub(/^[ \t]+|[ \t]+$/, "", $2)
            print $1, $2
        }' |
        sort -u
}

# statementPercent prints the statement-weighted total, which is the number the
# floor is measured against and which this script exists to say you cannot read.
statementPercent() {
    go tool cover -func="$1" |
        awk -F'\t' '$1 == "total:" { gsub(/%/, "", $NF); print $NF }'
}

lost="$(comm -23 <(fullyCovered "$before") <(fullyCovered "$after"))"
gained="$(comm -13 <(fullyCovered "$before") <(fullyCovered "$after"))"

printf 'coverage-diff: %s at %s%% of statements\n' "$before" "$(statementPercent "$before")"
printf 'coverage-diff: %s at %s%% of statements\n' "$after" "$(statementPercent "$after")"
printf 'coverage-diff: the percentage is weighted by each block'"'"'s statement count,\n'
printf '               which a comment can move; compare the functions instead.\n'

if [ -n "$gained" ]; then
    printf '\nfunctions fully covered after and not before: %s\n' "$(printf '%s\n' "$gained" | wc -l | tr -d ' ')"
    printf '%s\n' "$gained" | sed 's/^/  + /'
fi

if [ -n "$lost" ]; then
    printf '\nfunctions fully covered before and not after: %s\n' "$(printf '%s\n' "$lost" | wc -l | tr -d ' ')"
    printf '%s\n' "$lost" | sed 's/^/  - /'
    printf '\ncoverage-diff: a function lost a block that used to run; the move is not behaviour preserving.\n' >&2
    exit 1
fi

printf '\ncoverage-diff: no function lost full coverage.\n'
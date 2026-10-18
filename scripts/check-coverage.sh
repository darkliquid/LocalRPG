#!/usr/bin/env bash
#
# Run the Go suite and fail if statement coverage drops below a floor.
#
# Run through `mise run test:backend`, or directly with
# `bash scripts/check-coverage.sh`.
#
# Environment:
#   GO_COVER_MIN   the floor, as a percentage (default 72.0)
#   GO_COVER_KEEP  write the profile here instead of a temporary file
#
# The floor exists so the number can only go up. It sits below the measured total
# rather than on it, because a few tests skip on a machine without a sound card or
# a browser, and a floor that a headless runner cannot clear is a floor nobody
# keeps. Raise it deliberately, in a commit that says why.
set -euo pipefail

min="${GO_COVER_MIN:-72.0}"
profile="${GO_COVER_KEEP:-$(mktemp -t localrpg-cover.XXXXXX.out)}"
kept=0
[ -n "${GO_COVER_KEEP:-}" ] && kept=1
trap '[ "$kept" -eq 1 ] || rm -f "$profile"' EXIT

# -v so a CI log shows which test failed, and one profile for the whole run so the
# total is the project's rather than a package's.
go test -v -count=1 -shuffle=on -covermode=atomic -coverprofile="$profile" ./...

total="$(go tool cover -func="$profile" | awk '/^total:/ {gsub(/%/, "", $3); print $3}')"
[ -n "$total" ] || { printf 'coverage: could not read a total from %s\n' "$profile" >&2; exit 1; }

printf '\nGo statement coverage: %s%% (floor %s%%)\n' "$total" "$min"

if awk -v t="$total" -v m="$min" 'BEGIN { exit (t + 0 < m + 0) ? 0 : 1 }'; then
  printf 'coverage: %s%% is below the %s%% floor\n' "$total" "$min" >&2
  printf 'coverage: add tests, or lower GO_COVER_MIN in a commit that says why\n' >&2
  exit 1
fi

printf 'coverage: %s%% clears the floor\n' "$total"
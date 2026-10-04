#!/usr/bin/env bash
#
# Run Vale over the prose and the source comments this repository tracks.
#
# Run through `mise run lint:prose`, or directly with `bash scripts/lint-prose.sh`.
#
# Environment:
#   STRICT=1    exit non-zero when Vale reports error-level alerts.
#               Default is report-only; see the note below.
#   SUMMARY=1   print counts and the worst offenders instead of every finding.
#               This is what CI uses, because the styles currently find tens of
#               thousands of alerts and a full report would bury the log.
set -euo pipefail

# Vale walks every file its configuration has a section for, and it does not read
# .gitignore. So the file list comes from git, which is exactly the tracked files:
# node_modules, bin, the generated site and the local campaign content are
# excluded by construction rather than by a glob list somebody has to maintain.
mapfile -t files < <(git ls-files -- '*.md' '*.go' '*.ts' '*.tsx' '*.js' '*.jsx')

if [ "${#files[@]}" -eq 0 ]; then
  echo "no tracked markdown or source files to lint"
  exit 0
fi

# Report-only by default. The configured styles currently find thousands of
# error-level alerts, and the largest single source is a technical vocabulary Vale
# has never seen rather than prose that needs rewriting. Gating on that today would
# block every change. Set STRICT=1 once the vocabulary and the rule set are tuned.
strict="${STRICT:-0}"

if [ "${SUMMARY:-0}" = "1" ]; then
  report="$(mktemp)"
  trap 'rm -f "$report"' EXIT
  # --no-exit here regardless: the exit decision is made below from the counts, so
  # that summary mode and full mode agree.
  vale --no-exit --output=JSON "${files[@]}" >"$report"
  node scripts/vale-summary.mjs "$report" "${#files[@]}" && errors=0 || errors=$?
else
  if [ "$strict" = "1" ]; then
    echo "Linting ${#files[@]} tracked files with Vale (strict: errors fail)"
    vale "${files[@]}"
    exit $?
  fi
  echo "Linting ${#files[@]} tracked files with Vale (report only; STRICT=1 to gate)"
  vale --no-exit "${files[@]}"
  exit 0
fi

if [ "$strict" = "1" ] && [ "$errors" -gt 0 ]; then
  echo ""
  echo "Vale reported $errors error-level alert(s); see the full report with: mise run lint:prose"
  exit 1
fi

exit 0

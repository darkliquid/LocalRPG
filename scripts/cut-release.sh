#!/usr/bin/env bash
#
# Cut a release: bump (or set) the version, commit it, tag the commit and push
# the branch and tag. Run through `mise run release:cut`, or directly with
# `bash scripts/cut-release.sh <bump>`.
#
#   mise run release:cut              # bump the patch version
#   mise run release:cut minor        # bump the minor version
#   mise run release:cut major        # bump the major version
#   mise run release:cut 1.4.0        # set an explicit version
#
# Environment:
#   RELEASE_BRANCH  branch the release is cut from (default: main)
#   DRY_RUN=1       print the steps without changing, committing, or pushing
set -euo pipefail

VERSION_FILE="cmd/localrpg/main.go"
RELEASE_BRANCH="${RELEASE_BRANCH:-main}"
DRY_RUN="${DRY_RUN:-0}"
BUMP="${1:-patch}"

die() {
  printf 'release: %s\n' "$*" >&2
  exit 1
}

step() {
  if [[ "$DRY_RUN" == "1" ]]; then
    printf 'DRY RUN: %s\n' "$*"
    return 0
  fi
  "$@"
}

# version_gt returns success when $1 is a greater X.Y.Z than $2.
version_gt() {
  local IFS=.
  local -a a b
  read -r -a a <<<"$1"
  read -r -a b <<<"$2"
  local i
  for i in 0 1 2; do
    if ((10#${a[i]:-0} > 10#${b[i]:-0})); then return 0; fi
    if ((10#${a[i]:-0} < 10#${b[i]:-0})); then return 1; fi
  done
  return 1
}

command -v git >/dev/null 2>&1 || die "git is required"
cd "$(git rev-parse --show-toplevel 2>/dev/null)" 2>/dev/null || die "not inside a git repository"

current="$(sed -nE 's/^var Version = "([^"]+)".*/\1/p' "$VERSION_FILE")"
[[ -n "$current" ]] || die "could not read the version from $VERSION_FILE"
[[ "$current" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "current version '$current' is not a plain X.Y.Z version"

# Bump from whichever is newer, the version in the source or the last tag. A tag
# cut by hand can be ahead of the source, and bumping from a stale literal would
# produce a version that is already released.
baseline="$current"
latest_tag="$(git describe --tags --abbrev=0 --match 'v[0-9]*.[0-9]*.[0-9]*' 2>/dev/null || true)"
if [[ -n "$latest_tag" ]] && version_gt "${latest_tag#v}" "$baseline"; then
  baseline="${latest_tag#v}"
fi

read -r major minor patch <<<"${baseline//./ }"
case "$BUMP" in
  major) next="$((major + 1)).0.0" ;;
  minor) next="$major.$((minor + 1)).0" ;;
  patch) next="$major.$minor.$((patch + 1))" ;;
  *)
    [[ "$BUMP" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] ||
      die "expected major, minor, patch, or an explicit X.Y.Z; got '$BUMP'"
    next="$BUMP"
    ;;
esac

version_gt "$next" "$baseline" || die "new version $next is not greater than the last version $baseline"

[[ -z "$(git status --porcelain)" ]] ||
  die "the working tree has uncommitted changes; commit or stash them first"

branch="$(git rev-parse --abbrev-ref HEAD)"
[[ "$branch" == "$RELEASE_BRANCH" ]] ||
  die "releases are cut from '$RELEASE_BRANCH' (currently on '$branch'); set RELEASE_BRANCH to override"

if git rev-parse -q --verify "refs/tags/v$next" >/dev/null; then
  die "tag v$next already exists"
fi

printf 'release: %s -> %s (branch %s)\n' "$baseline" "$next" "$branch"

if [[ "$DRY_RUN" == "1" ]]; then
  printf 'DRY RUN: set Version in %s to %s\n' "$VERSION_FILE" "$next"
else
  tmp="$(mktemp)"
  sed -E "s/^var Version = \".*\"/var Version = \"$next\"/" "$VERSION_FILE" >"$tmp"
  mv "$tmp" "$VERSION_FILE"
fi

step git add "$VERSION_FILE"
step git commit -m "chore(release): v$next"
step git tag -a "v$next" -m "v$next"
step git push --follow-tags origin "$branch"

if [[ "$DRY_RUN" == "1" ]]; then
  printf 'DRY RUN: nothing was changed\n'
else
  printf 'release: tagged v%s and pushed %s\n' "$next" "$branch"
  printf 'release: .github/workflows/release.yml now builds and publishes it\n'
fi

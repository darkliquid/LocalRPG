#!/usr/bin/env bash
#
# Run lightweight cross-platform Go tests.
# On Windows, skip packages requiring native GUI window DLLs or SQLite file locking.
# On macOS and Linux, run the full test suite.
set -euo pipefail

if [ "${RUNNER_OS:-}" = "Windows" ] || [[ "$(uname -s)" =~ ^(MINGW|MSYS|CYGWIN) ]]; then
  # Exclude GUI/Wails binaries, SQLite-locked stores, and native DLL-dependent media/provider packages on Windows runner.
  packages=$(go list ./... | grep -v -E "(cmd/localrpg|pkg/e2e|pkg/gui|pkg/engine|pkg/storage|pkg/export|pkg/media|pkg/provider)")
  # shellcheck disable=SC2086
  go test -v $packages
else
  go test -v ./...
fi

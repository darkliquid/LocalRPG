#!/usr/bin/env bash
#
# Capture the showcase screenshots the documentation site displays.
#
# The app is launched against a throwaway copy of website/demo, so the caches
# and campaign index it writes never touch the repository, then driven
# headlessly through website/scenarios/screenshots.yaml. Run it with
# `mise run site:screenshots`; the images land in website/screenshots and are
# picked up by the next `mise run site:build`.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

port="${PORT:-8080}"
binary="bin/localrpg"

if [[ ! -x "$binary" ]]; then
  echo "error: $binary is missing; run 'mise run build' first" >&2
  exit 1
fi

# Chrome is not always on PATH (a distribution package installs it under
# /opt), so hand the driver an explicit path when we can find one.
if [[ -z "${CHROME_EXEC:-}" ]]; then
  for candidate in google-chrome chromium chromium-browser /opt/google/chrome/chrome; do
    if command -v "$candidate" >/dev/null 2>&1; then
      CHROME_EXEC="$(command -v "$candidate")"
      break
    fi
    if [[ -x "$candidate" ]]; then
      CHROME_EXEC="$candidate"
      break
    fi
  done
  export CHROME_EXEC
fi

if [[ -z "${CHROME_EXEC:-}" ]]; then
  echo "warning: no Chrome found; set CHROME_EXEC to a browser binary" >&2
fi

demo="$(mktemp -d)"
server_pid=""

cleanup() {
  if [[ -n "$server_pid" ]] && kill -0 "$server_pid" 2>/dev/null; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  rm -rf "$demo"
}
trap cleanup EXIT

cp -R website/demo/. "$demo/"

echo "Starting LocalRPG on 127.0.0.1:${port} against a copy of website/demo"
"$binary" gui --port "$port" --dir "$demo" >"$demo/server.log" 2>&1 &
server_pid=$!

ready=0
for _ in $(seq 1 150); do
  if ! kill -0 "$server_pid" 2>/dev/null; then
    echo "error: the server exited before it was ready:" >&2
    cat "$demo/server.log" >&2
    exit 1
  fi
  if (exec 3<>"/dev/tcp/127.0.0.1/${port}") 2>/dev/null; then
    exec 3>&- || true
    ready=1
    break
  fi
  sleep 0.2
done

if [[ "$ready" -ne 1 ]]; then
  echo "error: the server did not accept connections on port ${port}:" >&2
  cat "$demo/server.log" >&2
  exit 1
fi

mkdir -p website/screenshots

"$binary" debug test-run \
  --scenario website/scenarios/screenshots.yaml \
  --port "$port" \
  --headless \
  --report-dir "$demo/reports"

echo
echo "Screenshots written to website/screenshots:"
ls -1 website/screenshots/*.jpg website/screenshots/*.png 2>/dev/null || echo "  (none captured)"

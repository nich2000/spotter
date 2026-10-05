#!/bin/bash
# Run on the Mac, outside Docker. Never starts Apple collectors from the server.
set -euo pipefail
cd "$(dirname "$0")"
if [[ "$(uname -s)" != Darwin ]]; then
  echo 'Spotter helper requires macOS.' >&2
  exit 1
fi
mkdir -p .cache
chmod 700 .cache
go build -o .cache/spotter-agent ./cmd/spotter-agent
server="${SPOTTER_SERVER_URL:-http://localhost:18080}"
if [[ $# -gt 0 ]]; then
  exec .cache/spotter-agent -server "$server" "$@"
fi
if ! security find-generic-password -a spotter -s "spotter-device-${server#*://}" -w >/dev/null 2>&1; then
  read -r -p 'Код сопряжения из Настройки → MacBook: ' code
  exec .cache/spotter-agent -server "$server" -pair "$code"
fi
exec .cache/spotter-agent -server "$server"

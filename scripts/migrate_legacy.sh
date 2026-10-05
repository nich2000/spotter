#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
source_file="${1:-data/state.json.workspace.json}"
mode="${2:-dry-run}"
if [ "$mode" != dry-run ] && [ "$mode" != import ]; then echo 'Use dry-run or import' >&2; exit 1; fi
umask 077
backup_dir=".cache/migration-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$backup_dir"
cp "$source_file" "$backup_dir/workspace.json"
shasum -a 256 "$backup_dir/workspace.json" > "$backup_dir/SHA256SUMS"
docker compose --env-file .env.docker run --rm -v "$(pwd)/$backup_dir/workspace.json:/migration/workspace.json:ro" api -role "$mode" -legacy /migration/workspace.json

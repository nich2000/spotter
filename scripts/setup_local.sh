#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
umask 077
if [ -e .env.docker ]; then
  echo '.env.docker already exists; preserving credentials.'
  exit 0
fi
python3 - <<'PY'
import secrets,os
with open('.env.docker','x') as f:
    for key in ['POSTGRES_PASSWORD','NATS_PASSWORD','MINIO_ROOT_PASSWORD']:
        f.write(key+'='+secrets.token_hex(24)+'\n')
    f.write('MINIO_ROOT_USER=spotter-local\n')
os.chmod('.env.docker',0o600)
PY
echo 'Created private .env.docker. Start: docker compose --env-file .env.docker up -d --build'

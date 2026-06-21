#!/usr/bin/env bash
# Extended matrix: alternate backends (SQLite, S3/minio), rspamd spam scanning,
# a hard-crash (SIGKILL) durability check, and a concurrency/load burst.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
cd "$HERE"
COMPOSE="docker compose -f docker-compose.ext.yml"
cleanup() { $COMPOSE down -v --remove-orphans >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "== building + starting sidecars and servers =="
$COMPOSE build
$COMPOSE up -d minio rspamd mta-sqlite mta-s3 mta-rspamd

echo "== running extended suite =="
set +e
$COMPOSE run --rm ext-runner
code=$?
set -e
if [ $code -ne 0 ]; then
  echo "== ext-runner failed; recent logs =="
  $COMPOSE logs --tail=30 mta-sqlite mta-s3 mta-rspamd rspamd minio || true
  exit $code
fi

echo "== hard-crash recovery: SIGKILL mta-sqlite then restart =="
docker kill -s KILL "$($COMPOSE ps -q mta-sqlite)" >/dev/null 2>&1 || true
$COMPOSE up -d mta-sqlite
set +e
$COMPOSE run --rm -e CRASH_CHECK=1 ext-runner
code=$?
set -e
exit $code

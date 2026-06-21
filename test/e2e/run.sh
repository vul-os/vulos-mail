#!/usr/bin/env bash
# Bring up the closed-loop mail ecosystem (private DNS + two MTAs), run the full
# protocol + cross-server auth suite, then tear everything down. Exit code is the
# runner's (0 = all checks passed).
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
cd "$HERE"
COMPOSE="docker compose -f docker-compose.e2e.yml"

cleanup() { $COMPOSE down -v --remove-orphans >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "== generating DKIM keys + DNS zone =="
./gen.sh

echo "== building images =="
$COMPOSE build

echo "== starting dns + mail servers =="
$COMPOSE up -d dns mta-a mta-b

echo "== running e2e suite =="
set +e
$COMPOSE run --rm runner
code=$?
set -e

if [ $code -ne 0 ]; then
  echo "== runner failed (exit $code); recent server logs =="
  $COMPOSE logs --tail=40 mta-a mta-b dns || true
fi
exit $code

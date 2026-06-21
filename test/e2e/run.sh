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
$COMPOSE up -d dns mta-a mta-b mta-tls

echo "== running e2e suite =="
set +e
$COMPOSE run --rm runner
code=$?
set -e

if [ $code -ne 0 ]; then
  echo "== runner failed (exit $code); recent server logs =="
  $COMPOSE logs --tail=40 mta-a mta-b mta-tls dns || true
  exit $code
fi

echo "== restart persistence: DKIM key stable + data survives =="
dkim_key() { $COMPOSE logs --no-color mta-a 2>/dev/null | grep -oE 'p=[A-Za-z0-9+/=]+' | tail -1; }
DKIM_BEFORE="$(dkim_key)"
$COMPOSE restart mta-a
set +e
$COMPOSE run --rm -e RESTART_CHECK=1 runner
code=$?
set -e
DKIM_AFTER="$(dkim_key)"
if [ -n "$DKIM_BEFORE" ] && [ "$DKIM_BEFORE" = "$DKIM_AFTER" ]; then
  echo "  [PASS] DKIM public key identical across restart"
else
  echo "  [FAIL] DKIM key changed across restart"; code=1
fi

if [ $code -ne 0 ]; then
  echo "== restart phase failed; recent logs =="
  $COMPOSE logs --tail=40 mta-a || true
fi
exit $code

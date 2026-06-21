#!/usr/bin/env bash
# Fully-containerized webmail UI test (server + seeder + headless Chrome, all in
# Docker — no host Chrome/node needed).
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"; cd "$HERE"
C="docker compose -f docker-compose.web.yml"
cleanup(){ $C down -v --remove-orphans >/dev/null 2>&1 || true; }
trap cleanup EXIT
echo "== building server + puppeteer runner =="
$C build
$C up -d mta
echo "== seeding =="
$C run --rm web-seed
echo "== running webmail UI test (in Docker) =="
set +e; $C run --rm web-runner; code=$?; set -e
[ $code -ne 0 ] && { echo "== mta logs =="; $C logs --tail=20 mta || true; }
exit $code

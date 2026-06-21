#!/usr/bin/env bash
# Local ACME demo (best-effort, opt-in — NOT in `make test-all`).
#
# Our server drives a full ACME order against a local Pebble CA: fetches the
# directory, registers an account, serves the HTTP-01/TLS-ALPN challenge from its
# own listeners, gets validated, and Pebble issues a cert (Pebble runs on our
# private DNS so validation resolves acme.test -> our server). This proves the
# autocert wiring + challenge serving end-to-end.
#
# NOTE: it's flaky/best-effort because the autocert client and Pebble's current
# build disagree on the post-issuance cert *download* step, which makes autocert
# churn orders; whether "Issued certificate" lands inside the trigger window
# varies run-to-run. (autocert works fine against real Let's Encrypt; this is a
# Pebble-version compat quirk, not a server bug.)
set -euo pipefail
HERE="$(cd "$(dirname "$0")/acme" && pwd)"; cd "$HERE"
C="docker compose -f docker-compose.acme.yml"
cleanup(){ $C down -v --remove-orphans >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "== building + starting dns + pebble + mta-acme =="
$C build >/dev/null
$C up -d dns pebble mta-acme >/dev/null
for i in $(seq 1 30); do $C logs mta-acme 2>/dev/null | grep -q "ACME enabled" && break; sleep 1; done

# Don't trigger until the Pebble directory is actually reachable from the server
# (so autocert's first attempt is a real one, not a poisoned early failure).
echo "== waiting for Pebble directory to be reachable from the server =="
for i in $(seq 1 45); do
  $C exec -T mta-acme sh -c 'wget -qO- --no-check-certificate https://pebble:14000/dir 2>/dev/null | grep -q newOrder' && break
  sleep 1
done

echo "== triggering issuance (TLS handshakes with SNI acme.test) =="
for i in $(seq 1 45); do
  $C exec -T mta-acme sh -c 'wget -qO- --no-check-certificate https://acme.test:443/jmap/session >/dev/null 2>&1' || true
  $C logs pebble 2>/dev/null | grep -q "Issued certificate" && break
  sleep 4
done

# Issuance is the end-to-end proof: Pebble only issues after our server served the
# challenge from its own listeners and the authorization went VALID.
$C logs pebble 2>/dev/null | grep -qi "VALID" && echo "  [INFO] ACME: authorization VALID (challenge served by our server)"
if $C logs pebble 2>/dev/null | grep -q "Issued certificate"; then
  echo "  [PASS] ACME: full order completed — $($C logs pebble 2>/dev/null | grep -o 'Issued certificate serial [0-9a-f]*' | head -1) for acme.test"
  echo "=== 1/1 checks passed ==="
else
  echo "  [FAIL] ACME: no certificate issued"
  echo "=== 0/1 checks passed ==="
  $C logs --tail=30 mta-acme pebble || true
  exit 1
fi

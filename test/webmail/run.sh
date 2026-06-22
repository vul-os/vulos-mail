#!/usr/bin/env bash
# Boot the server (plaintext, temp data), seed a few messages, drive the webmail
# SPA in headless Chrome, assert UI behavior, then tear down.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
PORT_JMAP=18080; PORT_MX=18525
USER=alice@vulos.to; PW=pw
DATA="$(mktemp -d)"
BIN="$(mktemp -u)"

# Resolve puppeteer-core: local node_modules, else npm install, else /tmp/shot.
if [ ! -d "$HERE/node_modules/puppeteer-core" ]; then
  (cd "$HERE" && npm install --silent --no-audit --no-fund 2>/dev/null) || true
fi
if [ ! -d "$HERE/node_modules/puppeteer-core" ] && [ -d /tmp/shot/node_modules/puppeteer-core ]; then
  export NODE_PATH=/tmp/shot/node_modules
fi
# Resolve Chrome.
if [ -z "${PUPPETEER_EXECUTABLE_PATH:-}" ]; then
  for c in "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
           "$(command -v google-chrome || true)" "$(command -v chromium || true)"; do
    [ -x "$c" ] && export PUPPETEER_EXECUTABLE_PATH="$c" && break
  done
fi

cleanup() { kill "${SRV:-}" 2>/dev/null || true; rm -rf "$DATA" "$BIN"; }
trap cleanup EXIT

echo "== building React webmail (Vite) =="
( cd "$ROOT/webmail" && npm ci --silent --no-audit --no-fund && npm run build )

echo "== building + booting server (plaintext) =="
( cd "$ROOT" && go build -o "$BIN" ./cmd/vulos-mail )
VULOS_DATA_DIR="$DATA" VULOS_ACCOUNT="$USER" VULOS_PASSWORD="$PW" VULOS_WEBMAIL_DIR="$ROOT/webmail/dist" \
  VULOS_MX_ADDR="127.0.0.1:$PORT_MX" VULOS_SUBMIT_ADDR="127.0.0.1:18587" VULOS_IMAP_ADDR="127.0.0.1:18143" \
  VULOS_JMAP_ADDR="127.0.0.1:$PORT_JMAP" VULOS_METRICS_ADDR="127.0.0.1:18090" "$BIN" >/tmp/webtest-srv.log 2>&1 &
SRV=$!
# wait for readiness
for i in $(seq 1 40); do curl -fsS -u "$USER:$PW" "http://127.0.0.1:$PORT_JMAP/jmap/session" >/dev/null 2>&1 && break; sleep 0.5; done

echo "== seeding mail =="
python3 "$HERE/seed.py" 127.0.0.1 "$PORT_MX" "$USER"
sleep 1

echo "== running webmail UI test =="
BASE_URL="http://127.0.0.1:$PORT_JMAP" VULOS_USER="$USER" VULOS_PW="$PW" node "$HERE/ui_test.cjs"

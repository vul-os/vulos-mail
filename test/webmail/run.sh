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

echo "== building + booting server (plaintext) =="
( cd "$ROOT" && go build -o "$BIN" ./cmd/vulos-mail )
VULOS_DATA_DIR="$DATA" VULOS_ACCOUNT="$USER" VULOS_PASSWORD="$PW" VULOS_WEBMAIL_DIR="$ROOT/webmail" \
  VULOS_MX_ADDR="127.0.0.1:$PORT_MX" VULOS_SUBMIT_ADDR="127.0.0.1:18587" VULOS_IMAP_ADDR="127.0.0.1:18143" \
  VULOS_JMAP_ADDR="127.0.0.1:$PORT_JMAP" VULOS_METRICS_ADDR="127.0.0.1:18090" "$BIN" >/tmp/webtest-srv.log 2>&1 &
SRV=$!
# wait for readiness
for i in $(seq 1 40); do curl -fsS -u "$USER:$PW" "http://127.0.0.1:$PORT_JMAP/jmap/session" >/dev/null 2>&1 && break; sleep 0.5; done

echo "== seeding mail =="
python3 - "$PORT_MX" "$USER" <<'PY'
import smtplib, sys
port, user = int(sys.argv[1]), sys.argv[2]
mails = [("Dana Okoro <boss@acme.io>","Q3 deck + budget","Hi,\n\nAttached are the numbers.\n\n> earlier question\n> answered here\n\nDana"),
         ("Carol <carol@x.test>","lunch friday?","noon works"),
         ("GitHub <noreply@github.com>","[vul-os/vulos-mail] CI passed","green build https://github.com/vul-os/vulos-mail")]
for frm,subj,body in mails:
    s=smtplib.SMTP("127.0.0.1",port,timeout=15)
    s.sendmail("x@y.io",[user],f"From: {frm}\r\nTo: {user}\r\nSubject: {subj}\r\nDate: Sun, 21 Jun 2026 10:00:00 +0000\r\n\r\n{body}\r\n")
    s.quit()
print("seeded", len(mails))
PY
sleep 1

echo "== running webmail UI test =="
BASE_URL="http://127.0.0.1:$PORT_JMAP" VULOS_USER="$USER" VULOS_PW="$PW" node "$HERE/ui_test.cjs"

#!/usr/bin/env bash
# Generates the DKIM signing keys for a.test/b.test (in the PEM format the server
# loads) AND the matching DNS zone (MX / A / SPF / DKIM / DMARC). Run by run.sh
# before bringing the stack up so the published DNS records match what each MTA
# signs with — i.e. SPF/DKIM/DMARC actually verify across the wire.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"

rm -rf "$HERE/data-a" "$HERE/data-b" "$HERE/zones"
mkdir -p "$HERE/data-a/dkim" "$HERE/data-b/dkim" "$HERE/zones"

TXTA="$(cd "$ROOT" && go run ./cmd/dkimgen -domain a.test -keyout "$HERE/data-a/dkim/a.test.pem")"
TXTB="$(cd "$ROOT" && go run ./cmd/dkimgen -domain b.test -keyout "$HERE/data-b/dkim/b.test.pem")"

cat > "$HERE/zones/db.test" <<EOF
\$ORIGIN test.
\$TTL 60
@   IN SOA ns.test. admin.test. ( 1 60 60 60 60 )
@   IN NS  ns.test.
ns  IN A   172.28.0.10

; --- a.test (mta-a @ 172.28.0.20) ---
a        IN MX 10 mxa
a        IN TXT "v=spf1 ip4:172.28.0.20 ~all"
mxa      IN A 172.28.0.20
_dmarc.a IN TXT "v=DMARC1; p=quarantine; rua=mailto:postmaster@a.test"
$TXTA

; --- b.test (mta-b @ 172.28.0.30) ---
b        IN MX 10 mxb
b        IN TXT "v=spf1 ip4:172.28.0.30 ~all"
mxb      IN A 172.28.0.30
_dmarc.b IN TXT "v=DMARC1; p=quarantine; rua=mailto:postmaster@b.test"
$TXTB
EOF

echo "generated $HERE/zones/db.test + DKIM keys"

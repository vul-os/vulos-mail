// Command dkimgen generates a DKIM signing key for the e2e test harness: it
// writes the private key in the same PEM format the server loads
// (dataDir/dkim/<domain>.pem) and prints the matching DNS TXT record (split into
// 255-char strings for zone-file compatibility) to stdout.
//
//	go run ./cmd/dkimgen -domain a.test -keyout test/e2e/data-a/dkim/a.test.pem
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vul-os/vulos-mail/internal/dkim"
)

func main() {
	domain := flag.String("domain", "", "signing domain (e.g. a.test)")
	selector := flag.String("selector", "vulos-mail", "DKIM selector")
	keyout := flag.String("keyout", "", "path to write the PEM private key")
	flag.Parse()
	if *domain == "" || *keyout == "" {
		fmt.Fprintln(os.Stderr, "usage: dkimgen -domain <d> -keyout <path> [-selector s]")
		os.Exit(2)
	}

	key, txt, err := dkim.GenerateRSAKey(2048)
	if err != nil {
		fmt.Fprintln(os.Stderr, "generate:", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(*keyout), 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "mkdir:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*keyout, dkim.MarshalPrivateKey(key), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "write key:", err)
		os.Exit(1)
	}

	// Emit a zone line with the TXT value split into <=255-char quoted strings.
	var sb strings.Builder
	for i := 0; i < len(txt); i += 255 {
		end := i + 255
		if end > len(txt) {
			end = len(txt)
		}
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(`"` + txt[i:end] + `"`)
	}
	fmt.Printf("%s._domainkey.%s. IN TXT ( %s )\n", *selector, *domain, sb.String())
}

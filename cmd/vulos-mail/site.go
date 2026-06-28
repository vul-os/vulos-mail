package main

import "embed"

// siteFS holds the standalone marketing landing served for the Vulos Mail origin
// (mail.vulos.org). It is mounted read-only at /site/* and rendered at the exact
// "/" path for signed-out visitors, so it never shadows the webmail SPA, which
// lives at / behind sign-in. Mirrors lilmail/site.go.
//
//go:embed all:site
var siteFS embed.FS

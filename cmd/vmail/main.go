// Command vmail is the (placeholder) entrypoint. Real wiring lands in later
// waves (ingest pipeline, protocol adapters, mtaout). For now it exists so the
// module builds end-to-end.
package main

import "fmt"

func main() {
	fmt.Println("vmail — see docs/DESIGN.md")
}

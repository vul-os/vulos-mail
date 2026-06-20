package server

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestSafeNameNeutralizesTraversal verifies safeName strips path separators and
// ".." so an account address can never escape the data directory when used to
// build its on-disk path.
func TestSafeNameNeutralizesTraversal(t *testing.T) {
	cases := []string{
		"../../etc/passwd",
		"a/../../b",
		"....//....//",
		"..\\..\\windows",
		"/etc/shadow",
		"alice@vulos.to",
	}
	root := "/data"
	for _, in := range cases {
		got := safeName(in)
		if strings.Contains(got, "/") || strings.Contains(got, "\\") {
			t.Errorf("safeName(%q)=%q still contains a path separator", in, got)
		}
		if strings.Contains(got, "..") {
			t.Errorf("safeName(%q)=%q still contains \"..\"", in, got)
		}
		// The resulting path must stay within root.
		full := filepath.Join(root, "accounts", got)
		if !strings.HasPrefix(filepath.Clean(full), filepath.Clean(root)+string(filepath.Separator)) {
			t.Errorf("safeName(%q)=%q escapes root: %q", in, got, full)
		}
	}
}

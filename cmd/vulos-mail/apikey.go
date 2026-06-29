package main

import "crypto/subtle"

// checkAPIKey compares the presented key k against the configured apiKey using
// constant-time comparison to avoid timing side-channels (CWE-208). Returns
// false when apiKey is empty (API key auth disabled).
func checkAPIKey(k, apiKey string) bool {
	if apiKey == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(k), []byte(apiKey)) == 1
}

package main

import "testing"

func TestCheckAPIKey(t *testing.T) {
	// Empty configured key means auth is disabled — all inputs rejected.
	if checkAPIKey("anything", "") {
		t.Error("empty apiKey must reject all presented keys")
	}
	if checkAPIKey("", "") {
		t.Error("empty apiKey must reject empty presented key")
	}

	// Correct key is accepted.
	if !checkAPIKey("supersecret", "supersecret") {
		t.Error("correct key must be accepted")
	}

	// Wrong key is rejected.
	if checkAPIKey("wrongkey", "supersecret") {
		t.Error("wrong key must be rejected")
	}

	// Prefix of the correct key is rejected (length-extension guard).
	if checkAPIKey("supers", "supersecret") {
		t.Error("prefix of correct key must be rejected")
	}

	// Correct key with a trailing byte is rejected.
	if checkAPIKey("supersecretX", "supersecret") {
		t.Error("key with extra trailing byte must be rejected")
	}

	// Empty presented key against a non-empty configured key is rejected.
	if checkAPIKey("", "supersecret") {
		t.Error("empty presented key must be rejected when apiKey is set")
	}
}

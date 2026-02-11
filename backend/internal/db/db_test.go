package db

import "testing"

func TestBuildDSNForLibsqlAddsToken(t *testing.T) {
	dsn, err := buildDSN("libsql://chat.example.turso.io", "abc123")
	if err != nil {
		t.Fatalf("build dsn: %v", err)
	}

	if dsn != "libsql://chat.example.turso.io?authToken=abc123" {
		t.Fatalf("unexpected dsn: %s", dsn)
	}
}

func TestBuildDSNForFileURL(t *testing.T) {
	dsn, err := buildDSN("file:local.db", "ignored")
	if err != nil {
		t.Fatalf("build dsn: %v", err)
	}

	if dsn != "file:local.db" {
		t.Fatalf("unexpected dsn: %s", dsn)
	}
}

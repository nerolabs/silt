package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestADaemonStagesUnderItsStore: a daemon's temporary files go to <store>/tmp, emptied at
// startup, unless the operator chose TMPDIR — a large publish stages ~1.6× the file, and the
// system temp dir is often a small tmpfs.
func TestADaemonStagesUnderItsStore(t *testing.T) {
	store := t.TempDir()
	t.Setenv("TMPDIR", "")
	os.MkdirAll(filepath.Join(store, "tmp", "silt-client-stale"), 0o700)
	if err := useStoreTempDir(store); err != nil {
		t.Fatal(err)
	}
	if got, want := os.TempDir(), filepath.Join(store, "tmp"); got != want {
		t.Fatalf("os.TempDir() = %s, want %s", got, want)
	}
	if _, err := os.Stat(filepath.Join(store, "tmp", "silt-client-stale")); !os.IsNotExist(err) {
		t.Fatal("a stale staging directory survived startup")
	}
	t.Setenv("TMPDIR", "/operator/chose")
	if err := useStoreTempDir(store); err != nil || os.Getenv("TMPDIR") != "/operator/chose" {
		t.Fatalf("an operator's TMPDIR was overridden: %v %s", err, os.Getenv("TMPDIR"))
	}
}

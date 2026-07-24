package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRunWiresTheVersionGate drives the real entrypoint against an
// instance whose tracked dump forges a newer schema version: run() must
// exit 2 before any verb code touches the instance (§8.6 forward-only).
// This is the wiring's own test — the dispatcher's gate hook is a
// variable main assigns, and only an end-to-end refusal proves it did.
func TestRunWiresTheVersionGate(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".selftracked"), 0o755); err != nil {
		t.Fatal(err)
	}
	header := "-- selftracked dump schema_version=99 tasks=0 artifacts=0\n"
	if err := os.WriteFile(filepath.Join(dir, ".selftracked", "dump.sql"), []byte(header), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"selftracked", "list"}

	if got := run(); got != 2 {
		t.Fatalf("run() with a newer-versioned tracked dump exited %d, want 2", got)
	}
}

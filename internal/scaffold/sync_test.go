package scaffold

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSidecarHashesLastDump is INV-351: .selftracked/dump.hash holds the
// SHA-256 of the dump this DB last produced — here, the dump init wrote.
func TestSidecarHashesLastDump(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := writeScaffold(context.Background(), root, false); err != nil {
		t.Fatal(err)
	}
	dumpBytes, err := os.ReadFile(filepath.Join(root, instanceDir, dumpFile))
	if err != nil {
		t.Fatal(err)
	}
	sidecar, err := os.ReadFile(filepath.Join(root, instanceDir, "dump.hash"))
	if err != nil {
		t.Fatalf("sidecar not written by init: %v", err)
	}
	if got, want := strings.TrimSpace(string(sidecar)), sha256Hex(string(dumpBytes)); got != want {
		t.Fatalf("sidecar = %q, want SHA-256 of the dump %q", got, want)
	}
}

// TestTwoWriterDumpConflictsLoudly is INV-003 and INV-362: two writers
// editing dump.sql on divergent branches produce a textual merge conflict,
// not a silent auto-merge — selftracked ships no merge driver, so git's
// default conflict surfacing is exactly the loud failure the single-writer
// axiom's violation must produce.
func TestTwoWriterDumpConflictsLoudly(t *testing.T) {
	t.Parallel()
	root := newRepo(t)
	if err := writeScaffold(context.Background(), root, false); err != nil {
		t.Fatal(err)
	}
	dump := filepath.Join(instanceDir, dumpFile)
	gitCmd(t, root, "add", "-A")
	gitCmd(t, root, "commit", "-q", "-m", "base tracker")
	base := gitOut(t, root, "rev-parse", "HEAD")

	// Writer A on branch a.
	gitCmd(t, root, "checkout", "-q", "-b", "a")
	appendLine(t, filepath.Join(root, dump), "-- writer A change\n")
	gitCmd(t, root, "commit", "-q", "-am", "writer A")

	// Writer B from the same base, a different edit.
	gitCmd(t, root, "checkout", "-q", "-b", "b", base)
	appendLine(t, filepath.Join(root, dump), "-- writer B change\n")
	gitCmd(t, root, "commit", "-q", "-am", "writer B")

	// Merge B into A: the two edits to the tail collide.
	gitCmd(t, root, "checkout", "-q", "a")
	merge := exec.Command("git", "-C", root, "merge", "b")
	out, err := merge.CombinedOutput()
	if err == nil {
		t.Fatalf("two-writer merge auto-resolved; a conflict was expected:\n%s", out)
	}
	conflicted, err := os.ReadFile(filepath.Join(root, dump))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(conflicted), "<<<<<<<") {
		t.Fatalf("no textual conflict markers in dump.sql after a two-writer merge:\n%s", conflicted)
	}
	// No merge driver is configured for dump.sql — the conflict is git's
	// default, not something a driver could have silently resolved.
	if attr := gitOut(t, root, "check-attr", "merge", dump); !strings.Contains(attr, "merge: unspecified") {
		t.Fatalf("a merge driver is configured for dump.sql (%q); none must ship", attr)
	}
}

// TestFileSyncExclusionDocumented is INV-363: the generated onboarding docs
// tell adopters git is the only sync channel and the per-machine
// db.sqlite*/sidecar must be excluded from any file-sync tool.
func TestFileSyncExclusionDocumented(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := writeScaffold(context.Background(), root, false); err != nil {
		t.Fatal(err)
	}
	prompt, err := os.ReadFile(filepath.Join(root, promptTarget))
	if err != nil {
		t.Fatal(err)
	}
	body := string(prompt)
	for _, want := range []string{"db.sqlite", "dump.hash", "exclude", "file-sync"} {
		if !strings.Contains(strings.ToLower(body), strings.ToLower(want)) {
			t.Errorf("PROMPT.md's sync guidance is missing %q", want)
		}
	}
}

func appendLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(line); err != nil {
		t.Fatal(err)
	}
}

func gitOut(t *testing.T, root string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out))
}

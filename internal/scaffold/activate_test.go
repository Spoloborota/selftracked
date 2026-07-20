package scaffold

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitInit makes root a git repo with hooks under .git/hooks, quiet.
func gitInit(t *testing.T, root string) {
	t.Helper()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
}

// TestActivationCleanRepoPrintsTakeover is INV-419: a repo with no
// incumbent hooks and no core.hooksPath gets the takeover command and the
// trust note, and NOT a chaining recipe.
func TestActivationCleanRepoPrintsTakeover(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	gitInit(t, root)
	out := hookActivation(context.Background(), root)
	if !strings.Contains(out, "git config core.hooksPath .selftracked/hooks") {
		t.Errorf("takeover command missing:\n%s", out)
	}
	if !strings.Contains(out, "execute on your machine") {
		t.Errorf("trust note missing:\n%s", out)
	}
	if strings.Contains(out, "|| exit $?") {
		t.Errorf("clean repo must not print a chaining recipe:\n%s", out)
	}
}

// TestActivationHooksPathSetSuppressesTakeover is INV-420/421/422/423/424:
// a set core.hooksPath means printing the takeover would disable the host
// gate, so init prints the chaining recipe — both hooks, exit-propagated
// pre-commit, top-placed post-commit, subprocess-not-source — and NOT the
// takeover command.
func TestActivationHooksPathSetSuppressesTakeover(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	gitInit(t, root)
	if out, err := exec.Command("git", "-C", root, "config", "core.hooksPath", ".husky").CombinedOutput(); err != nil {
		t.Fatalf("git config: %v: %s", err, out)
	}
	out := hookActivation(context.Background(), root)
	if strings.Contains(out, "git config core.hooksPath .selftracked/hooks") {
		t.Errorf("INV-420: takeover command must be suppressed when hooksPath is set:\n%s", out)
	}
	for _, want := range []string{
		".selftracked/hooks/pre-commit || exit $?", // INV-422 exit propagation
		"at the TOP of",                  // INV-423 top placement
		".selftracked/hooks/post-commit", // INV-421 both hooks
		"never `source`",                 // INV-424 subprocess mandate
		"execute on your machine",        // trust note
	} {
		if !strings.Contains(out, want) {
			t.Errorf("chaining recipe missing %q:\n%s", want, out)
		}
	}
}

// TestActivationIncumbentHookSuppressesTakeover is INV-420's second trigger:
// a live incumbent pre-commit (no hooksPath) also forces the chaining recipe.
// A .sample template must NOT count as incumbent.
func TestActivationIncumbentHookSuppressesTakeover(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	gitInit(t, root)
	hooks := filepath.Join(root, ".git", "hooks")

	// Git's shipped .sample is inert — still takeover.
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit.sample"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if out := hookActivation(context.Background(), root); !strings.Contains(out, "git config core.hooksPath") {
		t.Errorf("a .sample hook must not count as incumbent:\n%s", out)
	}

	// A live pre-commit forces chaining.
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := hookActivation(context.Background(), root)
	if strings.Contains(out, "git config core.hooksPath .selftracked/hooks") {
		t.Errorf("INV-420: live incumbent pre-commit must suppress takeover:\n%s", out)
	}
	if !strings.Contains(out, "|| exit $?") {
		t.Errorf("expected chaining recipe with an incumbent hook:\n%s", out)
	}
}

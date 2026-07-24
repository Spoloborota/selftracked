package verb

import (
	"errors"
	"testing"

	"github.com/Spoloborota/selftracked/internal/cli"
)

// TestPathsMoveValidatesTheNewRoot binds the two write paths into
// path_dictionary.root to one validation. `paths set` has always run the root
// through validateText; `paths move` writes the same column and did not, so a
// control byte could enter the dictionary — and from there the dump, a
// filesystem path, and a git argv — through the move door only.
func TestPathsMoveValidatesTheNewRoot(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	seedInstance(t, dir)

	env := &cli.Env{Stdout: &nopWriter{}, Stderr: &nopWriter{}}
	if err := pathsSet(env, []string{"research", "docs/research"}, false, ""); err != nil {
		t.Fatal(err)
	}
	err := pathsMove(env, []string{"research", "docs/bad\x1besc"}, false)
	if err == nil {
		t.Fatal("paths move accepted a control byte in the new root; want a refusal")
	}
	var coded *cli.CodedError
	if !errors.As(err, &coded) || coded.Code != "control-chars" {
		t.Fatalf("want a control-chars refusal, got %v", err)
	}
}

type nopWriter struct{}

func (*nopWriter) Write(p []byte) (int, error) { return len(p), nil }

package ref_test

import (
	"errors"
	"testing"

	"github.com/Spoloborota/selftracked/internal/ref"
)

func TestGrammar(t *testing.T) {
	t.Parallel()

	good := []struct {
		in   string
		want ref.Ref
	}{
		{"#14", ref.Ref{Kind: ref.Task, Task: 14}},
		{"14", ref.Ref{Kind: ref.Task, Task: 14}},
		{"epic:token-rotation", ref.Ref{Kind: ref.Epic, Epic: "token-rotation"}},
		{"epic:token-rotation/S2", ref.Ref{Kind: ref.Story, Epic: "token-rotation", Story: "S2"}},
		{"research:2026-03-05-options.md", ref.Ref{Kind: ref.Artifact, Class: "research", Rel: "2026-03-05-options.md"}},
		{"research@backend:2026-03-01-cache.md", ref.Ref{
			Kind: ref.Artifact, Class: "research", Scope: "backend", Rel: "2026-03-01-cache.md",
		}},
	}
	for _, tc := range good {
		got, err := ref.Parse(tc.in)
		if err != nil {
			t.Errorf("Parse(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Parse(%q) = %+v; want %+v", tc.in, got, tc.want)
		}
	}

	bad := []string{
		"",               // nothing
		"#",              // prefix without digits
		"#x",             // prefix with non-digits
		"0",              // ids start at 1
		"-3",             // a leading dash is never a reference
		"epic:",          // empty slug
		"epic:a/",        // empty story id
		"epic:a/2",       // story id without the S
		"epic:a/Sx",      // story id with non-digits
		"epic@x:a",       // epic takes no scope
		":path",          // empty class
		"class:",         // empty relpath
		"@scope:x",       // empty class, scoped
		"class@:x",       // empty scope
		"archive",        // §6.2 invariant: bare keywords never parse
		"unarchive",      // same
		"token-rotation", /* a slug without its epic: prefix */
	}
	for _, in := range bad {
		if got, err := ref.Parse(in); err == nil {
			t.Errorf("Parse(%q) = %+v; want a refusal", in, got)
		} else if !errors.Is(err, ref.ErrInvalid) {
			t.Errorf("Parse(%q) error %v is not ErrInvalid", in, err)
		}
	}
}

func TestTaskRefRendering(t *testing.T) {
	t.Parallel()

	if got := ref.TaskRef(14); got != "#14" {
		t.Fatalf("TaskRef(14) = %q; want %q", got, "#14")
	}
}

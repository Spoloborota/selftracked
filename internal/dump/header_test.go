package dump_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Spoloborota/selftracked/internal/dump"
)

func TestHeaderVersionIsStrict(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		line string
		want int
		ok   bool
	}{
		{"real header", "-- selftracked dump schema_version=3 tasks=0 artifacts=0", 3, true},
		{"no prefix", "schema_version=3 tasks=0", 0, false},
		{"zero version", "-- selftracked dump schema_version=0 tasks=0 artifacts=0", 0, false},
		{"negative", "-- selftracked dump schema_version=-1 tasks=0 artifacts=0", 0, false},
		{"non-integer", "-- selftracked dump schema_version=x tasks=0 artifacts=0", 0, false},
		{"missing field", "-- selftracked dump tasks=0 artifacts=0", 0, false},
		{"empty", "", 0, false},
	}
	for _, c := range cases {
		v, ok := dump.HeaderVersion(c.line)
		if v != c.want || ok != c.ok {
			t.Errorf("%s: HeaderVersion(%q) = (%d,%v), want (%d,%v)", c.name, c.line, v, ok, c.want, c.ok)
		}
	}
}

func TestTrackedHeaderVersionBoundsTheRead(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// A missing dump is not a header.
	if v, ok := dump.TrackedHeaderVersion(dir); ok || v != 0 {
		t.Fatalf("missing dump: (%d,%v), want (0,false)", v, ok)
	}

	// A real header parses.
	header := "-- selftracked dump schema_version=7 tasks=0 artifacts=0\nrest\n"
	if err := os.WriteFile(filepath.Join(dir, "dump.sql"), []byte(header), 0o600); err != nil {
		t.Fatal(err)
	}
	if v, ok := dump.TrackedHeaderVersion(dir); !ok || v != 7 {
		t.Fatalf("real header: (%d,%v), want (7,true)", v, ok)
	}

	// A first line with no newline inside the 4 KiB bound is not a
	// selftracked header — reported explicitly, not via an ignored
	// scanner error (an S11 close-review finding).
	huge := "-- selftracked dump schema_version=99 " + strings.Repeat("x", 8192)
	if err := os.WriteFile(filepath.Join(dir, "dump.sql"), []byte(huge), 0o600); err != nil {
		t.Fatal(err)
	}
	if v, ok := dump.TrackedHeaderVersion(dir); ok || v != 0 {
		t.Fatalf("oversized first line: (%d,%v), want (0,false)", v, ok)
	}
}

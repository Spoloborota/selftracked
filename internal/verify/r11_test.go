//nolint:testpackage // white-box: the R11 variant table asserts the unexported hookReferences matcher directly
package verify

import "testing"

// TestHookReferencesVariantTable is R11's variant table (INV-308, INV-496):
// the textual matcher must accept the canonical chaining line and its
// common hand-edited spellings, and must NOT match a reference that lives
// only inside a comment. Both directions are documented parent-project
// incident classes — a matcher blind to a spelling fails open silently in
// exactly the state it exists to warn about, and an over-eager one
// false-positives on a mere mention.
func TestHookReferencesVariantTable(t *testing.T) {
	t.Parallel()
	const name = "pre-commit"
	cases := []struct {
		label string
		line  string
		want  bool
	}{
		{"canonical", `exec .selftracked/hooks/pre-commit "$@"`, true},
		{"sh-prefixed", `sh .selftracked/hooks/pre-commit`, true},
		{"quoted-path", `"$(dirname "$0")/.selftracked/hooks/pre-commit"`, true},
		{"variable-prefixed", `HOOK=.selftracked/hooks/pre-commit; exec "$HOOK"`, true},
		{"trailing-inline-comment", `echo hi   # runs .selftracked/hooks/pre-commit`, false},
		{"whole-line-comment", `# .selftracked/hooks/pre-commit`, false},
		{"unrelated-hook", `exec ./other-tool --check`, false},
		{"empty", ``, false},
	}
	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			t.Parallel()
			if got := hookReferences([]byte(c.line), name); got != c.want {
				t.Fatalf("hookReferences(%q) = %v, want %v", c.line, got, c.want)
			}
		})
	}
}

// TestHookReferencesMultiLine confirms a reference on any live line counts,
// and that a file whose only reference is commented out does not.
func TestHookReferencesMultiLine(t *testing.T) {
	t.Parallel()
	chained := "#!/bin/sh\nset -e\nexec .selftracked/hooks/post-commit \"$@\"\n"
	if !hookReferences([]byte(chained), "post-commit") {
		t.Fatal("a live reference on a later line must match")
	}
	commentedOnly := "#!/bin/sh\n# formerly: .selftracked/hooks/post-commit\nexec ./legacy-hook\n"
	if hookReferences([]byte(commentedOnly), "post-commit") {
		t.Fatal("a reference living only inside a comment must NOT match (false-silence direction)")
	}
}

// TestHookReferencesStatedResidual pins the accepted, unfixtured-in-spirit
// residual (INV-309): a quoted '#' inside a live string BEFORE the real
// reference defeats the naive after-'#' strip. This is asserted so the
// ceiling is visible and a future "fix" that changes it is a conscious
// decision, not an accident.
func TestHookReferencesStatedResidual(t *testing.T) {
	t.Parallel()
	line := `echo "#x"; .selftracked/hooks/pre-commit`
	if hookReferences([]byte(line), "pre-commit") {
		t.Fatal("stated residual changed: the naive strip now sees past a quoted '#' — update INV-309 deliberately")
	}
}

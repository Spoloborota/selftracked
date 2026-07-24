package verify_test

import (
	"strings"
	"testing"
)

// TestR5RefusesALeadingDashCitation binds the argv guard: a commits cell is
// dump-supplied, and git reads a leading-dash argument as an option rather
// than a revision, so such a token must be reported without ever reaching
// `git cat-file`. The distinct message ("is not a revision", not "does not
// resolve") is what proves the guard fired instead of git.
func TestR5RefusesALeadingDashCitation(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	git(t, h.root, "commit", "--allow-empty", "-m", "seed")

	h.exec(`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e', 'g', 'ACTIVE', '2020-01-01T00:00:00Z')`)
	h.exec(`INSERT INTO stories (epic, id, title, status) VALUES ('e', 'S1', 't', 'DONE')`)
	h.exec(`INSERT INTO worklog (epic, seq, story, date, state, commits) VALUES ('e', 1, 'S1', 'd', 'DONE', ?)`,
		"--output=/dev/null")
	h.regen()

	rep := h.verify(false)
	mustRed(t, rep, "R5")
	var found bool
	for _, v := range rep.Red {
		if v.Rule == "R5" && strings.Contains(v.Message, "is not a revision") {
			found = true
		}
	}
	if !found {
		t.Fatalf("a leading-dash citation was not refused by the argv guard: %+v", rep.Red)
	}
}

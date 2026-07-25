package verb

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Spoloborota/selftracked/internal/cli"
	"github.com/Spoloborota/selftracked/internal/schema"
)

// The terminal-epic guard is the WHOLE boundary for the verb surface —
// `internal/schema/ddl.sql`'s stories_transition trigger validates only a
// story's own old→new matrix and knows nothing of the parent epic — and
// since `r16-reports-only-what-a-verb-can-clear` it carries a carve-out.
// The CLI scripts drive it end to end; these tests pin it at the package
// that owns it, where the exemption's shape (an argument that defaults to
// guarding, a move-descriptor field whose zero value is "guarded") is
// visible and a mis-scoped edit is a one-line diff.

// guardDB is a fresh schema holding one epic in the requested status, with
// or without the `epic close` events row that makes the close OURS.
func guardDB(t *testing.T, status string, closedByVerb bool) *sql.DB {
	t.Helper()
	db, err := schema.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := schema.Create(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	// close_sweep is CHECK-tied to CLOSED exactly; status_note is
	// CHECK-required for PAUSED and DISSOLVED.
	sweep := ""
	if status == epicClosed {
		sweep = "2026-01-05"
	}
	if _, err := db.ExecContext(t.Context(),
		`INSERT INTO epics (slug, goal, status, status_note, close_sweep, created_at)
		 VALUES ('e', 'g', ?, 'n', ?, 'd')`, status, sweep); err != nil {
		t.Fatalf("seed epic %s: %v", status, err)
	}
	if closedByVerb {
		if _, err := db.ExecContext(t.Context(),
			`INSERT INTO events (at, entity, event, detail)
			 VALUES ('d', 'epic:e', 'epic', 'close: 2026-01-05')`); err != nil {
			t.Fatalf("seed close event: %v", err)
		}
	}
	return db
}

// inTx runs fn against a transaction and rolls back: the subject is the
// guard's verdict, not a commit.
func inTx(t *testing.T, db *sql.DB, fn func(tx *sql.Tx) error) error {
	t.Helper()
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	return fn(tx)
}

// refusalCode is the §6.1 code of a domain refusal, or "" for nil, or the
// literal "not-a-refusal" for an error that is not one — so a test can tell
// "wrong refusal" from "infrastructure blew up".
func refusalCode(err error) string {
	if err == nil {
		return ""
	}
	var coded *cli.CodedError
	if errors.As(err, &coded) {
		return coded.Code
	}
	return "not-a-refusal"
}

// TestRefuseTerminalEpicExcept pins the carve-out's exact boundary. The
// rows that matter most are the two where status and PROVENANCE disagree:
// a CLOSED epic that arrived through `import` passed no close gate, so R16
// makes no claim about it and the repair for a claim nobody made must not
// be granted.
func TestRefuseTerminalEpicExcept(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		status       string
		closedByVerb bool
		allowClosed  bool
		want         string // "" = admitted
	}{
		"verb-closed epic admits the exemption": {
			status: epicClosed, closedByVerb: true, allowClosed: true, want: "",
		},
		"verb-closed epic still refuses WITHOUT the exemption": {
			status: epicClosed, closedByVerb: true, allowClosed: false, want: "terminal",
		},
		"import-arrived CLOSED epic refuses even WITH the exemption": {
			status: epicClosed, closedByVerb: false, allowClosed: true, want: "terminal",
		},
		"DISSOLVED epic refuses even WITH the exemption": {
			status: epicDissolved, closedByVerb: false, allowClosed: true, want: "terminal",
		},
		// A DISSOLVED epic cannot have been closed by verb, but an events
		// trail is forgeable (that is R12's whole subject), so the flag must
		// not become reachable by planting one.
		"DISSOLVED epic refuses even with a close event present": {
			status: epicDissolved, closedByVerb: true, allowClosed: true, want: "terminal",
		},
		"an ACTIVE epic is never the guard's business": {
			status: epicActive, closedByVerb: false, allowClosed: false, want: "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db := guardDB(t, tc.status, tc.closedByVerb)
			err := inTx(t, db, func(tx *sql.Tx) error {
				return refuseTerminalEpicExcept(context.Background(), tx, "e", "a story transition", tc.allowClosed)
			})
			if got := refusalCode(err); got != tc.want {
				t.Fatalf("refuseTerminalEpicExcept = %q (%v), want %q", got, err, tc.want)
			}
		})
	}
}

// TestRefuseTerminalEpicIsAlwaysStrict: the exported-to-the-package entry
// point every other call site uses must pass false, whatever the epic's
// provenance. If it ever started forwarding a caller's opinion, `story
// add`, `criteria met`, `edit --epic` and the rest would inherit the
// carve-out silently.
func TestRefuseTerminalEpicIsAlwaysStrict(t *testing.T) {
	t.Parallel()
	db := guardDB(t, epicClosed, true)
	err := inTx(t, db, func(tx *sql.Tx) error {
		return refuseTerminalEpic(context.Background(), tx, "e", "story add")
	})
	if got := refusalCode(err); got != "terminal" {
		t.Fatalf("refuseTerminalEpic on a verb-closed epic = %q (%v), want terminal", got, err)
	}
}

// TestMoveStoryGuardsByDefault is the structural half of the carve-out:
// the exemption travels in the move descriptor, and its ZERO VALUE must
// guard. A transition added to stories.go tomorrow with no thought about
// terminal epics gets the strict behaviour — this is that promise under
// test, driven through moveStory rather than the guard directly so the
// wiring is what is proved.
func TestMoveStoryGuardsByDefault(t *testing.T) {
	t.Parallel()
	seed := func(t *testing.T) *sql.DB {
		t.Helper()
		db := guardDB(t, epicClosed, true)
		if _, err := db.ExecContext(t.Context(),
			`INSERT INTO stories (epic, id, title, status) VALUES ('e','S1','t','PLANNED')`); err != nil {
			t.Fatal(err)
		}
		return db
	}
	// PLANNED is a legal source for BOTH moves below, so neither refusal
	// can come from the source-status check — only from the guard.
	t.Run("a move that does not ask for the exemption is refused", func(t *testing.T) {
		t.Parallel()
		db := seed(t)
		err := inTx(t, db, func(tx *sql.Tx) error {
			_, err := moveStory(context.Background(), tx, "e", "S1", move{
				sources: []string{storyPlanned}, target: storyReady, detail: subReady,
			})
			return err
		})
		if got := refusalCode(err); got != "terminal" {
			t.Fatalf("an unexempted move on a verb-closed epic = %q (%v), want terminal", got, err)
		}
	})
	t.Run("dissolve's declared exemption is honoured", func(t *testing.T) {
		t.Parallel()
		db := seed(t)
		err := inTx(t, db, func(tx *sql.Tx) error {
			_, err := moveStory(context.Background(), tx, "e", "S1", move{
				sources: []string{storyPlanned}, target: storyDissolved,
				worklog: storyDissolved, note: "repair", detail: "dissolve: repair",

				closedEpicRepair: true,
			})
			return err
		})
		if err != nil {
			t.Fatalf("the declared repair must be admitted; got %v", err)
		}
	})
}

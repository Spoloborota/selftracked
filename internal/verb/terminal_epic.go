package verb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Spoloborota/selftracked/internal/rules"
)

// refuseTerminalEpic refuses a write that would re-open one of §6.4's
// close conditions on an epic that is already CLOSED or DISSOLVED
// (amendment `terminal-epics-refuse-reopening-writes`). The close gate is
// evaluated at the transition and there is deliberately no epic reopen (the
// "Schema gates (triggers)" subsection of §5 — NOT §5.4, which is the
// `epic_criteria` block; the wrong pointer stood here and in tasks #57/#60
// until this comment was rewritten), so without this guard a closed epic
// can permanently display the very states its gate exists to forbid — a
// PLANNED story, a workable task homed to it, an unmet criterion.
//
// There is NO schema-level backstop behind it: `internal/schema/ddl.sql`'s
// stories_transition trigger validates only a story's own old→new matrix
// and knows nothing of the parent epic. This function is therefore the
// whole boundary FOR THE VERB SURFACE — and only for it. `import` writes
// story rows straight into any epic, terminal ones included
// (`import_insert.go`'s insertExplicitStories and materializeStories issue
// bare INSERTs and never consult this guard), which is §1.1's deliberate
// open-INSERT seam, the state task #53 records, and the reason R16 exists
// as a detection control rather than a second guard. Stated precisely
// because "the whole boundary" would tell the next reader to stop looking
// at exactly the door the state actually comes through.
//
// Deliberately NOT routed through this guard: `worklog add --story V-N`
// (the sanctioned post-close surface, which already REQUIRES CLOSED),
// `worklog add --corrects N` (§5.7's append-only correction, which the
// schema allows against terminal stories by design), `edit --detach`
// (it removes a violation rather than creating one — and it is the
// repair path this refusal names for `reopen`), artifact links on an
// `epic:` target (attaching a retrospective to a closed epic is ordinary
// documentation and breaks no close condition), and — narrowest of the
// four — `story dissolve` of a non-terminal story on an epic THIS TRACKER
// closed by verb
// (amendment `r16-reports-only-what-a-verb-can-clear`): DISSOLVED is the
// one story target that SATISFIES close condition (1), the DISSOLVED
// worklog row it appends satisfies (2), and conditions (4)–(6) do not
// involve story status, so like `edit --detach` it removes a violation
// rather than creating one, and it is the repair R16's story finding
// names. That last one is expressed as refuseTerminalEpicExcept's
// allowClosed argument, false everywhere else including here.
//
// A missing epic is reported as not-found rather than silently passing:
// every caller here has already resolved a slug the user typed.
func refuseTerminalEpic(ctx context.Context, tx *sql.Tx, slug, verb string) error {
	return refuseTerminalEpicExcept(ctx, tx, slug, verb, false)
}

// refuseTerminalEpicExcept is refuseTerminalEpic with its single carve-out
// made explicit. allowClosed admits the write on an epic that is CLOSED
// **and was closed by this tracker's own `epic close`** — the provenance
// predicate rules.ClosedByVerbSQL states and R16 filters all three of its
// clauses on. Status alone is not enough and the difference is not
// academic: `import` writes whatever status its corpus declares, so an epic
// can arrive CLOSED having passed no gate. R16 stays silent about such an
// epic, so admitting a repair there would grant a write capability for a
// state class the motivating detector never reports — the widening
// `r16-reports-only-what-a-verb-can-clear` was narrowed away from. A
// DISSOLVED epic is refused for the same reason, whatever the flag says.
//
// The argument is REQUIRED, and false is the guarding value: a call site
// that forgets to think about it gets the strict behaviour, and the
// exemption is visible at the one place that asks for it.
func refuseTerminalEpicExcept(ctx context.Context, tx *sql.Tx, slug, verb string, allowClosed bool) error {
	terminal, status, err := epicIsTerminal(ctx, tx, slug)
	if err != nil {
		return err
	}
	if !terminal {
		return nil
	}
	if allowClosed && status == epicClosed {
		byVerb, err := rules.EpicClosedByVerb(ctx, tx, slug)
		if err != nil {
			return fmt.Errorf("terminal-epic guard: %w", err)
		}
		if byVerb {
			return nil
		}
	}
	return refuse("terminal",
		"epic:%s is %s; post-close work is a V-row (worklog add --story V-N) "+
			"or a correction row (worklog add --corrects N) — %s cannot reopen a closed record",
		slug, status, verb)
}

// epicIsTerminal reports whether the epic is in a terminal state, along
// with the state itself so callers can name it.
func epicIsTerminal(ctx context.Context, tx *sql.Tx, slug string) (bool, string, error) {
	var status string
	err := tx.QueryRowContext(ctx, `SELECT status FROM epics WHERE slug = ?`, slug).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", refuse("not-found", "no epic %q", slug)
	}
	if err != nil {
		return false, "", fmt.Errorf("epic status: %w", err)
	}
	return status == epicClosed || status == epicDissolved, status, nil
}

// refuseTerminalEpicOfTask guards a write against the epic a TASK is
// homed to — `reopen`'s case, where the task itself is the argument and
// its epic is the record being reopened. An unhomed task passes.
func refuseTerminalEpicOfTask(ctx context.Context, tx *sql.Tx, id int64, verb string) error {
	var slug sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT epic FROM tasks WHERE id = ?`, id).Scan(&slug)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // the caller's own not-found refusal is the better message
	}
	if err != nil {
		return fmt.Errorf("task epic: %w", err)
	}
	if !slug.Valid || slug.String == "" {
		return nil
	}
	terminal, status, err := epicIsTerminal(ctx, tx, slug.String)
	if err != nil {
		return err
	}
	if terminal {
		return refuse("terminal",
			"#%d is homed to epic:%s, which is %s; detach it first (selftracked edit %d --detach) "+
				"or leave it terminal — %s would put workable work back under a closed record",
			id, slug.String, status, id, verb)
	}
	return nil
}

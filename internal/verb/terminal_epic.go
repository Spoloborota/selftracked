package verb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// refuseTerminalEpic refuses a write that would re-open one of §6.4's
// close conditions on an epic that is already CLOSED or DISSOLVED
// (amendment `terminal-epics-refuse-reopening-writes`). The close gate is
// evaluated at the transition and there is deliberately no epic reopen
// (§5.4), so without this guard a closed epic can permanently display the
// very states its gate exists to forbid — a PLANNED story, a workable
// task homed to it, an unmet criterion.
//
// Deliberately NOT routed through this guard: `worklog add --story V-N`
// (the sanctioned post-close surface, which already REQUIRES CLOSED),
// `worklog add --corrects N` (§5.7's append-only correction, which the
// schema allows against terminal stories by design), `edit --detach`
// (it removes a violation rather than creating one — and it is the
// repair path this refusal names for `reopen`), and artifact links on an
// `epic:` target (attaching a retrospective to a closed epic is ordinary
// documentation and breaks no close condition).
//
// A missing epic is reported as not-found rather than silently passing:
// every caller here has already resolved a slug the user typed.
func refuseTerminalEpic(ctx context.Context, tx *sql.Tx, slug, verb string) error {
	terminal, status, err := epicIsTerminal(ctx, tx, slug)
	if err != nil {
		return err
	}
	if terminal {
		return refuse("terminal",
			"epic:%s is %s; post-close work is a V-row (worklog add --story V-N) "+
				"or a correction row (worklog add --corrects N) — %s cannot reopen a closed record",
			slug, status, verb)
	}
	return nil
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

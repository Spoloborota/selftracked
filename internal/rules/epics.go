package rules

import (
	"context"
	"fmt"
)

// ClosedByVerbSQL is THE definition of "this tracker closed this epic by
// verb": an `epic` events row carrying the close stamp. It is returned as
// an SQL `EXISTS (…)` fragment rather than a finished query because its two
// consumers need it in different positions — R16 correlates it against the
// outer row (`e.slug`), while the terminal-epic guard asks it of one bound
// slug — and a second hand-written copy of the predicate is exactly the
// drift this package exists to prevent.
//
// The distinction it draws is PROVENANCE, not status. An epic can be
// `CLOSED` without ever having passed a close gate: `import` writes whatever
// status its corpus declares (§1.1's open INSERT path, task #53). Such an
// epic made no claim this tracker adjudicated, so R16 has nothing to
// re-check there and the §6.4 `story dissolve` carve-out has nothing to
// repair. Both surfaces must agree about that or the carve-out grants a
// write capability for a state class the detector never reports — the
// widening an earlier draft of `r16-reports-only-what-a-verb-can-clear` was
// rejected for.
//
// SECURITY CONTRACT: slugExpr is interpolated into SQL TEXT. It is an SQL
// *expression* supplied by the calling code — `e.slug`, `?1` — and must
// never be a slug, a user string, or anything derived from input. Every
// call site in this repository passes a source literal; a caller that needs
// to name a value passes a placeholder and binds it.
func ClosedByVerbSQL(slugExpr string) string {
	return `EXISTS (SELECT 1 FROM events ev
		WHERE ev.entity = 'epic:' || ` + slugExpr + ` AND ev.event = 'epic'
		  AND ev.detail LIKE 'close:%')`
}

// EpicClosedByVerb answers ClosedByVerbSQL for one epic, over the same
// fragment the rules use. A missing epic answers false: absence of a close
// event is exactly what the predicate reports, and the caller's own
// not-found refusal is the better message for a slug that does not exist.
func EpicClosedByVerb(ctx context.Context, q Querier, slug string) (bool, error) {
	var closed int
	if err := q.QueryRowContext(ctx, `SELECT `+ClosedByVerbSQL(`?1`), slug).Scan(&closed); err != nil {
		return false, fmt.Errorf("epic %q closed by verb: %w", slug, err)
	}
	return closed == 1, nil
}

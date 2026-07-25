package rules

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Querier is the minimal read surface both *sql.DB and *sql.Tx satisfy.
// The lookup below is asked from both: `worklog add` composes its refusal
// INSIDE the write transaction, while a read-only caller holds a plain
// handle. One interface keeps that from becoming two copies of the query.
type Querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// The six story statuses of §5.7's CHECK constraint. Named here, rather
// than only in `internal/verb`, because surfaces outside the verbs reason
// about the terminal / non-terminal split and hand-written subsets had
// already diverged; `internal/verb`'s own constants alias these. Today the
// one consumer is `worklog add`'s refusal clause — R10 and `prime` are to
// read the same predicate when the stories that amend them land, and this
// package is where they will find it rather than write a third copy.
const (
	StoryPlanned    = "PLANNED"
	StoryReady      = "READY"
	StoryBlocked    = "BLOCKED"
	StoryInProgress = "IN-PROGRESS"
	StoryDone       = "DONE"
	StoryDissolved  = "DISSOLVED"
)

// Story is one story's identity and current status — the two columns the
// callers of this package need, and none of them needs more.
type Story struct {
	ID     string
	Status string
}

// NonTerminalStoryStatuses is THE definition of "a story that can still
// receive work": every status that is not DONE and not DISSOLVED. A
// PLANNED story is a home (`story ready` → `story start` reaches it with
// no scope change) and a BLOCKED one is the sanctioned PO-absent state, so
// both count — the set is deliberately wider than the READY/IN-PROGRESS
// pair R10's idle clause uses.
//
// The ORDER IS MEANINGFUL and is not the schema's declaration order: it
// runs from the status nearest to receiving work to the furthest, and
// StoryToOffer sorts by it. A caller wanting only the membership test may
// ignore the order; a caller wanting the priority must not re-derive it.
//
// A fresh slice is returned on every call: a package-level slice is a
// mutable global, and a caller that sorted or truncated it in place would
// silently redefine the predicate for everyone else.
func NonTerminalStoryStatuses() []string {
	return []string{StoryInProgress, StoryReady, StoryBlocked, StoryPlanned}
}

// StoryToOffer returns the ONE story of an epic that a message should
// offer as the home for a new episode, and whether any exists at all. The
// false return is the dead-zone answer — no story of this epic is in a
// non-terminal status, so work can be recorded nowhere under it.
//
// Selection is by status priority (NonTerminalStoryStatuses' order), not
// by id, and that is a correctness property rather than a preference. The
// WIP index admits one IN-PROGRESS story per epic, so offering a PLANNED
// story while a different story holds the slot would advise a `story
// start` guaranteed to refuse — after `story ready` had already changed a
// story's status for nothing. Sorting IN-PROGRESS first means every other
// branch is reached only when the slot is free, which is what lets those
// branches promise `story start` will work.
//
// The id breaks ties within one status, numeric part first so S2 precedes
// S10 among equals. §5.7's CHECK is only `GLOB 'S[0-9]*'` — it pins one
// digit after the S and still admits `S1a`, `S02` or `S1.5`, which can
// share a numeric key — so the raw id is compared after it. The result is
// therefore total and deterministic for every id the schema admits,
// without claiming a numeric order the schema does not guarantee.
func StoryToOffer(ctx context.Context, q Querier, epic string) (Story, bool, error) {
	statuses := NonTerminalStoryStatuses()
	// Both the IN list and the priority CASE are generated from the one Go
	// slice above and bound as arguments: the status vocabulary is never
	// spelled into SQL text, which is the drift this package exists to
	// prevent. Placeholders are ordinal (?1 is the epic), so each status
	// binds once and is referenced twice.
	var in, order strings.Builder
	args := make([]any, 0, len(statuses)+1)
	args = append(args, epic)
	// ?1 is the epic, so the statuses start one past it.
	const firstStatusParam = 2
	for i, s := range statuses {
		p := "?" + strconv.Itoa(i+firstStatusParam)
		if i > 0 {
			in.WriteString(",")
		}
		in.WriteString(p)
		_, _ = fmt.Fprintf(&order, " WHEN %s THEN %d", p, i)
		args = append(args, s)
	}
	query := `SELECT id, status FROM stories WHERE epic = ?1 AND status IN (` + in.String() +
		`) ORDER BY CASE status` + order.String() + ` END, CAST(substr(id, 2) AS INTEGER), id LIMIT 1`
	var s Story
	err := q.QueryRowContext(ctx, query, args...).Scan(&s.ID, &s.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return Story{}, false, nil
	}
	if err != nil {
		return Story{}, false, fmt.Errorf("story to offer for epic %q: %w", epic, err)
	}
	return s, true, nil
}

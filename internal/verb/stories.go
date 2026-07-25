package verb

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"slices"
	"strings"

	"github.com/Spoloborota/selftracked/internal/cli"
	"github.com/Spoloborota/selftracked/internal/rules"
)

const (
	// storyArity: SLUG SID.
	storyArity = 2

	// The story-status vocabulary has ONE definition, in internal/rules,
	// because surfaces outside the verbs (the worklog refusal clause today;
	// R10 and prime as their stories land) must agree with the verbs about
	// which statuses are terminal. These are aliases, not a second copy —
	// the call sites keep the short unexported names.
	storyPlanned    = rules.StoryPlanned
	storyReady      = rules.StoryReady
	storyBlocked    = rules.StoryBlocked
	storyInProgress = rules.StoryInProgress
	storyDone       = rules.StoryDone
	storyDissolved  = rules.StoryDissolved

	evStory   = "story"
	evWorklog = "worklog"
)

// StoryVerbs returns the S6 `story` and `worklog` catalog entries.
func StoryVerbs() []cli.Verb {
	var title, reason, resolution, commits, gate, review, why string
	return []cli.Verb{{Name: "story", Subs: []cli.Sub{
		{
			Name: subAdd, Arity: 1, Usage: "story add SLUG --title T [--json]",
			Flags: func(fs *flag.FlagSet) { fs.StringVar(&title, "title", "", "the story title (required)") },
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return storyAdd(e, pos[0], title)
			},
		},
		{
			Name: subReady, Arity: storyArity, Usage: "story ready SLUG SID [--json]",
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return storyReadyVerb(e, pos[0], pos[1])
			},
		},
		{
			Name: "start", Arity: storyArity, Usage: "story start SLUG SID [--json]",
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return storyStart(e, pos[0], pos[1])
			},
		},
		{
			Name: "block", Arity: storyArity, Usage: "story block SLUG SID --reason TEXT [--json]",
			Flags: func(fs *flag.FlagSet) { fs.StringVar(&reason, "reason", "", "what blocks it (required)") },
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return storyBlock(e, pos[0], pos[1], reason)
			},
		},
		{
			Name: "unblock", Arity: storyArity, Usage: "story unblock SLUG SID --resolution TEXT [--json]",
			Flags: func(fs *flag.FlagSet) {
				fs.StringVar(&resolution, "resolution", "", "what cleared the block (required)")
			},
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return storyUnblock(e, pos[0], pos[1], resolution)
			},
		},
		{
			Name: "done", Arity: storyArity,
			Usage: "story done SLUG SID --commits RANGE --gate G [--review R] [--json]",
			Flags: func(fs *flag.FlagSet) {
				fs.StringVar(&commits, "commits", "", "the proving commit range (required)")
				fs.StringVar(&gate, "gate", "", "the gate that went green (required)")
				fs.StringVar(&review, "review", "", "review reference")
			},
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return storyDoneVerb(e, pos[0], pos[1], commits, gate, review)
			},
		},
		{
			Name: "dissolve", Arity: storyArity, Usage: "story dissolve SLUG SID --why TEXT [--json]",
			Flags: func(fs *flag.FlagSet) { fs.StringVar(&why, "why", "", "the dissolve reason (required)") },
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return storyDissolve(e, pos[0], pos[1], why)
			},
		},
	}}, worklogVerb()}
}

// appendWorklog writes one episode row at the epic's next contiguous seq.
func appendWorklog(ctx context.Context, tx *sql.Tx,
	slug, story, state, commits, gate, review, note string,
) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, note)
		VALUES (?1, COALESCE((SELECT MAX(seq) FROM worklog WHERE epic = ?1), 0) + 1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)`,
		slug, story, now(), state, commits, gate, review, note); err != nil {
		return err //nolint:wrapcheck // the contiguity trigger and CHECKs speak through the mapper
	}
	return nil
}

func storyEntity(slug, sid string) string { return epicPrefix + slug + "/" + sid }

func storyAdd(e *cli.Env, slug, title string) error {
	if title == "" {
		return refuse("usage", "story add requires --title")
	}
	if err := validateText("--title", title); err != nil {
		return err
	}
	var sid string
	err := Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		ctx := context.Background()
		if err := refuseTerminalEpic(ctx, tx, slug, "story add"); err != nil {
			return nil, err
		}
		var next int64
		err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(MAX(CAST(substr(id, 2) AS INTEGER)), 0) + 1
			FROM stories WHERE epic = ?`, slug).Scan(&next)
		if err != nil {
			return nil, fmt.Errorf("story add: %w", err)
		}
		sid = fmt.Sprintf("S%d", next)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO stories (epic, id, title) VALUES (?, ?, ?)`, slug, sid, title); err != nil {
			return nil, err //nolint:wrapcheck // FK and CHECKs speak through the mapper
		}
		return []Event{{Entity: storyEntity(slug, sid), Event: evStory, Detail: "add: " + title}}, nil
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s/%s [PLANNED]\n", slug, sid)
	return nil
}

// move is one story transition: its legal source statuses, the target,
// the blocked-field value, the episode row it appends (empty = none), its
// events detail, and whether it is a close-condition REPAIR.
type move struct {
	sources []string
	target  string
	blocked string
	worklog string
	note    string
	detail  string
	commits string
	gate    string
	review  string

	// closedEpicRepair exempts this transition from the terminal-epic guard
	// on an epic THIS TRACKER closed by verb — and only there; a DISSOLVED
	// epic, and a CLOSED one that arrived through `import` without ever
	// passing a close gate, are both refused regardless (amendment
	// `r16-reports-only-what-a-verb-can-clear`, §6.4). Set it ONLY for a
	// transition whose target satisfies a close condition instead of
	// breaking one; `dissolve` is the sole such transition, and the field's
	// zero value means a move added to this table tomorrow is guarded
	// without anyone remembering to guard it.
	closedEpicRepair bool
}

// moveStory applies a transition INSIDE tx after guarding the source
// status: the §5.7 trigger treats a same-status write as a legal no-op,
// so a verb that only trusted the trigger could re-affirm DONE/BLOCKED
// forever and append a duplicate episode each time (INV-119). The guard
// refuses any status that is not a declared source for this move.
func moveStory(ctx context.Context, tx *sql.Tx, slug, sid string, m move) ([]Event, error) {
	// One guard covers every story transition routing here — ready, block,
	// unblock, done, dissolve (start carries its own call) — because each of
	// them would otherwise append episode history to a closed epic outside
	// the V-row surface §6.4 sanctioned. `dissolve` is the ONE exception,
	// and it declares itself through move.closedEpicRepair rather than by
	// skipping the call: the strict behaviour is the default, so a
	// transition added to the table without a thought about the terminal
	// case still refuses.
	if err := refuseTerminalEpicExcept(ctx, tx, slug, "a story transition", m.closedEpicRepair); err != nil {
		return nil, err
	}
	var cur string
	err := tx.QueryRowContext(ctx,
		`SELECT status FROM stories WHERE epic = ? AND id = ?`, slug, sid).Scan(&cur)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, refuse("not-found", "no story %s in epic %q", sid, slug)
	}
	if err != nil {
		return nil, fmt.Errorf("story transition: %w", err)
	}
	if !slices.Contains(m.sources, cur) {
		return nil, refuse("wrong-state", "story %s is %s; this transition applies from %s",
			sid, cur, strings.Join(m.sources, "/"))
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE stories SET status = ?, blocked = ? WHERE epic = ? AND id = ?`,
		m.target, m.blocked, slug, sid); err != nil {
		return nil, err //nolint:wrapcheck // the matrix trigger speaks through the mapper
	}
	if m.worklog != "" {
		if err := appendWorklog(ctx, tx, slug, sid, m.worklog, m.commits, m.gate, m.review, m.note); err != nil {
			return nil, err
		}
	}
	return []Event{{Entity: storyEntity(slug, sid), Event: evStory, Detail: m.detail}}, nil
}

// storyReadyVerb: PLANNED → READY. No worklog row — no worklog state
// exists for this transition (§6.2). (The spec's readiness precondition
// is DoR, the empty-blocked field, which PLANNED already satisfies; a
// DoD-content gate would be unspecified scope.)
func storyReadyVerb(e *cli.Env, slug, sid string) error {
	err := Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		return moveStory(context.Background(), tx, slug, sid, move{
			sources: []string{storyPlanned}, target: storyReady, detail: subReady,
		})
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s/%s -> READY\n", slug, sid)
	return nil
}

// storyStart: READY + DoR (empty blocked, schema-checked) + WIP slot
// free (the partial unique index is the arbiter; the verb pre-checks for
// a named message).
func storyStart(e *cli.Env, slug, sid string) error {
	err := Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		ctx := context.Background()
		// `start` does not route through moveStory (it owns the WIP-1
		// check), so it carries the terminal-epic guard itself.
		if err := refuseTerminalEpic(ctx, tx, slug, "story start"); err != nil {
			return nil, err
		}
		var wip string
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM stories WHERE epic = ? AND status = 'IN-PROGRESS'`, slug).Scan(&wip)
		if err == nil {
			return nil, refuse("wip", "WIP limit: story %s is IN-PROGRESS in epic %q — finish or block it first", wip, slug)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("story start: %w", err)
		}
		res, err := tx.ExecContext(ctx,
			`UPDATE stories SET status = 'IN-PROGRESS' WHERE epic = ? AND id = ?`, slug, sid)
		if err != nil {
			return nil, err //nolint:wrapcheck // matrix/DoR CHECKs speak through the mapper
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil, refuse("not-found", "no story %s in epic %q", sid, slug)
		}
		if err := appendWorklog(ctx, tx, slug, sid, storyInProgress, "", "", "", ""); err != nil {
			return nil, err
		}
		return []Event{{Entity: storyEntity(slug, sid), Event: evStory, Detail: "start"}}, nil
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s/%s -> IN-PROGRESS\n", slug, sid)
	return nil
}

func storyBlock(e *cli.Env, slug, sid, reason string) error {
	if reason == "" {
		return refuse("usage", "story block requires --reason")
	}
	if err := validateText("--reason", reason); err != nil {
		return err
	}
	err := Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		return moveStory(context.Background(), tx, slug, sid, move{
			sources: []string{storyPlanned, storyReady, storyInProgress},
			target:  storyBlocked, blocked: reason,
			worklog: "BLOCKED-ON-OWNER", note: reason, detail: "block: " + reason,
		})
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s/%s -> BLOCKED\n", slug, sid)
	return nil
}

// storyUnblock: BLOCKED → READY. No worklog row — the durable record is
// the events row carrying the resolution VERBATIM (§6.2): when the block
// was an owner question, the resolution starts with the literal `PO:`
// prefix and quotes the verdict; presence-of-text is verb-enforced only.
func storyUnblock(e *cli.Env, slug, sid, resolution string) error {
	if resolution == "" {
		return refuse("usage", "story unblock requires --resolution (what cleared the block)")
	}
	if err := validateText("--resolution", resolution); err != nil {
		return err
	}
	err := Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		return moveStory(context.Background(), tx, slug, sid, move{
			sources: []string{storyBlocked}, target: storyReady, detail: resolution,
		})
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s/%s -> READY\n", slug, sid)
	return nil
}

// storyDone: the status flip and the DONE worklog row are one
// transaction — no non-atomic path exists (§6.2).
func storyDoneVerb(e *cli.Env, slug, sid, commits, gate, review string) error {
	if commits == "" || gate == "" {
		return refuse("usage", "story done requires --commits and --gate")
	}
	for _, f := range [][2]string{{"--commits", commits}, {"--gate", gate}, {"--review", review}} {
		if err := validateText(f[0], f[1]); err != nil {
			return err
		}
	}
	err := Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		return moveStory(context.Background(), tx, slug, sid, move{
			sources: []string{storyInProgress}, target: storyDone,
			worklog: storyDone, commits: commits, gate: gate, review: review,
			detail: "done: " + commits + "; gate: " + gate,
		})
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s/%s -> DONE\n", slug, sid)
	return nil
}

// storyDissolve is the one story transition that stays open on a
// verb-closed epic (§6.4; amendment `r16-reports-only-what-a-verb-can-clear`):
// it is the repair R16's story finding names, and DISSOLVED is the only
// story target that satisfies close condition (1) rather than breaking it.
// The carve-out is narrow in three independent ways, each enforced
// elsewhere and each covered by a fixture: it does NOT widen the source set
// (an already-terminal story still meets moveStory's `wrong-state`
// refusal), it does not reach a DISSOLVED epic, and it does not reach a
// CLOSED epic that no `epic close` produced — refuseTerminalEpicExcept
// checks provenance, not status.
func storyDissolve(e *cli.Env, slug, sid, why string) error {
	if why == "" {
		return refuse("usage", "story dissolve requires --why")
	}
	if err := validateText("--why", why); err != nil {
		return err
	}
	err := Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		return moveStory(context.Background(), tx, slug, sid, move{
			sources: []string{storyPlanned, storyReady, storyBlocked, storyInProgress},
			target:  storyDissolved, worklog: storyDissolved, note: why, detail: "dissolve: " + why,

			closedEpicRepair: true,
		})
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s/%s -> DISSOLVED\n", slug, sid)
	return nil
}

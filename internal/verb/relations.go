package verb

import (
	"context"
	"database/sql"
	"flag"
	"fmt"

	"github.com/Spoloborota/selftracked/internal/cli"
	"github.com/Spoloborota/selftracked/internal/ref"
)

// relTripleArity: <id> <type> <id>.
const relTripleArity = 3

const (
	relDepends    = "depends"
	relRelates    = "relates"
	relSupersedes = "supersedes"
	relDuplicates = "duplicates"

	evRel = "rel"
)

// RelationVerbs returns the S5b `rel` catalog entry plus `log`.
func RelationVerbs() []cli.Verb {
	return []cli.Verb{relVerb(), logVerb()}
}

func relVerb() cli.Verb {
	var note string
	return cli.Verb{Name: "rel", Subs: []cli.Sub{
		{
			Name: "add", Arity: relTripleArity,
			Usage: "rel add <id> <depends|relates|supersedes> <id> [--note N] [--json]",
			Flags: func(fs *flag.FlagSet) { fs.StringVar(&note, "note", "", "why the relation exists") },
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return relAdd(e, pos, note)
			},
		},
		{
			Name: "rm", Arity: relTripleArity,
			Usage: "rel rm <id> <type> <id> [--json]",
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return relRM(e, pos)
			},
		},
		{
			Name: "tree", Arity: 1,
			Usage: "rel tree <id> [--json]",
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return relTree(e, pos)
			},
		},
		{
			Name: "cycles", Arity: 0,
			Usage: "rel cycles [--json]",
			Run: func(e *cli.Env, _ []string, _ *flag.FlagSet) error {
				return relCycles(e)
			},
		},
	}}
}

func relType(tok string, forAdd bool) (string, error) {
	switch tok {
	case relDepends, relRelates, relSupersedes:
		return tok, nil
	case relDuplicates:
		// The duplicates fact has one writer (set-status --dup-of) and one
		// remover (reopen); rel never offers it (§5.6).
		if forAdd {
			return "", refuse("usage", "rel has no duplicates type; set-status --dup-of is its single writer")
		}
		return "", refuse("usage", "rel has no duplicates type; reopen is its single remover")
	}
	return "", refuse("usage", "unknown relation type %q", tok)
}

func relAdd(e *cli.Env, pos []string, note string) error {
	from, err := taskID(pos[0])
	if err != nil {
		return err
	}
	typ, err := relType(pos[1], true)
	if err != nil {
		return err
	}
	to, err := taskID(pos[2])
	if err != nil {
		return err
	}
	if err := validateText("--note", note); err != nil {
		return err
	}
	err = Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		return relAddTx(tx, from, typ, to, note)
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s %s %s\n", ref.TaskRef(from), typ, ref.TaskRef(to))
	return nil
}

func relAddTx(tx *sql.Tx, from int64, typ string, to int64, note string) ([]Event, error) {
	ctx := context.Background()
	if err := relAddChecks(ctx, tx, from, typ, to); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO task_links (from_task, to_task, type, note) VALUES (?, ?, ?, ?)`,
		from, to, typ, note); err != nil {
		return nil, err //nolint:wrapcheck // constraint codes ride to the mapper
	}
	detail := "add " + typ + " " + ref.TaskRef(to)
	if note != "" {
		detail += ": " + note
	}
	return []Event{{Entity: ref.TaskRef(from), Event: evRel, Detail: detail}}, nil
}

// relAddChecks validates both endpoints, the §5.6 target-status rule,
// and the cycle rule for directed types.
func relAddChecks(ctx context.Context, tx *sql.Tx, from int64, typ string, to int64) error {
	target, err := scanTask(ctx, tx, to)
	if err != nil {
		return err
	}
	if _, err := scanTask(ctx, tx, from); err != nil {
		return err
	}
	// depends/supersedes refuse LABEL and DUPLICATE targets: a LABEL can
	// never become terminal, a DUPLICATE masks its canonical (§5.6).
	if typ != relRelates && (target.Status == statusLabel || target.Status == statusDuplicate) {
		return refuse("bad-target", "%s is %s; %s targets must be workable tasks",
			ref.TaskRef(to), target.Status, typ)
	}
	if typ != relRelates {
		cyclic, err := wouldCycle(ctx, tx, typ, from, to)
		if err != nil {
			return err
		}
		if cyclic {
			return refuse("cycle", "adding %s %s→%s would create a cycle",
				typ, ref.TaskRef(from), ref.TaskRef(to))
		}
	}
	return nil
}

// wouldCycle walks same-type edges from `to` looking for `from` — the
// edge about to be added would close a loop exactly when to reaches from.
func wouldCycle(ctx context.Context, tx *sql.Tx, typ string, from, to int64) (bool, error) {
	seen := map[int64]bool{}
	frontier := []int64{to}
	for len(frontier) > 0 {
		cur := frontier[0]
		frontier = frontier[1:]
		if cur == from {
			return true, nil
		}
		if seen[cur] {
			continue
		}
		seen[cur] = true
		next, err := outEdges(ctx, tx, cur, typ)
		if err != nil {
			return false, err
		}
		frontier = append(frontier, next...)
	}
	return false, nil
}

// outEdges lists cur's same-type successors.
func outEdges(ctx context.Context, tx *sql.Tx, cur int64, typ string) ([]int64, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT to_task FROM task_links WHERE from_task = ? AND type = ?`, cur, typ)
	if err != nil {
		return nil, fmt.Errorf("cycle walk: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []int64
	for rows.Next() {
		var next int64
		if err := rows.Scan(&next); err != nil {
			return nil, fmt.Errorf("cycle walk: %w", err)
		}
		out = append(out, next)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cycle walk: %w", err)
	}
	return out, nil
}

func relRM(e *cli.Env, pos []string) error {
	from, err := taskID(pos[0])
	if err != nil {
		return err
	}
	typ, err := relType(pos[1], false)
	if err != nil {
		return err
	}
	to, err := taskID(pos[2])
	if err != nil {
		return err
	}
	err = Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		ctx := context.Background()
		res, err := tx.ExecContext(ctx,
			`DELETE FROM task_links WHERE from_task = ? AND to_task = ? AND type = ?`, from, to, typ)
		if err != nil {
			return nil, err //nolint:wrapcheck // constraint codes ride to the mapper
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil, refuse("not-found", "no %s link %s→%s", typ, ref.TaskRef(from), ref.TaskRef(to))
		}
		return []Event{{
			Entity: ref.TaskRef(from), Event: evRel,
			Detail: "rm " + typ + " " + ref.TaskRef(to),
		}}, nil
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "removed %s %s %s\n", ref.TaskRef(from), typ, ref.TaskRef(to))
	return nil
}

func relTree(e *cli.Env, pos []string) error {
	id, err := taskID(pos[0])
	if err != nil {
		return err
	}
	return Read(context.Background(), func(ctx context.Context, db *sql.DB) error {
		// relates is stored once and read from both ends (§5.6).
		rows, err := db.QueryContext(ctx, `
			SELECT type, '#' || to_task, note FROM task_links WHERE from_task = ?
			UNION ALL
			SELECT type || ' (inbound)', '#' || from_task, note FROM task_links
			WHERE to_task = ? AND (type = 'relates' OR type <> 'relates')
			ORDER BY 1, 2`, id, id)
		if err != nil {
			return fmt.Errorf("rel tree: %w", err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var typ, other, note string
			if err := rows.Scan(&typ, &other, &note); err != nil {
				return fmt.Errorf("rel tree: %w", err)
			}
			_, _ = fmt.Fprintf(e.Stdout, "%s %s %s %s\n", ref.TaskRef(id), typ, other, note)
		}
		return rows.Err()
	})
}

func relCycles(e *cli.Env) error {
	return Read(context.Background(), func(ctx context.Context, db *sql.DB) error {
		// Advisory scan: any task that can reach itself over depends or
		// supersedes edges. rel add refuses new cycles, so hits here mean
		// raw-SQL writes — worth reporting, not guessing about.
		found := false
		for _, typ := range []string{relDepends, relSupersedes} {
			hits, err := cycleStarts(ctx, db, typ)
			if err != nil {
				return err
			}
			for _, id := range hits {
				found = true
				_, _ = fmt.Fprintf(e.Stdout, "cycle: %s (%s)\n", ref.TaskRef(id), typ)
			}
		}
		if !found {
			_, _ = fmt.Fprintln(e.Stdout, "no cycles")
		}
		return nil
	})
}

// logVerb lists the events trail for one entity (§6.2).
func logVerb() cli.Verb {
	var limit int
	return cli.Verb{Name: "log", Subs: []cli.Sub{{
		Arity: 1,
		Usage: "log <ref> [--limit N] [--json]",
		Flags: func(fs *flag.FlagSet) { fs.IntVar(&limit, "limit", 0, "show only the last N events") },
		Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
			r, err := ref.Parse(pos[0])
			if err != nil {
				return refuse("usage", "%v", err)
			}
			entity := canonicalEntity(r)
			return Read(context.Background(), func(ctx context.Context, db *sql.DB) error {
				q := `SELECT seq, at, event, detail FROM events WHERE entity = ? ORDER BY seq`
				args := []any{entity}
				if limit > 0 {
					q = `SELECT seq, at, event, detail FROM (
						SELECT seq, at, event, detail FROM events WHERE entity = ?
						ORDER BY seq DESC LIMIT ?) ORDER BY seq`
					args = append(args, limit)
				}
				rows, err := db.QueryContext(ctx, q, args...)
				if err != nil {
					return fmt.Errorf("log: %w", err)
				}
				defer func() { _ = rows.Close() }()
				for rows.Next() {
					var seq int64
					var at, event, detail string
					if err := rows.Scan(&seq, &at, &event, &detail); err != nil {
						return fmt.Errorf("log: %w", err)
					}
					_, _ = fmt.Fprintf(e.Stdout, "%d %s %s %s\n", seq, at, event, detail)
				}
				return rows.Err()
			})
		},
	}}}
}

// cycleStarts lists tasks that can reach themselves over typ edges.
func cycleStarts(ctx context.Context, db *sql.DB, typ string) ([]int64, error) {
	rows, err := db.QueryContext(ctx, `
		WITH RECURSIVE walk(start, cur) AS (
			SELECT from_task, to_task FROM task_links WHERE type = ?1
			UNION
			SELECT walk.start, l.to_task FROM walk
			JOIN task_links l ON l.from_task = walk.cur AND l.type = ?1
		)
		SELECT DISTINCT start FROM walk WHERE start = cur ORDER BY start`, typ)
	if err != nil {
		return nil, fmt.Errorf("rel cycles: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("rel cycles: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rel cycles: %w", err)
	}
	return out, nil
}

// canonicalEntity renders a parsed ref the one way events store entities.
func canonicalEntity(r ref.Ref) string {
	switch r.Kind {
	case ref.Task:
		return ref.TaskRef(r.Task)
	case ref.Epic:
		return epicPrefix + r.Epic
	case ref.Story:
		return epicPrefix + r.Epic + "/" + r.Story
	case ref.Artifact:
		return pos0(r)
	}
	return ""
}

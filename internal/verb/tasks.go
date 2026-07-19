package verb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/Spoloborota/selftracked/internal/cli"
	"github.com/Spoloborota/selftracked/internal/ref"
)

// Verbs returns the S5a task-lifecycle catalog entries.
// The status vocabulary, spelled once.
const (
	statusOpen      = "OPEN"
	statusInReview  = "IN-REVIEW"
	statusTriage    = "NEEDS-TRIAGE"
	statusDone      = "DONE"
	statusWontDo    = "WONT-DO"
	statusDuplicate = "DUPLICATE"
	statusLabel     = "LABEL"

	jsonRefKey    = "ref"
	jsonEpicKey   = "epic"
	jsonStatusKey = "status"

	// setStatusArity: <id> <STATUS>.
	setStatusArity = 2
	epicPrefix     = "epic:"
	evUnpark       = "unpark"
	evCreate       = "create"
)

// Verbs returns the task-lifecycle (S5a) and relation/dictionary (S5b)
// catalog entries.
func Verbs() []cli.Verb {
	groups := [][]cli.Verb{
		{
			createVerb(), showVerb(), listVerb(), readyVerb(),
			setStatusVerb(), reopenVerb(), parkVerb(), unparkVerb(), editVerb(),
		},
		RelationVerbs(), ArtifactVerbs(), DictVerbs(),
		{StaleVerb()},
	}
	var n int
	for _, g := range groups {
		n += len(g)
	}
	out := make([]cli.Verb, 0, n)
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

func refuse(code, format string, args ...any) error {
	return &cli.CodedError{Code: code, Message: fmt.Sprintf(format, args...), Status: refusal}
}

func printJSON(e *cli.Env, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s\n", b)
	return nil
}

// taskRow is a task as verbs read and print it.
type taskRow struct {
	ID       int64  `json:"ref"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Note     string `json:"note,omitempty"`
	Parked   string `json:"parked,omitempty"`
	Epic     string `json:"epic,omitempty"`
	renderID string
}

func scanTask(ctx context.Context, q interface {
	QueryRowContext(qctx context.Context, query string, args ...any) *sql.Row
}, id int64,
) (taskRow, error) {
	var t taskRow
	var epic sql.NullString
	err := q.QueryRowContext(ctx,
		`SELECT id, title, status, status_note, parked, COALESCE(epic, '') FROM tasks WHERE id = ?`, id).
		Scan(&t.ID, &t.Title, &t.Status, &t.Note, &t.Parked, &epic.String)
	if errors.Is(err, sql.ErrNoRows) {
		return t, refuse("not-found", "no task %s", ref.TaskRef(id))
	}
	if err != nil {
		return t, fmt.Errorf("read task: %w", err)
	}
	t.Epic = epic.String
	t.renderID = ref.TaskRef(t.ID)
	return t, nil
}

// ---- create ----

func createVerb() cli.Verb {
	var title, status, note, epic string
	var label bool
	return cli.Verb{Name: "create", Subs: []cli.Sub{{
		Arity: 0,
		Usage: "create --title T [--status OPEN|IN-REVIEW|NEEDS-TRIAGE] [--note N] [--epic SLUG] [--label] [--json]",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&title, "title", "", "the task title (required)")
			fs.StringVar(&status, "status", "", "initial status")
			fs.StringVar(&note, "note", "", "status note")
			fs.StringVar(&epic, "epic", "", "home epic slug")
			fs.BoolVar(&label, "label", false, "create a LABEL marker row")
		},
		Run: func(e *cli.Env, _ []string, _ *flag.FlagSet) error {
			target, err := createTarget(title, note, epic, status, label)
			if err != nil {
				return err
			}
			var id int64
			err = Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
				stamp := now()
				var epicVal any
				if epic != "" {
					epicVal = epic
				}
				err := tx.QueryRowContext(context.Background(),
					`INSERT INTO tasks (title, status, status_note, epic, created_at, updated_at)
					 VALUES (?, ?, ?, ?, ?, ?) RETURNING id`,
					title, target, note, epicVal, stamp, stamp).Scan(&id)
				if err != nil {
					return nil, fmt.Errorf("create: %w", err)
				}
				return []Event{{Entity: ref.TaskRef(id), Event: evCreate, Detail: title}}, nil
			})
			if err != nil {
				return err
			}
			if e.JSON {
				return printJSON(e, map[string]string{jsonRefKey: ref.TaskRef(id)})
			}
			_, _ = fmt.Fprintln(e.Stdout, ref.TaskRef(id))
			return nil
		},
	}}}
}

// createTarget validates create's flags and resolves the initial status.
// Terminal statuses at creation exist only via import (§6.2): the flag's
// domain is the three non-terminal states, and LABEL has its own flag.
func createTarget(title, note, epic, status string, label bool) (string, error) {
	for _, f := range [][2]string{{"--title", title}, {"--note", note}, {"--epic", epic}} {
		if err := validateText(f[0], f[1]); err != nil {
			return "", err
		}
	}
	if title == "" {
		return "", refuse("usage", "create requires --title")
	}
	switch {
	case label && status != "":
		return "", refuse("usage", "--label and --status are mutually exclusive")
	case label:
		return statusLabel, nil
	case status == "":
		return statusTriage, nil
	}
	switch status {
	case statusOpen, statusInReview, statusTriage:
		return status, nil
	}
	return "", refuse("status", "create cannot produce status %q; terminal statuses exist only via import", status)
}

// ---- show ----

func showVerb() cli.Verb {
	return cli.Verb{Name: "show", Subs: []cli.Sub{{
		Arity: 1,
		Usage: "show <ref> [--json]",
		Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
			r, err := ref.Parse(pos[0])
			if err != nil {
				return refuse("usage", "%v", err)
			}
			return Read(context.Background(), func(ctx context.Context, db *sql.DB) error {
				switch r.Kind {
				case ref.Task:
					return showTask(ctx, e, db, r.Task)
				case ref.Epic:
					return showEpic(ctx, e, db, r.Epic)
				case ref.Story:
					return showStory(ctx, e, db, r)
				case ref.Artifact:
					return showArtifact(ctx, e, db, r)
				}
				return refuse("usage", "unshowable reference")
			})
		},
	}}}
}

func showTask(ctx context.Context, e *cli.Env, db *sql.DB, id int64) error {
	t, err := scanTask(ctx, db, id)
	if err != nil {
		return err
	}
	if e.JSON {
		return printJSON(e, map[string]any{
			jsonRefKey: t.renderID, "title": t.Title,
			jsonStatusKey: t.Status, "note": t.Note, "parked": t.Parked, jsonEpicKey: t.Epic,
		})
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s %s [%s]", t.renderID, t.Title, t.Status)
	if t.Epic != "" {
		_, _ = fmt.Fprintf(e.Stdout, " %s%s", epicPrefix, t.Epic)
	}
	if t.Parked != "" {
		_, _ = fmt.Fprintf(e.Stdout, " parked:%s", t.Parked)
	}
	if t.Note != "" {
		_, _ = fmt.Fprintf(e.Stdout, "\n  note: %s", t.Note)
	}
	_, _ = fmt.Fprintln(e.Stdout)
	return nil
}

func showEpic(ctx context.Context, e *cli.Env, db *sql.DB, slug string) error {
	var goal, status string
	err := db.QueryRowContext(ctx, `SELECT goal, status FROM epics WHERE slug = ?`, slug).Scan(&goal, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return refuse("not-found", "no epic %q", slug)
	}
	if err != nil {
		return fmt.Errorf("read epic: %w", err)
	}
	if e.JSON {
		return printJSON(e, map[string]string{jsonEpicKey: slug, "goal": goal, jsonStatusKey: status})
	}
	_, _ = fmt.Fprintf(e.Stdout, "epic:%s [%s] %s\n", slug, status, goal)
	return nil
}

func showStory(ctx context.Context, e *cli.Env, db *sql.DB, r ref.Ref) error {
	var title, status string
	err := db.QueryRowContext(ctx,
		`SELECT title, status FROM stories WHERE epic = ? AND id = ?`, r.Epic, r.Story).Scan(&title, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return refuse("not-found", "no story %s in epic %q", r.Story, r.Epic)
	}
	if err != nil {
		return fmt.Errorf("read story: %w", err)
	}
	// Stories show their episodes (§6.2).
	rows, err := db.QueryContext(ctx,
		`SELECT seq, date, state, commits, note FROM worklog
		 WHERE epic = ? AND story = ? ORDER BY seq`, r.Epic, r.Story)
	if err != nil {
		return fmt.Errorf("read episodes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	type episode struct {
		Seq     int64  `json:"seq"`
		Date    string `json:"date"`
		State   string `json:"state"`
		Commits string `json:"commits,omitempty"`
		Note    string `json:"note,omitempty"`
	}
	var episodes []episode
	for rows.Next() {
		var ep episode
		if err := rows.Scan(&ep.Seq, &ep.Date, &ep.State, &ep.Commits, &ep.Note); err != nil {
			return fmt.Errorf("read episodes: %w", err)
		}
		episodes = append(episodes, ep)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read episodes: %w", err)
	}
	if e.JSON {
		return printJSON(e, map[string]any{
			jsonEpicKey: r.Epic, "story": r.Story, "title": title, jsonStatusKey: status, "episodes": episodes,
		})
	}
	_, _ = fmt.Fprintf(e.Stdout, "epic:%s/%s [%s] %s\n", r.Epic, r.Story, status, title)
	for _, ep := range episodes {
		_, _ = fmt.Fprintf(e.Stdout, "  %d %s %s %s %s\n", ep.Seq, ep.Date, ep.State, ep.Commits, ep.Note)
	}
	return nil
}

func showArtifact(ctx context.Context, e *cli.Env, db *sql.DB, r ref.Ref) error {
	var id int64
	err := db.QueryRowContext(ctx,
		`SELECT id FROM artifacts WHERE class = ? AND scope = ? AND relpath = ?`,
		r.Class, r.Scope, r.Rel).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return refuse("not-found", "no artifact %s", pos0(r))
	}
	if err != nil {
		return fmt.Errorf("read artifact: %w", err)
	}
	// Reverse lookup: the tasks and epics linked to it (§6.2).
	tasks, err := linkedRefs(ctx, db,
		`SELECT '#' || task, role FROM task_artifacts WHERE artifact = ? ORDER BY task`, id)
	if err != nil {
		return err
	}
	epics, err := linkedRefs(ctx, db,
		`SELECT 'epic:' || epic, role FROM epic_artifacts WHERE artifact = ? ORDER BY epic`, id)
	if err != nil {
		return err
	}
	if e.JSON {
		return printJSON(e, map[string]any{"artifact": pos0(r), "tasks": tasks, "epics": epics})
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s\n  linked: %s\n", pos0(r), strings.Join(append(tasks, epics...), " "))
	return nil
}

// linkedRefs drains a (ref, role) reverse-lookup query.
func linkedRefs(ctx context.Context, db *sql.DB, q string, id int64) ([]string, error) {
	rows, err := db.QueryContext(ctx, q, id)
	if err != nil {
		return nil, fmt.Errorf("reverse lookup: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var r, role string
		if err := rows.Scan(&r, &role); err != nil {
			return nil, fmt.Errorf("reverse lookup: %w", err)
		}
		out = append(out, r+":"+role)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reverse lookup: %w", err)
	}
	return out, nil
}

func pos0(r ref.Ref) string {
	if r.Scope != "" {
		return r.Class + "@" + r.Scope + ":" + r.Rel
	}
	return r.Class + ":" + r.Rel
}

// ---- list / ready ----

func listVerb() cli.Verb {
	var status, epic string
	var parked, labels bool
	return cli.Verb{Name: "list", Subs: []cli.Sub{{
		Arity: 0,
		Usage: "list [--status S] [--epic SLUG] [--parked] [--labels] [--json]",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&status, "status", "", "filter by status")
			fs.StringVar(&epic, "epic", "", "filter by epic")
			fs.BoolVar(&parked, "parked", false, "only parked tasks")
			fs.BoolVar(&labels, "labels", false, "include LABEL marker rows")
		},
		Run: func(e *cli.Env, _ []string, _ *flag.FlagSet) error {
			return Read(context.Background(), func(ctx context.Context, db *sql.DB) error {
				// v_backlog already hides LABEL (§5); --labels widens to the
				// raw table instead.
				q := `SELECT id, title, status, status_note, parked, COALESCE(epic,'') FROM tasks WHERE 1=1`
				if !labels {
					q += ` AND status <> 'LABEL'`
				}
				var args []any
				if status != "" {
					q += ` AND status = ?`
					args = append(args, status)
				}
				if epic != "" {
					q += ` AND epic = ?`
					args = append(args, epic)
				}
				if parked {
					q += ` AND parked <> ''`
				}
				q += ` ORDER BY id`
				return listQuery(ctx, e, db, q, args...)
			})
		},
	}}}
}

func readyVerb() cli.Verb {
	var epic string
	return cli.Verb{Name: "ready", Subs: []cli.Sub{{
		Arity: 0,
		Usage: "ready [--epic SLUG] [--json]",
		Flags: func(fs *flag.FlagSet) { fs.StringVar(&epic, "epic", "", "filter by epic") },
		Run: func(e *cli.Env, _ []string, _ *flag.FlagSet) error {
			return Read(context.Background(), func(ctx context.Context, db *sql.DB) error {
				q := `SELECT id, title, status, status_note, parked, COALESCE(epic,'') FROM v_ready`
				var args []any
				if epic != "" {
					q += ` WHERE epic = ?`
					args = append(args, epic)
				}
				return listQuery(ctx, e, db, q, args...)
			})
		},
	}}}
}

func listQuery(ctx context.Context, e *cli.Env, db *sql.DB, q string, args ...any) error {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("list: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []taskRow
	for rows.Next() {
		var t taskRow
		if err := rows.Scan(&t.ID, &t.Title, &t.Status, &t.Note, &t.Parked, &t.Epic); err != nil {
			return fmt.Errorf("list: %w", err)
		}
		t.renderID = ref.TaskRef(t.ID)
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("list: %w", err)
	}
	if e.JSON {
		type item struct {
			Ref    string `json:"ref"`
			Title  string `json:"title"`
			Status string `json:"status"`
			Parked string `json:"parked,omitempty"`
			Epic   string `json:"epic,omitempty"`
		}
		items := make([]item, 0, len(out))
		for _, t := range out {
			items = append(items, item{Ref: t.renderID, Title: t.Title, Status: t.Status, Parked: t.Parked, Epic: t.Epic})
		}
		return printJSON(e, items)
	}
	for _, t := range out {
		_, _ = fmt.Fprintf(e.Stdout, "%s %s [%s]\n", t.renderID, t.Title, t.Status)
	}
	return nil
}

// ---- set-status ----

func setStatusVerb() cli.Verb {
	var note string
	var dupOf int64
	return cli.Verb{Name: "set-status", Subs: []cli.Sub{{
		Arity: setStatusArity,
		Usage: "set-status <id> <STATUS> [--note N] [--dup-of ID] [--json]",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&note, "note", "", "status note (the owner verdict on IN-REVIEW exits)")
			fs.Int64Var(&dupOf, "dup-of", 0, "canonical task for DUPLICATE")
		},
		Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
			id, err := taskID(pos[0])
			if err != nil {
				return err
			}
			target := pos[len(pos)-1]
			if err := validateText("--note", note); err != nil {
				return err
			}
			var from string
			err = Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
				return setStatus(tx, id, target, note, dupOf, &from)
			})
			if err != nil {
				return err
			}
			if e.JSON {
				return printJSON(e, map[string]string{jsonRefKey: ref.TaskRef(id), "from": from, "to": target})
			}
			_, _ = fmt.Fprintf(e.Stdout, "%s %s -> %s\n", ref.TaskRef(id), from, target)
			return nil
		},
	}}}
}

func setStatus(tx *sql.Tx, id int64, target, note string, dupOf int64, from *string) ([]Event, error) {
	ctx := context.Background()
	cur, err := scanTask(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	*from = cur.Status
	terminal := map[string]bool{statusDone: true, statusWontDo: true, statusDuplicate: true}
	if terminal[cur.Status] && target == statusOpen {
		return nil, refuse("use-reopen", "%s is %s; terminal→OPEN is reopen's job: selftracked reopen %d --why ...",
			ref.TaskRef(id), cur.Status, id)
	}

	dupVal, err := duplicateTarget(ctx, tx, target, dupOf)
	if err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE tasks SET status = ?, status_note = ?, dup_of = ?, parked = '', updated_at = ? WHERE id = ?`,
		target, note, dupVal, now(), id); err != nil {
		return nil, err //nolint:wrapcheck // the mapper reads the driver's constraint codes verbatim
	}
	events := []Event{{Entity: ref.TaskRef(id), Event: "set-status", Detail: note}}
	if cur.Parked != "" {
		// Any status transition clears parked automatically, and the
		// clearing is events-logged (§5.5).
		events = append(events, Event{Entity: ref.TaskRef(id), Event: evUnpark, Detail: "auto-cleared by status transition"})
	}
	if target == statusDuplicate {
		// set-status is the single writer of the duplicates link (§5.6).
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO task_links (from_task, to_task, type) VALUES (?, ?, 'duplicates')`,
			id, dupOf); err != nil {
			return nil, err //nolint:wrapcheck // constraint codes ride to the mapper
		}
	}
	return events, nil
}

// duplicateTarget validates the --dup-of pairing and the no-chains rule
// (§5.5; re-checked by R7): the canonical must not itself be DUPLICATE.
func duplicateTarget(ctx context.Context, tx *sql.Tx, target string, dupOf int64) (any, error) {
	if target != statusDuplicate {
		if dupOf != 0 {
			return nil, refuse("usage", "--dup-of only accompanies DUPLICATE")
		}
		return nil, nil //nolint:nilnil // a NULL dup_of is the value
	}
	if dupOf == 0 {
		return nil, refuse("usage", "DUPLICATE requires --dup-of <id>")
	}
	canon, err := scanTask(ctx, tx, dupOf)
	if err != nil {
		return nil, err
	}
	if canon.Status == statusDuplicate {
		return nil, refuse("dup-chain", "%s is itself DUPLICATE; point --dup-of at its canonical instead",
			ref.TaskRef(dupOf))
	}
	return dupOf, nil
}

// ---- reopen ----

func reopenVerb() cli.Verb {
	var why string
	return cli.Verb{Name: "reopen", Subs: []cli.Sub{{
		Arity: 1,
		Usage: "reopen <id> --why TEXT [--json]",
		Flags: func(fs *flag.FlagSet) { fs.StringVar(&why, "why", "", "the reopen reason (required)") },
		Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
			id, err := taskID(pos[0])
			if err != nil {
				return err
			}
			if why == "" {
				return refuse("usage", "reopen requires --why")
			}
			if err := validateText("--why", why); err != nil {
				return err
			}
			err = Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
				return reopenTask(tx, id, why)
			})
			if err != nil {
				return err
			}
			if e.JSON {
				return printJSON(e, map[string]string{jsonRefKey: ref.TaskRef(id), "to": statusOpen})
			}
			_, _ = fmt.Fprintf(e.Stdout, "%s reopened\n", ref.TaskRef(id))
			return nil
		},
	}}}
}

// reopenTask is the sanctioned terminal→OPEN path (§6.2).
func reopenTask(tx *sql.Tx, id int64, why string) ([]Event, error) {
	ctx := context.Background()
	cur, err := scanTask(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	switch cur.Status {
	case statusDone, statusWontDo:
		// Reopenable.
	case statusDuplicate:
		// reopen is the sole remover of the duplicates link (§5.6); the
		// link tables carry no delete trigger by amendment — relations,
		// not history, with the events trail as the audit.
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM task_links WHERE from_task = ? AND type = 'duplicates'`, id); err != nil {
			return nil, err //nolint:wrapcheck // constraint codes ride to the mapper
		}
	default:
		return nil, refuse("not-terminal", "%s is %s; reopen only reopens terminal tasks",
			ref.TaskRef(id), cur.Status)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE tasks SET status = 'OPEN', status_note = ?, dup_of = NULL, parked = '',
		 updated_at = ? WHERE id = ?`,
		why, now(), id); err != nil {
		return nil, err //nolint:wrapcheck // constraint codes ride to the mapper
	}
	return []Event{{Entity: ref.TaskRef(id), Event: "reopen", Detail: why}}, nil
}

// ---- park / unpark ----

func parkVerb() cli.Verb {
	var why string
	return cli.Verb{Name: "park", Subs: []cli.Sub{{
		Arity: 1,
		Usage: "park <id> --why TEXT [--json]",
		Flags: func(fs *flag.FlagSet) { fs.StringVar(&why, "why", "", "the deferral reason (required)") },
		Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
			id, err := taskID(pos[0])
			if err != nil {
				return err
			}
			if why == "" {
				return refuse("usage", "park requires --why")
			}
			if err := validateText("--why", why); err != nil {
				return err
			}
			err = Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
				if _, err := tx.ExecContext(context.Background(),
					`UPDATE tasks SET parked = ?, updated_at = ? WHERE id = ?`, why, now(), id); err != nil {
					return nil, err //nolint:wrapcheck // the parked-only-on-open CHECK speaks through the mapper
				}
				return []Event{{Entity: ref.TaskRef(id), Event: "park", Detail: why}}, nil
			})
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(e.Stdout, "%s parked (leaves the ready frontier, keeps its status)\n", ref.TaskRef(id))
			return nil
		},
	}}}
}

func unparkVerb() cli.Verb {
	return cli.Verb{Name: "unpark", Subs: []cli.Sub{{
		Arity: 1,
		Usage: "unpark <id> [--json]",
		Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
			id, err := taskID(pos[0])
			if err != nil {
				return err
			}
			err = Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
				if _, err := tx.ExecContext(context.Background(),
					`UPDATE tasks SET parked = '', updated_at = ? WHERE id = ?`, now(), id); err != nil {
					return nil, err //nolint:wrapcheck // constraint codes ride to the mapper
				}
				return []Event{{Entity: ref.TaskRef(id), Event: evUnpark, Detail: ""}}, nil
			})
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(e.Stdout, "%s unparked\n", ref.TaskRef(id))
			return nil
		},
	}}}
}

// ---- edit ----

// editPrefix bounds the old→new values an edit event records (§5.9): edit
// is ungated and its fields are unbounded prose, so events carry pointers,
// not archives — the full new value lives in the entity row and every
// dump, the full old value in the prior committed dump.
const editPrefix = 40

func bounded(s string) string {
	if len(s) <= editPrefix {
		return s
	}
	return s[:editPrefix] + "…"
}

func editVerb() cli.Verb {
	var title, note, epic string
	var detach bool
	return cli.Verb{Name: "edit", Subs: []cli.Sub{{
		Arity: 1,
		Usage: "edit <ref> [--title T] [--note N] [--epic SLUG|--detach] [--json]",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&title, "title", "", "new title")
			fs.StringVar(&note, "note", "", "new status note")
			fs.StringVar(&epic, "epic", "", "re-home to this epic")
			fs.BoolVar(&detach, "detach", false, "detach from its epic")
		},
		Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
			r, err := ref.Parse(pos[0])
			if err != nil {
				return refuse("usage", "%v", err)
			}
			if err := validateEdit(r, title, note, epic, detach); err != nil {
				return err
			}
			err = Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
				return editTask(tx, r.Task, title, note, epic, detach)
			})
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(e.Stdout, "%s edited\n", ref.TaskRef(r.Task))
			return nil
		},
	}}}
}

// validateEdit gates edit's flag combinations. Epic/story field edits
// (--goal/--dod/--consumes/--produces) arrive with their verbs at S6.
func validateEdit(r ref.Ref, title, note, epic string, detach bool) error {
	if r.Kind != ref.Task {
		return refuse("usage", "edit handles task refs at this stage; epic/story fields arrive with their verbs")
	}
	for _, f := range [][2]string{{"--title", title}, {"--note", note}, {"--epic", epic}} {
		if err := validateText(f[0], f[1]); err != nil {
			return err
		}
	}
	if epic != "" && detach {
		return refuse("usage", "--epic and --detach are mutually exclusive")
	}
	if title == "" && note == "" && epic == "" && !detach {
		return refuse("usage", "edit needs at least one field flag")
	}
	return nil
}

func editTask(tx *sql.Tx, id int64, title, note, epic string, detach bool) ([]Event, error) {
	ctx := context.Background()
	cur, err := scanTask(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	set, args, changes := editSet(cur, title, note, epic, detach)
	if len(set) == 0 {
		return nil, refuse("no-change", "nothing to edit on %s", ref.TaskRef(id))
	}
	args = append(args, now(), id)
	//nolint:gosec // the SET list is built from fixed column fragments above
	q := "UPDATE tasks SET " + strings.Join(set, ", ") + ", updated_at = ? WHERE id = ?"
	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		return nil, err //nolint:wrapcheck // constraint codes ride to the mapper
	}
	return []Event{{Entity: ref.TaskRef(id), Event: "edit", Detail: strings.Join(changes, "; ")}}, nil
}

// editSet builds the changed-field SET fragments and their bounded
// old→new change records (§5.9).
func editSet(cur taskRow, title, note, epic string, detach bool) ([]string, []any, []string) {
	var set []string
	var args []any
	var changes []string
	if title != "" && title != cur.Title {
		set, args = append(set, "title = ?"), append(args, title)
		changes = append(changes, fmt.Sprintf("title: %s→%s", bounded(cur.Title), bounded(title)))
	}
	if note != "" && note != cur.Note {
		set, args = append(set, "status_note = ?"), append(args, note)
		changes = append(changes, fmt.Sprintf("note: %s→%s", bounded(cur.Note), bounded(note)))
	}
	switch {
	case detach:
		set = append(set, "epic = NULL")
		changes = append(changes, fmt.Sprintf("epic: %s→", bounded(cur.Epic)))
	case epic != "" && epic != cur.Epic:
		set, args = append(set, "epic = ?"), append(args, epic)
		changes = append(changes, fmt.Sprintf("epic: %s→%s", bounded(cur.Epic), bounded(epic)))
	}
	return set, args, changes
}

func taskID(tok string) (int64, error) {
	r, err := ref.Parse(tok)
	if err != nil || r.Kind != ref.Task {
		return 0, refuse("usage", "%q is not a task id", tok)
	}
	return r.Task, nil
}

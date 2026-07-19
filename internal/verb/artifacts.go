package verb

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Spoloborota/selftracked/internal/cli"
	"github.com/Spoloborota/selftracked/internal/ref"
)

// linkArity: <target> <artifact-ref>.
const linkArity = 2

const (
	evLink      = "link"
	evUnlink    = "unlink"
	evArchive   = "archive"
	evUnarchive = "unarchive"
	roleHome    = "home"
)

// ArtifactVerbs returns the S5b `link` catalog entry (with its unlink and
// archive/unarchive sub-forms as §6.2 prints them).
func ArtifactVerbs() []cli.Verb {
	return []cli.Verb{linkVerb(), unlinkVerb()}
}

// linkVerb dispatches §6.2's three link forms. The keyword dispatch is
// unambiguous by construction: the §4 grammar cannot produce the bare
// tokens `archive`/`unarchive` (INV-187), so the first positional decides.
func linkVerb() cli.Verb {
	var role string
	var force bool
	return cli.Verb{Name: "link", Subs: []cli.Sub{{
		Arity: linkArity,
		Usage: "link <id|epic:SLUG> <class[@scope]:relpath> --role R | " +
			"link archive|unarchive <artifact-ref> [--force] [--json]",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&role, "role", "", "the link role (closed vocabulary)")
			fs.BoolVar(&force, "force", false, "archive even a live home")
		},
		Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
			switch pos[0] {
			case evArchive:
				return archiveArtifact(e, pos[1], true, force)
			case evUnarchive:
				return archiveArtifact(e, pos[1], false, false)
			}
			return linkArtifact(e, pos[0], pos[1], role)
		},
	}}}
}

func unlinkVerb() cli.Verb {
	return cli.Verb{Name: "unlink", Subs: []cli.Sub{{
		Arity: linkArity,
		Usage: "unlink <id|epic:SLUG> <class[@scope]:relpath> [--json]",
		Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
			return unlinkArtifact(e, pos[0], pos[1])
		},
	}}}
}

// linkTarget resolves the first positional to a task id or epic slug.
func linkTarget(tok string) (ref.Ref, error) {
	r, err := ref.Parse(tok)
	if err != nil {
		// A pointer to a non-file target (a worklog row, a prose section)
		// is a note, not a link — the grammar refusing it is the stated
		// limitation doing its job (INV-133).
		return r, refuse("usage", "%q is not a linkable target: %v", tok, err)
	}
	if r.Kind != ref.Task && r.Kind != ref.Epic {
		return r, refuse("usage", "links attach to tasks or epics; %q is neither", tok)
	}
	return r, nil
}

func artifactRef(tok string) (ref.Ref, error) {
	r, err := ref.Parse(tok)
	if err != nil || r.Kind != ref.Artifact {
		return r, refuse("usage", "%q is not a class[@scope]:relpath artifact reference", tok)
	}
	return r, nil
}

func linkArtifact(e *cli.Env, targetTok, artTok, role string) error {
	target, err := linkTarget(targetTok)
	if err != nil {
		return err
	}
	art, err := artifactRef(artTok)
	if err != nil {
		return err
	}
	if role == "" {
		return refuse("usage", "link requires --role from the closed vocabulary")
	}
	err = Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		ctx := context.Background()
		artID, err := ensureArtifact(ctx, tx, art)
		if err != nil {
			return nil, err
		}
		var q string
		var args []any
		var entity string
		if target.Kind == ref.Task {
			q = `INSERT INTO task_artifacts (task, artifact, role) VALUES (?, ?, ?)`
			args = []any{target.Task, artID, role}
			entity = ref.TaskRef(target.Task)
		} else {
			q = `INSERT INTO epic_artifacts (epic, artifact, role) VALUES (?, ?, ?)`
			args = []any{target.Epic, artID, role}
			entity = epicPrefix + target.Epic
		}
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return nil, err //nolint:wrapcheck // role vocabulary and FKs speak through the mapper
		}
		return []Event{{Entity: entity, Event: evLink, Detail: role + " " + pos0(art)}}, nil
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "linked %s %s (%s)\n", targetTok, pos0(art), role)
	return nil
}

// ensureArtifact resolves or creates the artifacts row, enforcing the
// §6.2 containment and existence rules.
func ensureArtifact(ctx context.Context, tx *sql.Tx, art ref.Ref) (int64, error) {
	var root string
	var ephemeral int64
	err := tx.QueryRowContext(ctx,
		`SELECT root, ephemeral FROM path_dictionary WHERE class = ? AND scope = ?`,
		art.Class, art.Scope).Scan(&root, &ephemeral)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, refuse("not-found", "no registered root for class %q scope %q; run selftracked paths set",
			art.Class, art.Scope)
	}
	if err != nil {
		return 0, fmt.Errorf("resolve root: %w", err)
	}
	clean, err := containedPath(art, root, ephemeral)
	if err != nil {
		return 0, err
	}
	var id int64
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM artifacts WHERE class = ? AND scope = ? AND relpath = ?`,
		art.Class, art.Scope, clean).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("resolve artifact: %w", err)
	}
	err = tx.QueryRowContext(ctx,
		`INSERT INTO artifacts (class, scope, relpath) VALUES (?, ?, ?) RETURNING id`,
		art.Class, art.Scope, clean).Scan(&id)
	if err != nil {
		return 0, err //nolint:wrapcheck // constraint codes ride to the mapper
	}
	return id, nil
}

// containedPath enforces §6.2's containment (no .. escapes, no absolute
// paths — root registration is what retention and R3 reason about) and
// the existence rule (the file must exist unless the class is ephemeral).
func containedPath(art ref.Ref, root string, ephemeral int64) (string, error) {
	clean := filepath.ToSlash(filepath.Clean(art.Rel))
	if filepath.IsAbs(art.Rel) || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", refuse("containment", "%q escapes the %s root; relpaths resolve inside their registered root",
			art.Rel, art.Class)
	}
	if ephemeral == 0 {
		full := filepath.Join(root, clean)
		if _, err := os.Stat(full); err != nil {
			return "", refuse("not-found", "file not found: %s", full)
		}
	}
	return clean, nil
}

// lookupArtifactID finds an existing artifacts row by reference.
func lookupArtifactID(ctx context.Context, tx *sql.Tx, art ref.Ref) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM artifacts WHERE class = ? AND scope = ? AND relpath = ?`,
		art.Class, art.Scope, filepath.ToSlash(filepath.Clean(art.Rel))).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, refuse("not-found", "no artifact %s", pos0(art))
	}
	if err != nil {
		return 0, fmt.Errorf("resolve artifact: %w", err)
	}
	return id, nil
}

func unlinkArtifact(e *cli.Env, targetTok, artTok string) error {
	target, err := linkTarget(targetTok)
	if err != nil {
		return err
	}
	art, err := artifactRef(artTok)
	if err != nil {
		return err
	}
	err = Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		ctx := context.Background()
		artID, err := lookupArtifactID(ctx, tx, art)
		if err != nil {
			return nil, err
		}
		var res sql.Result
		var entity string
		if target.Kind == ref.Task {
			res, err = tx.ExecContext(ctx,
				`DELETE FROM task_artifacts WHERE task = ? AND artifact = ?`, target.Task, artID)
			entity = ref.TaskRef(target.Task)
		} else {
			res, err = tx.ExecContext(ctx,
				`DELETE FROM epic_artifacts WHERE epic = ? AND artifact = ?`, target.Epic, artID)
			entity = epicPrefix + target.Epic
		}
		if err != nil {
			return nil, err //nolint:wrapcheck // constraint codes ride to the mapper
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil, refuse("not-found", "no link between %s and %s", targetTok, pos0(art))
		}
		return []Event{{Entity: entity, Event: evUnlink, Detail: pos0(art)}}, nil
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "unlinked %s %s\n", targetTok, pos0(art))
	return nil
}

func archiveArtifact(e *cli.Env, artTok string, toArchived, force bool) error {
	art, err := artifactRef(artTok)
	if err != nil {
		return err
	}
	err = Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		ctx := context.Background()
		artID, err := lookupArtifactID(ctx, tx, art)
		if err != nil {
			return nil, err
		}
		if toArchived {
			// Archiving a live home hides a task/epic's narrative home:
			// warn and demand --force (§6.2).
			live, err := liveHomeCount(ctx, tx, artID)
			if err != nil {
				return nil, err
			}
			if live > 0 && !force {
				return nil, refuse("live-home",
					"%s is a live home for %d entities; re-run with --force to archive it anyway",
					pos0(art), live)
			}
		}
		val := int64(0)
		event := evUnarchive
		if toArchived {
			val, event = 1, evArchive
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE artifacts SET archived = ? WHERE id = ?`, val, artID); err != nil {
			return nil, err //nolint:wrapcheck // constraint codes ride to the mapper
		}
		return []Event{{Entity: pos0(art), Event: event, Detail: ""}}, nil
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s %s\n", map[bool]string{true: "archived", false: "unarchived"}[toArchived], pos0(art))
	return nil
}

func liveHomeCount(ctx context.Context, tx *sql.Tx, artID int64) (int64, error) {
	var n int64
	err := tx.QueryRowContext(ctx, `
		SELECT (SELECT COUNT(*) FROM task_artifacts WHERE artifact = ?1 AND role = ?2)
		     + (SELECT COUNT(*) FROM epic_artifacts WHERE artifact = ?1 AND role = ?2)`,
		artID, roleHome).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("live-home count: %w", err)
	}
	return n, nil
}

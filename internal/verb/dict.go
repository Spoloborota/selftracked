package verb

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Spoloborota/selftracked/internal/cli"
)

const (
	evPaths = "paths"
	// dirMode: group-traversable directories, matching the repo's temp
	// conventions.
	dirMode = 0o750
	// pairArity: two positionals (a token and its value).
	pairArity = 2
)

// DictVerbs returns the S5b dictionary verbs: paths and config.
func DictVerbs() []cli.Verb {
	return []cli.Verb{pathsVerb(), configVerb()}
}

// classScope parses the CLASS[@SCOPE] token paths verbs take. This is not
// a §4 reference (no relpath); the token names a dictionary row.
func classScope(tok string) (string, string, error) {
	class, scope, _ := strings.Cut(tok, "@")
	if class == "" || strings.Contains(class, ":") {
		return "", "", refuse("usage", "%q is not CLASS[@SCOPE]", tok)
	}
	return class, scope, nil
}

func pathsVerb() cli.Verb {
	var ephemeral, withFiles bool
	var note string
	return cli.Verb{Name: "paths", Subs: []cli.Sub{
		{
			Name: "ls", Arity: 0, Usage: "paths ls [--json]",
			Run: func(e *cli.Env, _ []string, _ *flag.FlagSet) error {
				return pathsLS(e)
			},
		},
		{
			Name: "set", Arity: pairArity,
			Usage: "paths set CLASS[@SCOPE] ROOT [--ephemeral] [--note N] [--json]",
			Flags: func(fs *flag.FlagSet) {
				fs.BoolVar(&ephemeral, "ephemeral", false, "existence-exempt class")
				fs.StringVar(&note, "note", "", "what this root holds")
			},
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return pathsSet(e, pos, ephemeral, note)
			},
		},
		{
			Name: "move", Arity: pairArity,
			Usage: "paths move CLASS[@SCOPE] NEWROOT [--with-files] [--json]",
			Flags: func(fs *flag.FlagSet) {
				fs.BoolVar(&withFiles, "with-files", false, "move the directory too (git mv when possible)")
			},
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return pathsMove(e, pos, withFiles)
			},
		},
	}}
}

func pathsLS(e *cli.Env) error {
	return Read(context.Background(), func(ctx context.Context, db *sql.DB) error {
		rows, err := db.QueryContext(ctx,
			`SELECT class, scope, root, ephemeral, note FROM path_dictionary ORDER BY class, scope`)
		if err != nil {
			return fmt.Errorf("paths ls: %w", err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var class, scope, root, note string
			var eph int64
			if err := rows.Scan(&class, &scope, &root, &eph, &note); err != nil {
				return fmt.Errorf("paths ls: %w", err)
			}
			marker := ""
			if eph == 1 {
				marker = " (ephemeral)"
			}
			name := class
			if scope != "" {
				name = class + "@" + scope
			}
			_, _ = fmt.Fprintf(e.Stdout, "%s -> %s%s %s\n", name, root, marker, note)
		}
		return rows.Err()
	})
}

func pathsSet(e *cli.Env, pos []string, ephemeral bool, note string) error {
	class, scope, err := classScope(pos[0])
	if err != nil {
		return err
	}
	root := filepath.ToSlash(filepath.Clean(pos[1]))
	if err := validateText("ROOT", root); err != nil {
		return err
	}
	if err := validateText("--note", note); err != nil {
		return err
	}
	var warned string
	err = Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		ctx := context.Background()
		// Same-class overlapping roots are an ownership ambiguity — warned,
		// not refused (§5.2). Cross-class nesting is normal and silent.
		w, err := overlapWarning(ctx, tx, class, scope, root)
		if err != nil {
			return nil, err
		}
		warned = w
		eph := int64(0)
		if ephemeral {
			eph = 1
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO path_dictionary (class, scope, root, ephemeral, note) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (class, scope) DO UPDATE SET root = excluded.root,
				ephemeral = excluded.ephemeral, note = excluded.note`,
			class, scope, root, eph, note); err != nil {
			return nil, err //nolint:wrapcheck // constraint codes ride to the mapper
		}
		return []Event{{Entity: pos[0] + ":" + root, Event: evPaths, Detail: "set " + pos[0] + " -> " + root}}, nil
	})
	if err != nil {
		return err
	}
	if warned != "" {
		_, _ = fmt.Fprintln(e.Stderr, warned)
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s -> %s\n", pos[0], root)
	return nil
}

func overlapWarning(ctx context.Context, tx *sql.Tx, class, scope, root string) (string, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT scope, root FROM path_dictionary WHERE class = ? AND scope <> ?`, class, scope)
	if err != nil {
		return "", fmt.Errorf("overlap check: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var otherScope, otherRoot string
		if err := rows.Scan(&otherScope, &otherRoot); err != nil {
			return "", fmt.Errorf("overlap check: %w", err)
		}
		if root == otherRoot || strings.HasPrefix(root+"/", otherRoot+"/") ||
			strings.HasPrefix(otherRoot+"/", root+"/") {
			return fmt.Sprintf("warning: %s@%s root %q overlaps scope %q root %q (ownership ambiguity)",
				class, scope, root, otherScope, otherRoot), nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("overlap check: %w", err)
	}
	return "", nil
}

func pathsMove(e *cli.Env, pos []string, withFiles bool) error {
	class, scope, err := classScope(pos[0])
	if err != nil {
		return err
	}
	newRoot := filepath.ToSlash(filepath.Clean(pos[1]))
	var oldRoot string
	err = Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		ctx := context.Background()
		err := tx.QueryRowContext(ctx,
			`SELECT root FROM path_dictionary WHERE class = ? AND scope = ?`, class, scope).Scan(&oldRoot)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, refuse("not-found", "no registered root for %s", pos[0])
		}
		if err != nil {
			return nil, fmt.Errorf("paths move: %w", err)
		}
		// Moving a directory is a one-row update (§1 principle 7): every
		// class[@scope]:relpath reference resolves through this row.
		if _, err := tx.ExecContext(ctx,
			`UPDATE path_dictionary SET root = ? WHERE class = ? AND scope = ?`,
			newRoot, class, scope); err != nil {
			return nil, err //nolint:wrapcheck // constraint codes ride to the mapper
		}
		if withFiles {
			if err := moveFiles(ctx, oldRoot, newRoot); err != nil {
				return nil, err
			}
		}
		return []Event{{
			Entity: pos[0] + ":" + newRoot, Event: evPaths,
			Detail: "move " + oldRoot + " -> " + newRoot,
		}}, nil
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s: %s -> %s\n", pos[0], oldRoot, newRoot)
	return nil
}

// moveFiles moves the root directory: `git mv` inside a git repo (rename
// AND stage — no red window), plain rename otherwise (§6.2).
func moveFiles(ctx context.Context, oldRoot, newRoot string) error {
	if _, err := os.Stat(oldRoot); err != nil {
		return refuse("not-found", "root directory %q does not exist to move", oldRoot)
	}
	if err := os.MkdirAll(filepath.Dir(newRoot), dirMode); err != nil {
		return fmt.Errorf("paths move: %w", err)
	}
	if inGitRepo(ctx) {
		// The roots come from the path dictionary, not user-typed shell text.
		out, err := exec.CommandContext(ctx, "git", "mv", oldRoot, newRoot).CombinedOutput() //nolint:gosec
		if err != nil {
			return refuse("git", "git mv failed: %s", strings.TrimSpace(string(out)))
		}
		return nil
	}
	if err := os.Rename(oldRoot, newRoot); err != nil {
		return fmt.Errorf("paths move: %w", err)
	}
	return nil
}

func inGitRepo(ctx context.Context) bool {
	err := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree").Run()
	return err == nil
}

// configVerb: the sanctioned editor for the closed schema-v1 key list.
func configVerb() cli.Verb {
	return cli.Verb{Name: "config", Subs: []cli.Sub{
		{
			Name: "ls", Arity: 0, Usage: "config ls [--json]",
			Run: func(e *cli.Env, _ []string, _ *flag.FlagSet) error {
				return configLS(e)
			},
		},
		{
			Name: "set", Arity: pairArity,
			Usage: "config set <production_globs|idle_days|prime_cap> VALUE [--json]",
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return configSet(e, pos)
			},
		},
	}}
}

// configKeys is the closed schema-v1 list; new keys arrive only with
// schema versions (§6.2). System meta keys are NOT here by design.
var configKeys = map[string]func(string) error{
	"idle_days": positiveInt,
	"prime_cap": positiveInt,
	"production_globs": func(v string) error {
		for g := range strings.FieldsSeq(v) {
			if _, err := path.Match(g, "probe"); err != nil {
				return refuse("invalid", "unparseable glob %q", g)
			}
		}
		return nil
	},
}

func positiveInt(v string) error {
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return refuse("invalid", "%q is not a positive integer", v)
	}
	return nil
}

func configLS(e *cli.Env) error {
	return Read(context.Background(), func(ctx context.Context, db *sql.DB) error {
		rows, err := db.QueryContext(ctx, `
			SELECT key, value FROM meta
			WHERE key IN ('production_globs', 'idle_days', 'prime_cap') ORDER BY key`)
		if err != nil {
			return fmt.Errorf("config ls: %w", err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err != nil {
				return fmt.Errorf("config ls: %w", err)
			}
			_, _ = fmt.Fprintf(e.Stdout, "%s = %s\n", k, v)
		}
		return rows.Err()
	})
}

func configSet(e *cli.Env, pos []string) error {
	key, value := pos[0], pos[1]
	validate, known := configKeys[key]
	if !known {
		// The closed list is the contract: unknown keys and the system meta
		// keys (schema_version, events_archived_through, active_verb) are
		// equally unsettable here (§5.1, §6.2).
		return refuse("unknown-key",
			"%q is not a configuration key; the closed list: idle_days, prime_cap, production_globs", key)
	}
	if err := validateText("VALUE", value); err != nil {
		return err
	}
	if err := validate(value); err != nil {
		return err
	}
	err := Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		if _, err := tx.ExecContext(context.Background(),
			`UPDATE meta SET value = ? WHERE key = ?`, value, key); err != nil {
			return nil, err //nolint:wrapcheck // constraint codes ride to the mapper
		}
		return []Event{{Entity: key + "=" + value, Event: "config", Detail: "set " + key + " = " + value}}, nil
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s = %s\n", key, value)
	return nil
}

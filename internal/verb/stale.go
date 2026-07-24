package verb

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Spoloborota/selftracked/internal/cli"
)

// staleName is the verb and its refusal code — one token, so `prime`'s
// `json:"stale"` tags do not tip goconst over its threshold on a coincidence.
const staleName = "stale"

// StaleVerb returns §6.2's `stale`: git-changed files intersected with
// resolved non-ephemeral artifact links of non-terminal work, ordered
// path ASC — deterministic, so hooks and humans see one list.
func StaleVerb() cli.Verb {
	var since string
	var quiet bool
	return cli.Verb{Name: staleName, Subs: []cli.Sub{{
		Arity: 0,
		Usage: "stale [--since REF] [--quiet] [--json]",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&since, "since", "", "diff against this git ref (default: uncommitted changes)")
			fs.BoolVar(&quiet, "quiet", false, "print nothing; exit 1 when anything is stale")
		},
		Run: func(e *cli.Env, _ []string, _ *flag.FlagSet) error {
			return staleRun(e, since, quiet)
		},
	}}}
}

func staleRun(e *cli.Env, since string, quiet bool) error {
	ctx := context.Background()
	changed, err := gitChanged(ctx, since)
	if err != nil {
		return err
	}
	var stale []string
	err = Read(ctx, func(ctx context.Context, db *sql.DB) error {
		linked, err := activeArtifactPaths(ctx, db)
		if err != nil {
			return err
		}
		for p := range changed {
			if linked[p] {
				stale = append(stale, p)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(stale) // path ASC, deterministically (§6.2)
	if quiet {
		if len(stale) > 0 {
			return &cli.CodedError{Code: staleName, Message: fmt.Sprintf("%d stale artifact(s)", len(stale)), Status: refusal}
		}
		return nil
	}
	for _, p := range stale {
		_, _ = fmt.Fprintln(e.Stdout, p)
	}
	return nil
}

func gitChanged(ctx context.Context, since string) (map[string]bool, error) {
	args := []string{"diff", "--name-only"}
	if strings.HasPrefix(since, "-") {
		// A leading-dash --since would reach git diff as an option rather
		// than a revision (`--output=PATH` writes a file). No revision has
		// this shape: git refuses a branch or tag starting with '-'.
		return nil, cli.Usage("--since %q is not a revision", since)
	}
	if since != "" {
		args = append(args, since)
	} else {
		args = append(args, "HEAD")
	}
	out, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		detail := ""
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			detail = ": " + strings.TrimSpace(string(exitErr.Stderr))
		}
		return nil, refuse("git", "git %s failed%s", strings.Join(args, " "), detail)
	}
	changed := map[string]bool{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			changed[filepath.ToSlash(line)] = true
		}
	}
	return changed, nil
}

// activeArtifactPaths resolves every non-ephemeral, non-archived artifact
// linked to non-terminal work to its repo-relative path.
func activeArtifactPaths(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT pd.root || '/' || a.relpath
		FROM artifacts a
		JOIN path_dictionary pd ON pd.class = a.class AND pd.scope = a.scope
		WHERE pd.ephemeral = 0 AND a.archived = 0 AND (
			EXISTS (SELECT 1 FROM task_artifacts ta JOIN tasks t ON t.id = ta.task
			        WHERE ta.artifact = a.id AND t.status NOT IN ('DONE','WONT-DO','DUPLICATE'))
			OR EXISTS (SELECT 1 FROM epic_artifacts ea JOIN epics ep ON ep.slug = ea.epic
			        WHERE ea.artifact = a.id AND ep.status NOT IN ('CLOSED','DISSOLVED')))`)
	if err != nil {
		return nil, fmt.Errorf("stale: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]bool{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("stale: %w", err)
		}
		out[filepath.ToSlash(filepath.Clean(p))] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("stale: %w", err)
	}
	return out, nil
}

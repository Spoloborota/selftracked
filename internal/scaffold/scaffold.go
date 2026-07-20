// Package scaffold implements `init` (§9): the one command that turns an
// empty repo into a tracker. It creates the database (seeding meta and the
// default path dictionary), writes the deterministic dump and STATE.md, and
// generates every documentation artifact §9/§11 mandate — READMEs, the ADR
// template, PROMPT.md, AGENTS.md, the `.claude/` rule and skill, and the
// .gitignore entries. Hooks are S8b's; the SessionStart settings file ships
// here as static text (it names verbs, not code).
package scaffold

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Spoloborota/selftracked/internal/dump"
	"github.com/Spoloborota/selftracked/internal/schema"
	"github.com/Spoloborota/selftracked/internal/state"
)

//go:embed templates
var templates embed.FS

const (
	instanceDir = ".selftracked"
	dbFile      = "db.sqlite"
	dumpFile    = "dump.sql"
	stateFile   = "STATE.md"
	hooksDir    = ".selftracked/hooks"
	dirMode     = 0o750
	fileMode    = 0o644
	execMode    = 0o755

	// Target paths referenced in more than one place.
	promptTarget   = "PROMPT.md"
	agentsTarget   = "AGENTS.md"
	workReadme     = "work/README.md"
	settingsTarget = ".claude/settings.json"
)

// defaultRoots is the seeded path dictionary (§5.2, D14 minimal seed): the
// five classes every tracker starts with, each with its conventional root.
// init CREATES every root so a fresh `verify` is green (INV-064).
var defaultRoots = []struct {
	class, root string
	ephemeral   int
}{
	{"research", "docs/research", 0},
	{"adr", "docs/decisions", 0},
	{"workdir", "work", 1},
	{"run", "work/runs", 1},
	{"report", "work/reports", 0},
}

// staticFiles maps an embedded template to its path under the repo root.
// These are written on a fresh init; on adoption (see writeStatic) a few
// are protected from clobbering a host project's own files.
var staticFiles = []struct{ tmpl, target string }{
	{"templates/PROMPT.md", promptTarget},
	{"templates/AGENTS.md", agentsTarget},
	{"templates/adr_template.md", "docs/decisions/_template.md"},
	{"templates/readme_research.md", "docs/research/README.md"},
	{"templates/readme_decisions.md", "docs/decisions/README.md"},
	{"templates/readme_work.md", workReadme},
	{"templates/readme_runs.md", "work/runs/README.md"},
	{"templates/readme_reports.md", "work/reports/README.md"},
	{"templates/claude_rule.md", ".claude/rules/selftracked.md"},
	{"templates/claude_skill.md", ".claude/skills/selftracked/SKILL.md"},
	{"templates/claude_settings.json", settingsTarget},
}

// writeScaffold sets up (or refreshes) a tracker under root. It is
// deliberately non-destructive:
//
//   - A clone — a tracked dump.sql present with no local db.sqlite (the DB
//     is gitignored) — is NEVER re-initialised: init would overwrite the
//     tracked state with an empty one. It refuses and points at `load`,
//     even with --force, because a clone's truth is its dump.
//   - An existing tracker (db.sqlite present) refuses without --force. With
//     --force it is REFRESHED, not rebuilt: the derived files (dump, STATE)
//     are regenerated from the existing database and the generated docs are
//     refreshed — no row is dropped, the DB is opened read-only.
//   - A fresh repo gets the full build.
//
// Generated documents are never overwritten if they already exist
// (writeStatic), so an adopted repo's own PROMPT.md/README/ADR files
// survive; only files init itself owns exclusively (the dump, STATE.md, the
// database) are rewritten.
func writeScaffold(ctx context.Context, root string, force bool) error {
	hasDB, err := guardExisting(filepath.Join(root, instanceDir), force)
	if err != nil {
		return err
	}
	if err := makeDirs(root); err != nil {
		return err
	}
	if err := buildOrRefresh(ctx, root, hasDB); err != nil {
		return err
	}
	if err := writeStatic(root); err != nil {
		return err
	}
	if err := writeHooks(root); err != nil {
		return err
	}
	return mergeGitignore(root)
}

// guardExisting enforces the non-destructive contract and reports whether a
// local database is present. A clone (tracked dump, no DB) is never
// re-initialised; an existing tracker refuses without --force.
func guardExisting(instance string, force bool) (bool, error) {
	hasDB := exists(filepath.Join(instance, dbFile))
	hasDump := exists(filepath.Join(instance, dumpFile))
	if hasDump && !hasDB {
		return false, &cloneError{path: filepath.Join(instance, dumpFile)}
	}
	if hasDB && !force {
		return false, &existsError{path: filepath.Join(instance, dbFile)}
	}
	return hasDB, nil
}

// buildOrRefresh builds a fresh database or, when one already exists (the
// --force path), refreshes the derived files from it without dropping a row.
func buildOrRefresh(ctx context.Context, root string, hasDB bool) error {
	if hasDB {
		return refreshDatabase(ctx, root) // --force: keep the data
	}
	return buildDatabase(ctx, root) // fresh
}

// hookFiles are the generated git hooks (§9). They are TRACKED (not
// gitignored, INV-401): the tracked surface a clone inherits so
// `core.hooksPath .selftracked/hooks` activates the same gate everywhere.
var hookFiles = []struct{ tmpl, name string }{
	{"templates/hooks/pre-commit", "pre-commit"},
	{"templates/hooks/post-commit", "post-commit"},
}

// writeHooks writes the pre-commit and post-commit scripts executable,
// under the selftracked-owned .selftracked/hooks/. Unlike the generated
// docs (writeStatic), these are always rewritten: they live in
// selftracked's own namespace, never an adopter's, so a refresh keeps them
// current rather than preserving a stale copy.
func writeHooks(root string) error {
	for _, h := range hookFiles {
		content, err := templates.ReadFile(h.tmpl)
		if err != nil {
			return fmt.Errorf("init: read hook %s: %w", h.tmpl, err)
		}
		target := filepath.Join(root, hooksDir, h.name)
		if err := os.WriteFile(target, content, execMode); err != nil {
			return fmt.Errorf("init: write hook %s: %w", h.name, err)
		}
		// os.WriteFile only applies the mode when creating the file; a
		// --force refresh over an existing hook that lost its +x (a prior
		// checkout, an umask, core.fileMode=false) would otherwise stay
		// non-executable and git would silently skip it. Chmod unconditionally.
		if err := os.Chmod(target, execMode); err != nil {
			return fmt.Errorf("init: chmod hook %s: %w", h.name, err)
		}
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// makeDirs creates the instance dir, every seeded root, and the .claude
// subdirectories.
func makeDirs(root string) error {
	fixed := []string{instanceDir, hooksDir, ".claude/rules", ".claude/skills/selftracked"}
	dirs := make([]string, 0, len(fixed)+len(defaultRoots))
	dirs = append(dirs, fixed...)
	for _, r := range defaultRoots {
		dirs = append(dirs, r.root)
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), dirMode); err != nil {
			return fmt.Errorf("init: mkdir %s: %w", d, err)
		}
	}
	return nil
}

// buildDatabase creates a fresh DB (schema.Create seeds meta), seeds the
// default path dictionary, and writes the derived files.
func buildDatabase(ctx context.Context, root string) error {
	dbPath := filepath.Join(root, instanceDir, dbFile)
	db, err := schema.Open(dbPath)
	if err != nil {
		return fmt.Errorf("init: open db: %w", err)
	}
	defer func() { _ = db.Close() }()
	if err := schema.Create(ctx, db); err != nil {
		return fmt.Errorf("init: create schema: %w", err)
	}
	if err := seedPathDictionary(ctx, db); err != nil {
		return err
	}
	return writeDerived(ctx, root, db)
}

// refreshDatabase regenerates the derived files from an EXISTING database
// (the --force path). The DB is opened read-only: --force refreshes the
// tracker's generated surfaces, it does not touch a single tracked row.
func refreshDatabase(ctx context.Context, root string) error {
	dbPath := filepath.Join(root, instanceDir, dbFile)
	db, err := schema.OpenRead(dbPath)
	if err != nil {
		return fmt.Errorf("init: open db: %w", err)
	}
	defer func() { _ = db.Close() }()
	return writeDerived(ctx, root, db)
}

// writeDerived serializes the dump and renders STATE.md in the §6.1/§8.3
// order — dump file, THEN STATE.md, THEN the sidecar — so a crash between
// steps leaves derived files stale, never a sidecar that certifies a dump
// paired with the wrong STATE.md. (The composite dump.WriteFiles writes the
// sidecar immediately after the dump, which is why init cannot use it; the
// S5a close review split those functions for exactly this reason.)
func writeDerived(ctx context.Context, root string, db *sql.DB) error {
	instance := filepath.Join(root, instanceDir)
	text, err := dump.Serialize(ctx, db)
	if err != nil {
		return fmt.Errorf("init: serialize: %w", err)
	}
	if err := dump.WriteDumpFile(instance, text); err != nil {
		return fmt.Errorf("init: write dump: %w", err)
	}
	stateMD, err := state.Render(ctx, db)
	if err != nil {
		return fmt.Errorf("init: render STATE.md: %w", err)
	}
	if err := writeAt(filepath.Join(root, stateFile), stateMD); err != nil {
		return err
	}
	if err := dump.WriteSidecar(instance, text); err != nil {
		return fmt.Errorf("init: write sidecar: %w", err)
	}
	return nil
}

func seedPathDictionary(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("init: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, r := range defaultRoots {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO path_dictionary (class, scope, root, ephemeral) VALUES (?, '', ?, ?)`,
			r.class, r.root, r.ephemeral); err != nil {
			return fmt.Errorf("init: seed path %s: %w", r.class, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("init: commit seed: %w", err)
	}
	return nil
}

// writeStatic writes every generated document, skipping any target that
// already exists. init cannot tell an adopter's own PROMPT.md / README /
// ADR template from a stale selftracked copy, so it never overwrites one —
// a generated doc is authored once, and refreshing it is a delete-and-init
// the owner performs deliberately. Only the files init owns exclusively
// (the dump, STATE.md, the DB) are rewritten on --force.
func writeStatic(root string) error {
	for _, f := range staticFiles {
		target := filepath.Join(root, f.target)
		if exists(target) {
			continue // never clobber an existing file
		}
		content, err := templates.ReadFile(f.tmpl)
		if err != nil {
			return fmt.Errorf("init: read template %s: %w", f.tmpl, err)
		}
		if err := writeAt(target, content); err != nil {
			return err
		}
	}
	return nil
}

// mergeGitignore appends the selftracked entries to any existing
// .gitignore (or creates it), never duplicating a line already present.
func mergeGitignore(root string) error {
	want, err := templates.ReadFile("templates/gitignore")
	if err != nil {
		return fmt.Errorf("init: read gitignore template: %w", err)
	}
	target := filepath.Join(root, ".gitignore")
	//nolint:gosec // init reads/writes the target repo's own .gitignore by design
	existing, err := os.ReadFile(target)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("init: read .gitignore: %w", err)
	}
	if len(existing) == 0 {
		if err := writeAt(target, want); err != nil {
			return fmt.Errorf("init: write .gitignore: %w", err)
		}
		return nil
	}
	add := gitignoreAdditions(existing, want)
	if add == "" {
		return nil
	}
	merged := string(existing)
	if !strings.HasSuffix(merged, "\n") {
		merged += "\n"
	}
	merged += "\n# selftracked\n" + add
	if err := writeAt(target, []byte(merged)); err != nil {
		return fmt.Errorf("init: merge .gitignore: %w", err)
	}
	return nil
}

// gitignoreAdditions returns the non-comment lines of want not already
// present in existing, each newline-terminated (empty if nothing to add).
func gitignoreAdditions(existing, want []byte) string {
	present := map[string]bool{}
	for line := range strings.SplitSeq(string(existing), "\n") {
		present[strings.TrimSpace(line)] = true
	}
	var add strings.Builder
	for line := range strings.SplitSeq(string(want), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") || present[t] {
			continue
		}
		add.WriteString(line + "\n")
	}
	return add.String()
}

// writeAt writes b to path with the standard file mode.
func writeAt(path string, b []byte) error {
	//nolint:gosec // init writes files at the target repo root by design
	if err := os.WriteFile(path, b, fileMode); err != nil {
		return fmt.Errorf("init: write %s: %w", path, err)
	}
	return nil
}

// existsError is init's refusal when a tracker already exists and --force
// was not given.
type existsError struct{ path string }

func (e *existsError) Error() string {
	return e.path + " already exists; pass --force to refresh the generated files (the database is preserved)"
}

// cloneError is init's refusal on a clone: a tracked dump with no local DB.
// Re-initialising would discard the tracked state, so init points at load.
type cloneError struct{ path string }

func (e *cloneError) Error() string {
	return "a tracked " + e.path + " is present but there is no local database; " +
		"run selftracked load to rebuild it — init would discard the tracked state"
}

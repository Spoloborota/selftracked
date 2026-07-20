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
	dirMode     = 0o750
	fileMode    = 0o644

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

// protectedTargets are files a host project commonly already owns; init
// never overwrites them (it would clobber the adopter's own content). A
// fresh repo has none of these, so they are written normally there.
var protectedTargets = map[string]bool{
	agentsTarget:   true,
	settingsTarget: true,
}

// writeScaffold performs a full init under root. force permits
// re-initializing over an existing tracker (the DB and dump are rebuilt;
// generated docs are refreshed; protected and merge files are still
// handled gently).
func writeScaffold(ctx context.Context, root string, force bool) error {
	dbPath := filepath.Join(root, instanceDir, dbFile)
	if _, err := os.Stat(dbPath); err == nil && !force {
		return &existsError{path: dbPath}
	}
	if err := makeDirs(root); err != nil {
		return err
	}
	if err := buildDatabase(ctx, root); err != nil {
		return err
	}
	if err := writeStatic(root); err != nil {
		return err
	}
	return mergeGitignore(root)
}

// makeDirs creates the instance dir, every seeded root, and the .claude
// subdirectories.
func makeDirs(root string) error {
	fixed := []string{instanceDir, ".claude/rules", ".claude/skills/selftracked"}
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

// buildDatabase creates the DB (schema.Create seeds meta), seeds the
// default path dictionary, writes the dump + sidecar, and renders STATE.md.
func buildDatabase(ctx context.Context, root string) error {
	instance := filepath.Join(root, instanceDir)
	dbPath := filepath.Join(instance, dbFile)
	_ = os.Remove(dbPath) // --force: rebuild from scratch, no stale rows
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
	text, err := dump.Serialize(ctx, db)
	if err != nil {
		return fmt.Errorf("init: serialize: %w", err)
	}
	if err := dump.WriteFiles(instance, text); err != nil {
		return fmt.Errorf("init: write dump: %w", err)
	}
	stateMD, err := state.Render(ctx, db)
	if err != nil {
		return fmt.Errorf("init: render STATE.md: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, "STATE.md"), stateMD, fileMode); err != nil {
		return fmt.Errorf("init: write STATE.md: %w", err)
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

// writeStatic writes every generated document, skipping a protected target
// that already exists so an adopted repo's own AGENTS.md / .claude settings
// are never clobbered.
func writeStatic(root string) error {
	for _, f := range staticFiles {
		target := filepath.Join(root, f.target)
		if protectedTargets[f.target] {
			if _, err := os.Stat(target); err == nil {
				continue // adopter already owns this file
			}
		}
		content, err := templates.ReadFile(f.tmpl)
		if err != nil {
			return fmt.Errorf("init: read template %s: %w", f.tmpl, err)
		}
		if err := os.WriteFile(target, content, fileMode); err != nil {
			return fmt.Errorf("init: write %s: %w", f.target, err)
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
	return e.path + " already exists; pass --force to reinitialize"
}

package verb

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Spoloborota/selftracked/internal/cli"
	"github.com/Spoloborota/selftracked/internal/state"
)

// stateFile is the generated projection at the repo root (§9), a sibling of
// .selftracked, not a file inside it. stateFileMode is the tracked doc mode
// (§9): world-readable, matching the scaffold's other generated files.
const (
	stateFile     = "STATE.md"
	stateFileMode = 0o644
)

// StateVerb returns §6.2's `state`: it regenerates STATE.md from the DB via
// the deterministic renderer (INV-274). Read-only against tracker state — it
// opens the DB read-only, renders, and writes the one tracked file; no DB
// mutation, so it does not run the write pipeline.
func StateVerb() cli.Verb {
	return cli.Verb{Name: "state", Subs: []cli.Sub{{
		Arity: 0,
		Usage: "state",
		Run: func(e *cli.Env, _ []string, _ *flag.FlagSet) error {
			return stateRun(e)
		},
	}}}
}

func stateRun(e *cli.Env) error {
	ctx := context.Background()
	err := Read(ctx, func(ctx context.Context, db *sql.DB) error {
		return renderState(ctx, db, instanceDir)
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(e.Stdout, "wrote", filepath.Join(filepath.Dir(instanceDir), stateFile))
	return nil
}

// renderState is the pipeline's STATE.md slot (the S8c wiring of the stub in
// pipeline.go) and the `state` verb's body: render the DB to bytes, write
// them to STATE.md at the repo root. STATE.md is a sibling of .selftracked,
// so its directory is the parent of instanceDir. The write is atomic
// (temp + rename) so a crash never leaves a half-written projection paired
// with a fresh dump.
func renderState(ctx context.Context, db *sql.DB, instDir string) error {
	text, err := state.Render(ctx, db)
	if err != nil {
		return fmt.Errorf("render STATE.md: %w", err)
	}
	root := filepath.Dir(instDir)
	return atomicWrite(filepath.Join(root, stateFile), text)
}

// atomicWrite lands bytes at target via a temp file in the same directory
// and a rename — the rename is atomic on the local filesystems v0 targets.
func atomicWrite(target string, text []byte) error {
	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, filepath.Base(target)+".tmp-*")
	if err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(text); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", target, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	// STATE.md is a tracked, world-readable projection (§9) — 0644 like the
	// scaffold's other generated docs; os.CreateTemp makes it 0600.
	if err := os.Chmod(tmpName, stateFileMode); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	return nil
}

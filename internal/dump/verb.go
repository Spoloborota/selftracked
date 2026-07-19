package dump

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Spoloborota/selftracked/internal/cli"
	"github.com/Spoloborota/selftracked/internal/schema"
)

// Paths inside the instance directory (§3). The sidecar is per-machine
// and gitignored; the dump is the tracked review surface.
const (
	instanceDir = ".selftracked"
	dbFile      = "db.sqlite"
	dumpFile    = "dump.sql"
	hashFile    = "dump.hash"

	// sidecarMode: the sidecar is a per-machine scratch value; owner-only
	// is the least surprising permission for it.
	sidecarMode = 0o600
)

// Verb returns the §6.2 `dump [--stdout]` catalog entry.
func Verb() cli.Verb {
	var stdout bool
	return cli.Verb{
		Name: "dump",
		Subs: []cli.Sub{{
			Arity: 0,
			Usage: "dump [--stdout] [--json]",
			Flags: func(fs *flag.FlagSet) {
				fs.BoolVar(&stdout, "stdout", false, "print the dump instead of writing it")
			},
			Run: func(e *cli.Env, _ []string, _ *flag.FlagSet) error {
				return run(e, stdout)
			},
		}},
	}
}

func run(e *cli.Env, stdout bool) error {
	dir := instanceDir
	dbPath := filepath.Join(dir, dbFile)
	if _, err := os.Stat(dbPath); err != nil {
		return &cli.CodedError{
			Code:    "not-found",
			Message: "no " + dbPath + " here; run selftracked init first", Status: 1,
		}
	}
	db, err := schema.OpenRead(dbPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", dbPath, err)
	}
	defer func() { _ = db.Close() }()

	text, err := Serialize(context.Background(), db)
	if err != nil {
		return err
	}
	if stdout {
		_, err := e.Stdout.Write(text)
		return err //nolint:wrapcheck // the write target is the caller's own stream
	}
	return WriteFiles(dir, text)
}

// WriteFiles lands the dump and its sidecar the §8.3 way: render to a
// temp file, atomic rename, then the sidecar hash — a crash between the
// steps leaves derived files stale, never torn, and the next writer
// regenerates them. The write-verb pipeline does NOT use this composite:
// §6.1 puts STATE.md between the dump and the sidecar, so the pipeline
// calls WriteDumpFile and WriteSidecar around its renderer instead.
func WriteFiles(dir string, text []byte) error {
	if err := WriteDumpFile(dir, text); err != nil {
		return err
	}
	return WriteSidecar(dir, text)
}

// WriteDumpFile lands dump.sql alone via temp + atomic rename.
func WriteDumpFile(dir string, text []byte) error {
	target := filepath.Join(dir, dumpFile)
	tmp, err := os.CreateTemp(dir, dumpFile+".tmp-*")
	if err != nil {
		return fmt.Errorf("dump: temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(text); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("dump: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("dump: close temp: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("dump: rename: %w", err)
	}
	return nil
}

// WriteSidecar records the SHA-256 of the dump these bytes came from —
// §8.4 names init, write verbs, dump and load as its writers; the
// divergence matrix that READS it is S8b's.
func WriteSidecar(dir string, text []byte) error {
	sum := sha256.Sum256(text)
	sidecar := filepath.Join(dir, hashFile)
	content := []byte(hex.EncodeToString(sum[:]) + "\n")
	if err := os.WriteFile(sidecar, content, sidecarMode); err != nil {
		return fmt.Errorf("dump: sidecar: %w", err)
	}
	return nil
}

package migrate

import (
	"context"
	"fmt"

	"github.com/Spoloborota/selftracked/internal/cli"
)

// CLIGate is the dispatcher's version-gate hook (§6.1: every verb begins
// with the gate, before the §8.4 divergence check). It selects the
// per-verb mode, runs the gate on the working directory's instance, and
// surfaces the outcome: the notice line always goes to stderr — no
// output-limiting flag suppresses it — and a migration additionally
// rides Env.Migrated into prime's JSON (§11.1).
func CLIGate(e *cli.Env, verbName string) error {
	var mode Mode
	switch verbName {
	case "prime":
		// prime reports a newer-than-binary dump non-fatally in healthy
		// JSON (dump_requires_newer_binary, §8.6/§11.1) instead of dying
		// into the SessionStart fallback chain's uninformative loop.
		mode.SkipNewerRefusal = true
	case "load":
		// load replaces the database from the tracked dump: comparison
		// (i) still guards it, the migration arm would rebuild a thing
		// load is about to discard.
		mode.SkipMigration = true
	}
	res, err := Gate(context.Background(), DefaultDir, mode)
	if err != nil {
		return err
	}
	switch {
	case res.Healed:
		_, _ = fmt.Fprintf(e.Stderr, "selftracked: schema v%d re-dump completed (migration-crash residue healed)\n", res.To)
	case res.Migrated:
		_, _ = fmt.Fprintf(e.Stderr, "selftracked: schema migrated v%d→v%d\n", res.From, res.To)
		e.Migrated = fmt.Sprintf("v%d→v%d", res.From, res.To)
	}
	return nil
}

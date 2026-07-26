package verb

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"github.com/Spoloborota/selftracked/internal/cli"
)

const evCriteria = "criteria"

// criterionTimeout bounds each runnable criterion (§6.2 "per-command
// timeout"): long enough for a test suite, short enough that a hung
// command cannot hang the close.
const criterionTimeout = 5 * time.Minute

// executeCriterion runs one `$ ` command through the shell from the repo
// root with inherited env. Threat model (§6.2, stated in code as the
// opening record demands): runnable criteria are shell commands from repo
// state — executing them is the same trust decision as running the
// repo's build/tests or its tracked hooks; a hostile branch already owns
// those surfaces, so criteria add no new one.
func executeCriterion(ctx context.Context, command string) (bool, string) {
	cctx, cancel := context.WithTimeout(ctx, criterionTimeout)
	defer cancel()
	//nolint:gosec // repo-state command by design; see the threat model above
	cmd := exec.CommandContext(cctx, "sh", "-c", command)
	out, err := cmd.CombinedOutput()
	detail := command
	if err != nil {
		detail += ": " + firstLine(string(out)) + " (" + err.Error() + ")"
		return false, detail
	}
	return true, detail
}

func firstLine(s string) string {
	for i, r := range s {
		if r == '\n' {
			return s[:i]
		}
	}
	const clip = 120
	if len(s) > clip {
		return s[:clip]
	}
	return s
}

// CriteriaVerbs returns the S6 `criteria` catalog entry.
func CriteriaVerbs() []cli.Verb {
	var text, criterion, evidence string
	return []cli.Verb{{Name: "criteria", Subs: []cli.Sub{
		{
			Name: subAdd, Arity: 1, Usage: "criteria add SLUG --text|--criterion T [--json]",
			Flags: func(fs *flag.FlagSet) {
				fs.StringVar(&text, criterionName, "", "the criterion (prefix '$ ' makes it runnable)")
				fs.StringVar(&criterion, criterionAlias, "",
					"the same flag as --text, under the corpus's historical spelling")
			},
			Run: func(e *cli.Env, pos []string, fs *flag.FlagSet) error {
				value, flagName, err := criterionFlag(fs, text, criterion)
				if err != nil {
					return err
				}
				return criteriaAdd(e, pos[0], value, flagName)
			},
		},
		{
			Name: "met", Arity: pairArity, Usage: "criteria met SLUG SEQ --evidence E [--json]",
			Flags: func(fs *flag.FlagSet) { fs.StringVar(&evidence, "evidence", "", "why it is met (required)") },
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return criteriaMet(e, pos[0], pos[1], evidence)
			},
		},
		{
			Name: "check", Arity: 1, Usage: "criteria check SLUG [--json]",
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return criteriaCheck(e, pos[0])
			},
		},
	}}}
}

// criterionFlag resolves `criteria add`'s two spellings of ONE flag (§6.2,
// amendment `one-name-for-the-criterion-field`). `--criterion` is an
// accepted flag rather than a diagnosed typo because a diagnosis is not
// available here: the dispatcher parses flags in `parseFlags`
// (internal/cli/dispatch.go) and an unknown one dies in `fs.Parse` before
// any verb body runs, so the spelling an adopter learned from the import
// corpus can be given a meaning but never a message.
//
// `fs.Visit` — not the empty string — answers which were supplied: it walks
// only the flags actually set, so a `--text` given an empty value stays
// distinguishable from an absent `--text`, and the both-given refusal below
// fires on a real contradiction rather than on a default. It returns the
// resolved text and the flag name it arrived under, in that order.
func criterionFlag(fs *flag.FlagSet, text, criterion string) (string, string, error) {
	var gotText, gotCriterion bool
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case criterionName:
			gotText = true
		case criterionAlias:
			gotCriterion = true
		}
	})
	switch {
	case gotText && gotCriterion:
		if text != criterion {
			return "", "", refuse("usage",
				"criteria add: --%s and --%s are one flag under two names, "+
					"and they were given different values; supply either one",
				criterionName, criterionAlias)
		}
		return text, "--" + criterionName, nil
	case gotCriterion:
		return criterion, "--" + criterionAlias, nil
	default:
		return text, "--" + criterionName, nil
	}
}

// criteriaAdd takes the resolved text and the flag name it arrived under, so
// the §8.1 control-character refusal quotes the spelling the operator typed
// rather than the one the code prefers.
func criteriaAdd(e *cli.Env, slug, text, flagName string) error {
	if text == "" {
		// Names both spellings (§6.2): the reader who reached an empty
		// `criteria add` from the corpus side is looking for `--criterion`,
		// and a refusal that mentions only `--text` sends them guessing a
		// third name.
		return refuse("usage",
			"criteria add requires --%s (or --%s, the same flag under the corpus's "+
				"historical spelling)", criterionName, criterionAlias)
	}
	if err := validateText(flagName, text); err != nil {
		return err
	}
	err := Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		ctx := context.Background()
		if err := refuseTerminalEpic(ctx, tx, slug, "criteria add"); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO epic_criteria (epic, seq, criterion)
			VALUES (?1, COALESCE((SELECT MAX(seq) FROM epic_criteria WHERE epic = ?1), 0) + 1, ?2)`,
			slug, text); err != nil {
			return nil, err //nolint:wrapcheck // FK and CHECK speak through the mapper
		}
		return []Event{{Entity: epicPrefix + slug, Event: evCriteria, Detail: "add: " + text}}, nil
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "criterion added to epic:%s\n", slug)
	return nil
}

// criteriaMet is for owner-attested (non-runnable) criteria only —
// recorded prose is never presented as a machine-verified pass (§5.4).
func criteriaMet(e *cli.Env, slug, seqTok, evidence string) error {
	seq, err := strconv.ParseInt(seqTok, 10, 64)
	if err != nil {
		return refuse("usage", "%q is not a criterion seq", seqTok)
	}
	if evidence == "" {
		return refuse("usage", "criteria met requires --evidence")
	}
	if err := validateText("--evidence", evidence); err != nil {
		return err
	}
	err = Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		ctx := context.Background()
		if err := refuseTerminalEpic(ctx, tx, slug, "criteria met"); err != nil {
			return nil, err
		}
		var criterion string
		err := tx.QueryRowContext(ctx,
			`SELECT criterion FROM epic_criteria WHERE epic = ? AND seq = ?`, slug, seq).Scan(&criterion)
		if err != nil {
			return nil, refuse("not-found", "no criterion %d on epic %q", seq, slug)
		}
		if _, runnable := runnableCommand(criterion); runnable {
			return nil, refuse("runnable", "criterion %d is runnable; `criteria check` is its only judge", seq)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE epic_criteria SET met = 1, evidence = ? WHERE epic = ? AND seq = ?`,
			evidence, slug, seq); err != nil {
			return nil, err //nolint:wrapcheck // constraint codes ride to the mapper
		}
		return []Event{{
			Entity: epicPrefix + slug, Event: evCriteria,
			Detail: fmt.Sprintf("met %d: %s", seq, evidence),
		}}, nil
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "criterion %d met\n", seq)
	return nil
}

// criteriaCheck executes the runnable criteria standalone; the same
// engine runs inside epic close's condition (3).
func criteriaCheck(e *cli.Env, slug string) error {
	var output []string
	var failed, reportOnly bool
	err := Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		ctx := context.Background()
		// On a terminal epic the check still RUNS — "does this closed
		// epic's criterion still hold?" is a legitimate question — but
		// records nothing: persisting would overwrite the acceptance
		// record the epic was closed on, which no verb can restore
		// (amendment `terminal-epics-refuse-reopening-writes`).
		terminal, _, err := epicIsTerminal(ctx, tx, slug)
		if err != nil {
			return nil, err
		}
		reportOnly = terminal
		// Standalone check: only a failed RUNNABLE criterion is a
		// failure; unmet owner-attested criteria belong to the epic
		// close's condition (3), not to this verb (§6.2, task #7).
		_, failedRuns, out, err := runCriteria(ctx, tx, slug, !terminal)
		if err != nil {
			return nil, err
		}
		output = out
		failed = len(failedRuns) > 0
		return []Event{{
			Entity: epicPrefix + slug, Event: evCriteria,
			Detail: fmt.Sprintf("check: %d line(s), failed=%v", len(out), failed),
		}}, nil
	})
	if err != nil {
		return err
	}
	for _, l := range output {
		_, _ = fmt.Fprintln(e.Stdout, l)
	}
	if reportOnly {
		_, _ = fmt.Fprintf(e.Stdout,
			"epic:%s is terminal: results reported, not recorded\n", slug)
	}
	if failed {
		return refuse("criteria", "a runnable criterion failed")
	}
	return nil
}

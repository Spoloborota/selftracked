package verify

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/Spoloborota/selftracked/internal/cli"
	"github.com/Spoloborota/selftracked/internal/rules"
)

// Verb returns the §6.2 `verify [--quiet] [--fast]` catalog entry. Its
// contract is a stub cross-reference to §7 (INV-272); the rules live here.
func Verb() cli.Verb {
	var quiet, fast bool
	return cli.Verb{
		Name: "verify",
		Subs: []cli.Sub{{
			Arity: 0,
			Usage: "verify [--quiet] [--fast] [--json]",
			Flags: func(fs *flag.FlagSet) {
				fs.BoolVar(&quiet, "quiet", false, "suppress the report; exit code only")
				fs.BoolVar(&fast, "fast", false, "pre-commit partition: pure-SQL rules + R15 only")
			},
			Run: func(e *cli.Env, _ []string, _ *flag.FlagSet) error {
				return run(e, quiet, fast)
			},
		}},
	}
}

// run executes verify and maps its outcome to the §6.1 exit contract: a
// non-empty red set is a refusal (exit 1); advisory findings never change
// the exit; a missing tracker is a refusal; anything else is infrastructure.
func run(e *cli.Env, quiet, fast bool) error {
	rep, err := Run(context.Background(), instanceDir, fast)
	if err != nil {
		var nf *notFoundError
		if errors.As(err, &nf) {
			return &cli.CodedError{Code: "not-found", Message: nf.Error(), Status: 1}
		}
		return err
	}
	switch {
	case quiet:
		emitQuietAdvisory(e.Stderr, rep)
	case e.JSON:
		emitJSON(e.Stdout, rep)
	default:
		emitText(e.Stdout, rep)
	}
	if len(rep.Red) > 0 {
		return &cli.CodedError{
			Code:    "verify",
			Message: fmt.Sprintf("%d integrity violation(s)", len(rep.Red)),
			Status:  1,
		}
	}
	return nil
}

// emitQuietAdvisory is `--quiet`'s ONE concession (§7; amendment
// `r10-sees-the-window-it-was-meant-to-watch`). The pre-commit hook runs
// `verify --fast --quiet`, and advisory findings never touch the exit code
// — so without this line every advisory computed at the commit boundary is
// discarded, which made moving R10 into `--fast` inert.
//
// It prints exactly one line, to STDERR (stdout stays byte-empty under
// `--quiet`, which callers parse), naming the RULES and not their
// instances: a repository with dozens of advisory findings pays one line
// per commit, not dozens. It stays silent in the two cases where it would
// be noise or a duplicate: no advisory findings at all, and a red run —
// the red path already speaks, through the report and the exit code. The
// report body stays suppressed and the exit contract is untouched.
func emitQuietAdvisory(w io.Writer, rep Report) {
	if w == nil || len(rep.Red) > 0 || len(rep.Advisory) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "verify: advisory %s (run 'selftracked verify' for detail)\n",
		strings.Join(advisoryRuleNames(rep.Advisory), ", "))
}

// advisoryRuleNames is the distinct rule names of vs in RULE-NUMBER order,
// not in the order the engine happened to run them: the fast partition
// emits R15 before R10, so first-seen order would print "R15, R10" and vary
// with an unrelated reordering of stage1. Names that are not `R<n>` sort
// after the numbered ones, lexically — no advisory rule has that shape
// today, and a silent mis-sort is a worse answer than a defined one.
func advisoryRuleNames(vs []rules.Violation) []string {
	seen := make(map[string]bool, len(vs))
	names := make([]string, 0, len(vs))
	for _, v := range vs {
		if !seen[v.Rule] {
			seen[v.Rule] = true
			names = append(names, v.Rule)
		}
	}
	slices.SortFunc(names, func(a, b string) int {
		na, oka := ruleNumber(a)
		nb, okb := ruleNumber(b)
		switch {
		case oka && okb:
			if c := cmp.Compare(na, nb); c != 0 {
				return c
			}
		case oka != okb:
			if oka {
				return -1
			}
			return 1
		}
		return strings.Compare(a, b)
	})
	return names
}

// ruleNumber parses the `R<n>` rule-name shape, reporting whether it held.
func ruleNumber(name string) (int, bool) {
	rest, ok := strings.CutPrefix(name, "R")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	if err != nil {
		return 0, false
	}
	return n, true
}

func mode(fast bool) string {
	if fast {
		return "--fast"
	}
	return "full"
}

func emitText(w io.Writer, rep Report) {
	for _, v := range rep.Red {
		_, _ = fmt.Fprintf(w, "FAIL %s: %s\n", v.Rule, v.Message)
	}
	for _, v := range rep.Advisory {
		_, _ = fmt.Fprintf(w, "warn %s: %s\n", v.Rule, v.Message)
	}
	_, _ = fmt.Fprintf(w, "verify %s: %d violation(s), %d advisory\n", mode(rep.Fast), len(rep.Red), len(rep.Advisory))
}

// jsonFinding is verify's stable machine shape — rules.Violation has no
// json tags, and a severity field the caller can filter on is worth the
// small translation.
type jsonFinding struct {
	Rule     string `json:"rule"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

func emitJSON(w io.Writer, rep Report) {
	out := struct {
		Fast     bool          `json:"fast"`
		Red      []jsonFinding `json:"red"`
		Advisory []jsonFinding `json:"advisory"`
	}{Fast: rep.Fast, Red: findings(rep.Red, "red"), Advisory: findings(rep.Advisory, "advisory")}
	b, err := json.Marshal(out)
	if err != nil {
		// Unreachable for this closed struct; the exit code still speaks.
		return
	}
	_, _ = fmt.Fprintf(w, "%s\n", b)
}

func findings(vs []rules.Violation, severity string) []jsonFinding {
	out := make([]jsonFinding, 0, len(vs))
	for _, v := range vs {
		out = append(out, jsonFinding{Rule: v.Rule, Message: v.Message, Severity: severity})
	}
	return out
}

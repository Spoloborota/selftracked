package verify

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

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
	if !quiet {
		if e.JSON {
			emitJSON(e.Stdout, rep)
		} else {
			emitText(e.Stdout, rep)
		}
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

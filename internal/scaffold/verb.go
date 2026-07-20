package scaffold

import (
	"context"
	"errors"
	"flag"
	"fmt"

	"github.com/Spoloborota/selftracked/internal/cli"
)

// Verb returns the §6.2 `init [--force]` catalog entry (full contract §9).
func Verb() cli.Verb {
	var force bool
	return cli.Verb{
		Name: "init",
		Subs: []cli.Sub{{
			Arity: 0,
			Usage: "init [--force] [--json]",
			Flags: func(fs *flag.FlagSet) {
				fs.BoolVar(&force, "force", false, "reinitialize over an existing tracker")
			},
			Run: func(e *cli.Env, _ []string, _ *flag.FlagSet) error {
				return run(e, force)
			},
		}},
	}
}

func run(e *cli.Env, force bool) error {
	ctx := context.Background()
	if err := writeScaffold(ctx, ".", force); err != nil {
		var ex *existsError
		if errors.As(err, &ex) {
			return &cli.CodedError{Code: "exists", Message: ex.Error(), Status: 1}
		}
		var clone *cloneError
		if errors.As(err, &clone) {
			return &cli.CodedError{Code: "clone", Message: clone.Error(), Status: 1}
		}
		return err
	}
	_, _ = fmt.Fprintln(e.Stdout, "initialized selftracked in .selftracked/ — run selftracked verify")
	_, _ = fmt.Fprintln(e.Stdout, hookActivation(ctx, "."))
	return nil
}

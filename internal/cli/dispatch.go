package cli

import (
	"errors"
	"flag"
	"fmt"
	"strings"
)

// Run dispatches one invocation and returns the process exit code. It owns
// every §3.2 parsing obligation:
//
//	(a) arity checked in both directions before positionals are popped;
//	    leftover tokens after flag parsing refuse instead of dropping;
//	(b) flag parse errors become the JSON error object, exit 2;
//	(c) -h/--help prints the usage line to stdout and exits 0;
//	and the leading-dash rule: a -token is never accepted as a positional.
func Run(r *Registry, e *Env, args []string) int {
	err := dispatch(r, e, args)
	if err == nil {
		return 0
	}
	var coded *CodedError
	if errors.As(err, &coded) {
		emitError(e.Stderr, coded.Code, coded.Message)
		return coded.Status
	}
	// A verb error without classification goes through the §6.1 mapper:
	// constraint-family refusals exit 1, infrastructure exits 2.
	status, errCode := classify(err)
	emitError(e.Stderr, errCode, err.Error())
	return status
}

const (
	helpShort = "-h"
	helpLong  = "--help"
)

func isHelpToken(tok string) bool { return tok == helpShort || tok == helpLong }

func dispatch(r *Registry, e *Env, args []string) error {
	verb, rest, done, err := resolveVerb(r, e, args)
	if done || err != nil {
		return err
	}
	sub, rest, done, err := resolveSub(e, verb, rest)
	if done || err != nil {
		return err
	}
	pos, remainder, done, err := splitPositionals(e, verb, sub, rest)
	if done || err != nil {
		return err
	}

	fs, done, err := parseFlags(e, verb, sub, remainder)
	if done || err != nil {
		return err
	}

	// §6.1: every verb begins with the version gate. At S2 it is a stub
	// with the final shape (S11 fills in §8.6's migration escalation).
	if err := versionGate(); err != nil {
		return err
	}

	return sub.Run(e, pos, fs)
}

// parseFlags runs §3.2 (b) and (c) plus (a)'s back half: parse errors are
// the JSON object, ErrHelp is stdout usage, and leftover tokens refuse —
// stdlib parks them in Args() without complaint, so silence would drop
// them.
func parseFlags(e *Env, verb *Verb, sub *Sub, remainder []string) (*flag.FlagSet, bool, error) {
	fs := flagSet(verb.Name, sub.Name, sub, &e.JSON)
	if err := fs.Parse(remainder); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printSubUsage(e, sub)
			return nil, true, nil
		}
		return nil, false, Usage("%s: %v", name(verb, sub), err)
	}
	if extra := fs.Args(); len(extra) > 0 {
		return nil, false, Usage("%s: unexpected argument %q; usage: %s", name(verb, sub), extra[0], sub.Usage)
	}
	return fs, false, nil
}

// resolveVerb finds the verb, serving top-level help and refusing unknown
// or absent tokens. done=true means help was served.
func resolveVerb(r *Registry, e *Env, args []string) (*Verb, []string, bool, error) {
	if len(args) == 0 {
		return nil, nil, false, Usage("a verb is required; one of: %s", strings.Join(r.Names(), " "))
	}
	if isHelpToken(args[0]) || args[0] == "help" {
		printTopUsage(e, r)
		return nil, nil, true, nil
	}
	verb, ok := r.verbs[args[0]]
	if !ok {
		return nil, nil, false, Usage("unknown verb %q; one of: %s", args[0], strings.Join(r.Names(), " "))
	}
	return verb, args[1:], false, nil
}

// resolveSub pops the subverb token BEFORE positionals (§6.2). done=true
// means help was served and dispatch is complete.
func resolveSub(e *Env, verb *Verb, rest []string) (*Sub, []string, bool, error) {
	sub := &verb.Subs[0]
	if sub.Name == "" {
		return sub, rest, false, nil
	}
	if len(rest) == 0 || strings.HasPrefix(rest[0], "-") {
		// -h on a two-level verb without a subverb still deserves the
		// usage lines, not a JSON refusal.
		if len(rest) > 0 && isHelpToken(rest[0]) {
			printVerbUsage(e, verb)
			return nil, nil, true, nil
		}
		return nil, nil, false, Usage("%s needs a subverb; one of: %s", verb.Name, subNames(verb))
	}
	for i := range verb.Subs {
		if verb.Subs[i].Name == rest[0] {
			return &verb.Subs[i], rest[1:], false, nil
		}
	}
	return nil, nil, false, Usage("unknown %s subverb %q; one of: %s", verb.Name, rest[0], subNames(verb))
}

// splitPositionals enforces §3.2 (a)'s front half and the leading-dash
// rule: count the leading tokens that can legally be positionals — no
// positional domain (ids, slugs, S-ids, statuses, refs, roots) admits a
// leading dash — and refuse in both short directions before popping.
func splitPositionals(e *Env, verb *Verb, sub *Sub, rest []string) ([]string, []string, bool, error) {
	available := 0
	for available < len(rest) && !strings.HasPrefix(rest[available], "-") {
		available++
	}
	if available >= sub.Arity {
		return rest[:sub.Arity], rest[sub.Arity:], false, nil
	}
	if len(rest) > available {
		if isHelpToken(rest[available]) {
			printSubUsage(e, sub)
			return nil, nil, true, nil
		}
		return nil, nil, false, Usage("%s: %q cannot be a positional (positionals never start with '-'); usage: %s",
			name(verb, sub), rest[available], sub.Usage)
	}
	return nil, nil, false, Usage("%s expects %d positional(s), got %d; usage: %s",
		name(verb, sub), sub.Arity, available, sub.Usage)
}

func name(v *Verb, s *Sub) string {
	if s.Name == "" {
		return v.Name
	}
	return v.Name + " " + s.Name
}

func subNames(v *Verb) string {
	names := make([]string, len(v.Subs))
	for i := range v.Subs {
		names[i] = v.Subs[i].Name
	}
	return strings.Join(names, " ")
}

func printTopUsage(e *Env, r *Registry) {
	_, _ = fmt.Fprintf(e.Stdout, "usage: selftracked <verb> [args]\nverbs: %s\n", strings.Join(r.Names(), " "))
}

func printVerbUsage(e *Env, v *Verb) {
	for i := range v.Subs {
		_, _ = fmt.Fprintf(e.Stdout, "usage: %s\n", v.Subs[i].Usage)
	}
}

func printSubUsage(e *Env, s *Sub) {
	_, _ = fmt.Fprintf(e.Stdout, "usage: %s\n", s.Usage)
}

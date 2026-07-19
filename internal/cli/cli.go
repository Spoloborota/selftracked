// Package cli is the dispatcher: the closed verb registry, the §3.2
// parsing obligations, the §6.1 exit contract and JSON error shape. Verbs
// register here; nothing here knows what any verb does. The registry is the
// "closed set" of §1: an agent gets exactly the verbs it declares, and a
// token that is not one of them is a usage refusal, never a guess.
package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Env is where a verb's output goes and where it runs. Tests substitute
// buffers; main passes the process's own streams.
type Env struct {
	Stdout io.Writer
	Stderr io.Writer
	// JSON reports whether --json was set; the registry wires the flag on
	// every verb (§6.1: "--json on every verb") and dispatch fills this in
	// before the verb runs.
	JSON bool
}

// Sub is one runnable (verb, subverb) pair. A verb without subverbs
// registers exactly one Sub with an empty Name.
type Sub struct {
	Name string
	// Arity is the statically known positional count (§6.2) — checked in
	// both directions before anything is popped.
	Arity int
	// Usage is the one-line signature printed for -h and usage errors.
	Usage string
	// Flags declares the sub's own flags; the registry adds --json itself.
	Flags func(fs *flag.FlagSet)
	// Run receives exactly Arity positionals and the parsed FlagSet.
	Run func(e *Env, pos []string, fs *flag.FlagSet) error
}

// Verb is a named entry in the closed set.
type Verb struct {
	Name string
	Subs []Sub
}

// Registry is the closed verb set. The zero value is empty and usable;
// main installs the catalog, tests install fixtures.
type Registry struct {
	verbs map[string]*Verb
}

// ErrRegistration wraps every malformed registration: these are
// programmer errors caught at startup, not runtime conditions.
var ErrRegistration = errors.New("invalid verb registration")

// Register adds a verb. Registering is how a verb enters the closed set,
// and the registry — not the verb — owns the --json flag, so a registered
// verb CANNOT lack it: the §6.1 "--json on every verb" guarantee is
// structural, not reviewed.
func (r *Registry) Register(v Verb) error {
	if r.verbs == nil {
		r.verbs = map[string]*Verb{}
	}
	if v.Name == "" || strings.HasPrefix(v.Name, "-") {
		return fmt.Errorf("%w: %q needs a plain name", ErrRegistration, v.Name)
	}
	if _, dup := r.verbs[v.Name]; dup {
		return fmt.Errorf("%w: %q already registered", ErrRegistration, v.Name)
	}
	if len(v.Subs) == 0 {
		return fmt.Errorf("%w: %q needs at least one runnable form", ErrRegistration, v.Name)
	}
	named := v.Subs[0].Name != ""
	for i := range v.Subs {
		s := &v.Subs[i]
		if (s.Name != "") != named {
			return fmt.Errorf("%w: %q mixes named and unnamed forms", ErrRegistration, v.Name)
		}
		if s.Arity < 0 || s.Run == nil {
			return fmt.Errorf("%w: %q %q needs an arity and a Run", ErrRegistration, v.Name, s.Name)
		}
	}
	r.verbs[v.Name] = &v
	return nil
}

// Names returns the registered verb names, sorted, for usage output.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.verbs))
	for n := range r.verbs {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Verbs returns every registered verb, for structural tests: the
// registry's guarantees are only guarantees if a test can enumerate what
// carries them.
func (r *Registry) Verbs() []*Verb {
	verbs := make([]*Verb, 0, len(r.verbs))
	for _, n := range r.Names() {
		verbs = append(verbs, r.verbs[n])
	}
	return verbs
}

// flagSet builds the FlagSet for one sub exactly as §3.2 (b) requires:
// ContinueOnError so the JSON contract owns the exit, output discarded and
// Usage suppressed because the package's own prints bypass both the JSON
// shape and the discarded writer otherwise.
func flagSet(verb, sub string, s *Sub, jsonOut *bool) *flag.FlagSet {
	name := verb
	if sub != "" {
		name += " " + sub
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	fs.BoolVar(jsonOut, "json", false, "machine-readable output")
	if s.Flags != nil {
		s.Flags(fs)
	}
	return fs
}

// jsonError is the §6.1 error shape: {"error":{"code","message"}}.
type jsonError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// emitError writes the one error shape the contract allows. Dispatcher
// errors are always the JSON object: the callers that must parse a refusal
// are agents, and two formats would mean guessing which one came back.
func emitError(w io.Writer, code, message string) {
	var e jsonError
	e.Error.Code = code
	e.Error.Message = message
	b, err := json.Marshal(e)
	if err != nil {
		// Unreachable for a two-string struct; the contract still holds.
		b = []byte(`{"error":{"code":"internal","message":"error encoding failed"}}`)
	}
	_, _ = fmt.Fprintf(w, "%s\n", b)
}

// CodedError carries its §6.1 classification. Verbs return it when they
// refuse; the mapper turns everything else into the right exit by
// inspecting driver codes.
type CodedError struct {
	Code    string
	Message string
	Status  int // 1 refusal, 2 environment/infrastructure
}

func (c *CodedError) Error() string { return c.Message }

// Usage builds the exit-2 usage refusal the dispatcher emits for every
// shape violation: wrong arity, unknown verb, leftover tokens, a flag
// token where a positional belongs.
func Usage(format string, args ...any) *CodedError {
	return &CodedError{Code: "usage", Message: fmt.Sprintf(format, args...), Status: exitInfra}
}

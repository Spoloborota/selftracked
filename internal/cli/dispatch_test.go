package cli_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"strings"
	"testing"

	"github.com/Spoloborota/selftracked/internal/cli"
)

// fixtureRegistry builds the dispatcher-exercise registry the tests and
// the testscript harness share. These verbs exist ONLY in the test binary;
// the shipped registry stays exactly the §6.2 catalog (empty until S5a).
func fixtureRegistry() *cli.Registry {
	r := &cli.Registry{}
	// Registration fails only on a programmer error in this fixture, and
	// the harness has no *testing.T — a panic is the honest report.
	must := func(err error) {
		if err != nil {
			panic(err)
		}
	}
	must(r.Register(cli.Verb{
		Name: "probe",
		Subs: []cli.Sub{{
			Arity: 2,
			Usage: "probe A B [--note N] [--json]",
			Flags: func(fs *flag.FlagSet) { fs.String("note", "", "a note") },
			Run: func(e *cli.Env, pos []string, fs *flag.FlagSet) error {
				_, _ = fmt.Fprintf(e.Stdout, "probe %s %s note=%s json=%v\n",
					pos[0], pos[1], fs.Lookup("note").Value, e.JSON)
				return nil
			},
		}},
	}))
	must(r.Register(cli.Verb{
		Name: "duo",
		Subs: []cli.Sub{
			{
				Name: "add", Arity: 1, Usage: "duo add X [--json]",
				Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
					_, _ = fmt.Fprintf(e.Stdout, "added %s\n", pos[0])
					return nil
				},
			},
			{
				Name: "list", Arity: 0, Usage: "duo list [--json]",
				Run: func(e *cli.Env, _ []string, _ *flag.FlagSet) error {
					_, _ = fmt.Fprintln(e.Stdout, "listed")
					return nil
				},
			},
		},
	}))
	return r
}

// runCLI runs one invocation against the fixture registry and returns
// exit code, stdout, stderr.
func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	e := &cli.Env{Stdout: &out, Stderr: &errb}
	code := cli.Run(fixtureRegistry(), e, args)
	return code, out.String(), errb.String()
}

// wantUsage asserts the §6.1 refusal shape: exit 2, stderr carrying
// exactly the JSON error object with code "usage", stdout silent.
func wantUsage(t *testing.T, args ...string) string {
	t.Helper()
	code, out, errb := runCLI(t, args...)
	if code != 2 {
		t.Fatalf("%v: exit %d; want 2", args, code)
	}
	if out != "" {
		t.Fatalf("%v: stdout %q; a refusal writes nothing there", args, out)
	}
	var e struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(errb), &e); err != nil {
		t.Fatalf("%v: stderr %q is not the JSON error object: %v", args, errb, err)
	}
	if e.Error.Code != "usage" {
		t.Fatalf("%v: error code %q; want usage", args, e.Error.Code)
	}
	if e.Error.Message == "" {
		t.Fatalf("%v: empty error message", args)
	}
	return e.Error.Message
}

func TestDispatchUsageMatrix(t *testing.T) {
	t.Parallel()

	t.Run("no-args-refused", func(t *testing.T) {
		t.Parallel()
		wantUsage(t)
	})
	t.Run("unknown-verb-refused", func(t *testing.T) {
		t.Parallel()
		msg := wantUsage(t, "nosuchverb")
		if !strings.Contains(msg, "nosuchverb") {
			t.Fatalf("message %q does not name the offending token", msg)
		}
	})
	t.Run("arity-too-few-refused-not-panicked", func(t *testing.T) {
		t.Parallel()
		msg := wantUsage(t, "probe", "onlyone")
		if !strings.Contains(msg, "expects 2") {
			t.Fatalf("message %q does not state the arity", msg)
		}
	})
	t.Run("leftover-args-refused-never-dropped", func(t *testing.T) {
		t.Parallel()
		msg := wantUsage(t, "probe", "a", "b", "extra")
		if !strings.Contains(msg, "extra") {
			t.Fatalf("message %q does not name the leftover token", msg)
		}
	})
	t.Run("leftover-after-flags-refused", func(t *testing.T) {
		t.Parallel()
		wantUsage(t, "probe", "a", "b", "--note", "n", "trailing")
	})
	t.Run("leading-dash-positional-refused", func(t *testing.T) {
		t.Parallel()
		// `probe --note x a` — flags mistakenly first must refuse, never
		// pop "--note" and "x" as the two positionals.
		msg := wantUsage(t, "probe", "--note", "x", "a")
		if !strings.Contains(msg, "--note") {
			t.Fatalf("message %q does not name the dash token", msg)
		}
	})
	t.Run("flag-parse-error-is-json-exit-2", func(t *testing.T) {
		t.Parallel()
		wantUsage(t, "probe", "a", "b", "--bogus")
	})
	t.Run("double-dash-honored-as-terminator", func(t *testing.T) {
		t.Parallel()
		// After the positionals, a literal -- ends flag parsing; what
		// follows is a leftover and must refuse, not silently vanish.
		wantUsage(t, "probe", "a", "b", "--", "sneaky")
	})
	t.Run("unknown-subverb-refused", func(t *testing.T) {
		t.Parallel()
		wantUsage(t, "duo", "nope")
	})
	t.Run("missing-subverb-refused", func(t *testing.T) {
		t.Parallel()
		wantUsage(t, "duo")
	})
}

func TestDispatchRunsAndOrder(t *testing.T) {
	t.Parallel()

	t.Run("subverb-pop-order", func(t *testing.T) {
		t.Parallel()
		// The subverb token is popped before positionals: "add" dispatches,
		// "x" is the positional — not the other way around.
		code, out, errb := runCLI(t, "duo", "add", "x")
		if code != 0 || errb != "" {
			t.Fatalf("exit %d stderr %q; want clean success", code, errb)
		}
		if out != "added x\n" {
			t.Fatalf("stdout %q", out)
		}
	})
	t.Run("subverb-arity-check", func(t *testing.T) {
		t.Parallel()
		code, out, _ := runCLI(t, "duo", "list")
		if code != 0 || out != "listed\n" {
			t.Fatalf("exit %d stdout %q", code, out)
		}
		wantUsage(t, "duo", "list", "surplus")
		wantUsage(t, "duo", "add")
	})
	t.Run("json-flag-reaches-the-verb", func(t *testing.T) {
		t.Parallel()
		code, out, _ := runCLI(t, "probe", "a", "b", "--json")
		if code != 0 || !strings.Contains(out, "json=true") {
			t.Fatalf("exit %d stdout %q; --json did not reach the verb", code, out)
		}
	})
}

func TestHelpIsStdoutUsageExit0(t *testing.T) {
	t.Parallel()

	// The bare word "help" is deliberately NOT special-cased: §3.2 (c)
	// names -h/--help only, and a reserved word would silently shadow any
	// future verb of that name (found by the S2 close review).
	wantUsage(t, "help")

	cases := [][]string{
		{"-h"},
		{"--help"},
		{"probe", "-h"},
		{"probe", "a", "b", "-h"},
		{"duo", "-h"},
	}
	for _, args := range cases {
		code, out, errb := runCLI(t, args...)
		if code != 0 {
			t.Errorf("%v: exit %d; help is not an error", args, code)
		}
		if errb != "" {
			t.Errorf("%v: stderr %q; help writes to stdout", args, errb)
		}
		if !strings.Contains(out, "usage") && !strings.Contains(out, "verbs:") {
			t.Errorf("%v: stdout %q does not look like usage", args, out)
		}
	}
}

// TestEveryRegisteredVerbHasJSON is the structural §6.1 guarantee: the
// registry wires --json, so no registered verb — present or future — can
// lack it. Enumerating the registry is what makes this a property of the
// mechanism rather than a per-verb review item.
func TestEveryRegisteredVerbHasJSON(t *testing.T) {
	t.Parallel()

	r := fixtureRegistry()
	for _, v := range r.Verbs() {
		for i := range v.Subs {
			var e cli.Env
			fs := cli.FlagSetForTest(v, &v.Subs[i], &e.JSON)
			if fs.Lookup("json") == nil {
				t.Errorf("%s %s: no --json flag", v.Name, v.Subs[i].Name)
			}
		}
	}
}

// TestSiblingSubverbElision checks the declaration mechanism §6.2's
// elision rule rides on: every sub declares its own statically known
// arity, the registry exposes them for audit, and a sibling set shaped
// like the catalog's (`epic …` — every sub takes SLUG except `list`)
// expresses that shape directly. Per-verb conformance with the printed
// catalog re-proves at each verb stage's close; this pins the mechanism.
func TestSiblingSubverbElision(t *testing.T) {
	t.Parallel()

	r := fixtureRegistry()
	var duo *cli.Verb
	for _, v := range r.Verbs() {
		if v.Name == "duo" {
			duo = v
		}
	}
	if duo == nil {
		t.Fatal("fixture verb duo not registered")
	}
	arities := map[string]int{}
	for i := range duo.Subs {
		arities[duo.Subs[i].Name] = duo.Subs[i].Arity
	}
	if arities["add"] != 1 || arities["list"] != 0 {
		t.Fatalf("declared arities %v; the elision exception (list takes none) must be declarable per sub", arities)
	}
}

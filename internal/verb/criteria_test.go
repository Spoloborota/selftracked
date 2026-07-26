package verb

import (
	"flag"
	"io"
	"strings"
	"testing"

	"github.com/Spoloborota/selftracked/internal/cli"
)

// criteriaAddFlagSet builds the FlagSet `criteria add` REALLY runs on: the
// declarations come from `CriteriaVerbs()`'s own `Flags` function, the way
// the dispatcher builds them (`flagSet`, internal/cli/cli.go — same
// ContinueOnError set, output discarded). A test that declared the two flags
// itself would stay green with `--criterion` undeclared in the catalog,
// which is exactly the registration regression worth catching here rather
// than only in the txtar fixture.
func criteriaAddFlagSet(t *testing.T) *flag.FlagSet {
	t.Helper()
	var sub *cli.Sub
	for _, v := range CriteriaVerbs() {
		if v.Name != evCriteria {
			continue
		}
		for i := range v.Subs {
			if v.Subs[i].Name == subAdd {
				sub = &v.Subs[i]
			}
		}
	}
	if sub == nil {
		t.Fatal("CriteriaVerbs() no longer declares a `criteria add` sub")
	}
	fs := flag.NewFlagSet("criteria add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if sub.Flags == nil {
		t.Fatal("`criteria add` declares no flags at all")
	}
	sub.Flags(fs)
	return fs
}

// criterionFlagValue reads a declared flag's current value off the FlagSet.
// It is how the test reaches the variables `CriteriaVerbs()` binds inside
// its own closure: an undeclared flag has no Lookup entry, and that is the
// regression, reported as a failure rather than as a nil dereference.
func criterionFlagValue(t *testing.T, fs *flag.FlagSet, name string) string {
	t.Helper()
	f := fs.Lookup(name)
	if f == nil {
		t.Fatalf("`criteria add` does not declare --%s; §6.2 makes both spellings one accepted flag", name)
	}
	return f.Value.String()
}

// TestCriterionFlagResolution pins `criteria add`'s two spellings of one flag
// (§6.2, amendment `one-name-for-the-criterion-field`) at the resolution
// itself, where the txtar fixture cannot see the flag NAME that comes back —
// and the name is load-bearing: it is what the §8.1 control-character
// refusal quotes, so an operator who typed `--criterion` is not told their
// `--text` is malformed.
//
// The parse is driven through the catalog's own FlagSet rather than by
// setting the two strings directly, for two reasons: what distinguishes an
// absent flag from one given an empty value is `fs.Visit`, which only a
// parse populates; and the declarations under test are the registered ones,
// so undeclaring either spelling fails here.
func TestCriterionFlagResolution(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		args      []string
		wantValue string
		wantFlag  string
		wantErr   string
	}{
		{"primary", []string{"--text", "a criterion"}, "a criterion", "--text", ""},
		{"alias", []string{"--criterion", "a criterion"}, "a criterion", "--criterion", ""},
		{"neither", nil, "", "--text", ""},
		{"both-same", []string{"--text", "same", "--criterion", "same"}, "same", "--text", ""},
		{"both-differ", []string{"--text", "one", "--criterion", "other"}, "", "", "different values"},
		// The empty-versus-absent pair, which is the whole reason the
		// resolution reads fs.Visit and not the strings: `--text ''` is a
		// value that contradicts the alias, while an omitted `--text` is not.
		{"empty-primary-contradicts", []string{"--text", "", "--criterion", "x"}, "", "", "different values"},
		{"empty-primary-alone", []string{"--text", ""}, "", "--text", ""},
		{"empty-alias-alone", []string{"--criterion", ""}, "", "--criterion", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fs := criteriaAddFlagSet(t)
			if err := fs.Parse(tc.args); err != nil {
				t.Fatalf("parse %v: %v", tc.args, err)
			}
			text := criterionFlagValue(t, fs, criterionName)
			criterion := criterionFlagValue(t, fs, criterionAlias)
			value, flagName, err := criterionFlag(fs, text, criterion)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("%v: want a refusal mentioning %q, got value=%q flag=%q err=%v",
						tc.args, tc.wantErr, value, flagName, err)
				}
				// A refusal must carry no usable value: a caller that ignored
				// the error would otherwise write one of the two claims.
				if value != "" {
					t.Fatalf("%v: a refused resolution must yield no value, got %q", tc.args, value)
				}
				// Both spellings are named, so the reader is not left guessing
				// which of the two the tool actually knows.
				for _, want := range []string{"--" + criterionName, "--" + criterionAlias} {
					if !strings.Contains(err.Error(), want) {
						t.Fatalf("%v: the refusal must name %s, got %q", tc.args, want, err.Error())
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("%v: unexpected refusal %v", tc.args, err)
			}
			if value != tc.wantValue || flagName != tc.wantFlag {
				t.Fatalf("%v: got (%q, %q), want (%q, %q)",
					tc.args, value, flagName, tc.wantValue, tc.wantFlag)
			}
		})
	}
}

// TestCriteriaAddUsageNamesBothSpellings pins the DoD clause directly: the
// surface that changed keeps a refusal naming the other spelling, so an
// adopter who learned `criterion` from the corpus is redirected rather than
// stopped. Asserted over the message a reader actually meets, and over both
// names, so a rewording that drops one fails here.
func TestCriteriaAddUsageNamesBothSpellings(t *testing.T) {
	t.Parallel()
	e := &cli.Env{Stdout: &strings.Builder{}, Stderr: &strings.Builder{}}
	err := criteriaAdd(e, "any-epic", "", "--"+criterionName)
	if err == nil {
		t.Fatal("criteria add with no criterion text must refuse")
	}
	msg := err.Error()
	for _, want := range []string{"--" + criterionName, "--" + criterionAlias} {
		if !strings.Contains(msg, want) {
			t.Fatalf("the empty-criterion refusal must name %s; got %q", want, msg)
		}
	}
}

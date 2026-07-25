package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Spoloborota/selftracked/internal/cli"
	"github.com/Spoloborota/selftracked/internal/dump"
	"github.com/Spoloborota/selftracked/internal/load"
	"github.com/Spoloborota/selftracked/internal/scaffold"
	"github.com/Spoloborota/selftracked/internal/verb"
	"github.com/Spoloborota/selftracked/internal/verify"
)

// TestGeneratedPromptListsEveryVerb keeps the generated PROMPT.md honest
// about the closed verb set. The catalog there is prose, so it drifts
// silently as stages add verbs — the pre-publication audit found `gate` and
// `import` missing from it, in a file every adopter reads as authoritative.
// Comparing the generated file against the registry the binary actually
// installs is what stops the next omission.
func TestGeneratedPromptListsEveryVerb(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	// The same catalog run() installs — a verb absent here does not exist.
	base := []cli.Verb{dump.Verb(), load.Verb(), verify.Verb(), scaffold.Verb()}
	catalog := make([]cli.Verb, 0, len(base)+len(verb.Verbs()))
	catalog = append(catalog, base...)
	catalog = append(catalog, verb.Verbs()...)

	reg := &cli.Registry{}
	for _, v := range catalog {
		if err := reg.Register(v); err != nil {
			t.Fatal(err)
		}
	}
	env := &cli.Env{Stdout: &discard{}, Stderr: &discard{}}
	if code := cli.Run(reg, env, []string{"init"}); code != 0 {
		t.Fatalf("init exited %d", code)
	}

	prompt, err := os.ReadFile(filepath.Join(dir, "PROMPT.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(prompt)
	var missing []string
	for _, v := range catalog {
		// A verb may appear bare (`prime`) or opening a signature
		// (`create --title T …`); both start with a backtick + name.
		if !strings.Contains(text, "`"+v.Name+"`") &&
			!strings.Contains(text, "`"+v.Name+" ") {
			missing = append(missing, v.Name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("PROMPT.md's verb catalog omits %v; it is generated, so fix the template", missing)
	}
}

type discard struct{}

func (*discard) Write(p []byte) (int, error) { return len(p), nil }

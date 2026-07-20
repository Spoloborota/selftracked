package scaffold

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGeneratedContentAssertions gives each review-class inventory row a
// concrete check: the required text is present in the generated file. The
// golden test pins the exact bytes; this test says WHY those bytes matter,
// row by row, so a paraphrase that still byte-matches an updated golden
// cannot silently drop a mandated sentence.
func TestGeneratedContentAssertions(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := writeScaffold(context.Background(), root, false); err != nil {
		t.Fatal(err)
	}
	// Whitespace-normalise so a match survives line-wrapping in the template.
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return strings.Join(strings.Fields(string(b)), " ")
	}
	norm := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	prompt := read("PROMPT.md")
	agents := read("AGENTS.md")
	work := read("work/README.md")
	skill := read(".claude/skills/selftracked/SKILL.md")
	rule := read(".claude/rules/selftracked.md")

	cases := []struct {
		inv, doc, want string
		text           string
	}{
		{"INV-409", "PROMPT.md", "the §11.2 rule pointer", "State changes only through verbs"},
		{"INV-409", "PROMPT.md", "the verb catalog", "`set-status`"},
		{"INV-410", "PROMPT.md", "the scope disclaimer", "authoring guidance in this generated file, not gated conventions"},
		{"INV-411", "PROMPT.md", "durable-doc rule 1", "Prose never duplicates DB-enumerable state"},
		{"INV-412", "PROMPT.md", "durable-doc rule 2 anchor", "as of dump <sha12>"},
		{"INV-412", "PROMPT.md", "rule 2 commit-first ordering", "commit first, anchor after"},
		{"INV-412", "PROMPT.md", "rule 2 sidecar caveat", "is NOT a shortcut for this digest"},
		{"INV-413", "PROMPT.md", "durable-doc rule 3", "come from the system clock"},
		{"INV-405", "PROMPT.md", "the CHANGELOG convention", "Keep a Changelog"},
		{"INV-407", "PROMPT.md", "the repack recommendation", "git repack"},
		{"INV-485", "PROMPT.md", "the privacy warning", "redaction tooling is deferred"},
		{"INV-471", "PROMPT.md", "state-only-through-verbs", "never hand-edit `dump.sql`"},
		{"INV-473", "PROMPT.md", "agents never answer PO", "never answered by an agent"},
		{"INV-474", "PROMPT.md", "the three-column PO pairing", "three status surfaces"},
		{"INV-244", "PROMPT.md", "block --reason for any blocker", "the tool for *any* blocker, not only owner questions"},
		{"INV-247", "PROMPT.md", "the PO: literal is locale-fixed", "deliberately locale-fixed"},
		{"INV-415", "AGENTS.md", "AGENTS points at prime", "selftracked prime --json"},
		{"INV-415", "AGENTS.md", "AGENTS points at the rule/verbs", "Change state only through verbs"},
		{"INV-404", "work/README.md", "opt-in class runbook", "`runbook`"},
		{"INV-404", "work/README.md", "opt-in class guide", "`guide`"},
		{"INV-404", "work/README.md", "opt-in class rfc", "`rfc`"},
		{"INV-404", "work/README.md", "opt-in class src", "`src`"},
		{"INV-404", "work/README.md", "opt-in class external", "`external`"},
		{"INV-406", "work/README.md", "the cleanup/gc note", "manual for now"},
		{"INV-476", "SKILL.md", "the loop's divergence stop-first", "stop and reconcile first"},
		{"INV-477", "SKILL.md", "backlog refinement re-prime", "re-`prime` between passes"},
		{"INV-478", "SKILL.md", "the drift rule", "`create` + park, one command"},
		{"INV-479", "SKILL.md", "the bookkeeping-commit rule", "bookkeeping commit"},
		{"INV-480", "SKILL.md", "the PO-absent branch", "never answer the PO"},
	}
	docs := map[string]string{
		"PROMPT.md": prompt, "AGENTS.md": agents, "work/README.md": work, "SKILL.md": skill,
	}
	for _, c := range cases {
		if !strings.Contains(docs[c.doc], norm(c.text)) {
			t.Errorf("%s (%s): %s — missing %q", c.inv, c.doc, c.want, c.text)
		}
	}
	// The rule file carries the no-raw-SQL and no-PO rules too (INV-471/473).
	for _, want := range []string{"run `sqlite3`", "answer a product-owner decision"} {
		if !strings.Contains(rule, norm(want)) {
			t.Errorf(".claude rule missing %q", want)
		}
	}
}

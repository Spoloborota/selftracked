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
	settings := read(".claude/settings.json")

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
		{"INV-412", "PROMPT.md", "rule 2 epoch-not-arithmetic honesty note", "pins the *epoch*, not the arithmetic"},
		{"INV-412", "PROMPT.md", "rule 2 unverifiable-forever rationale", "unverifiable forever"},
		{"INV-413", "PROMPT.md", "durable-doc rule 3", "come from the system clock"},
		{"INV-405", "PROMPT.md", "the CHANGELOG convention", "Keep a Changelog"},
		{"INV-407", "PROMPT.md", "the repack recommendation", "git repack"},
		{"INV-407", "PROMPT.md", "the repack RATIONALE", "one full blob per commit"},
		{"INV-472", "PROMPT.md", "the deny-list backstop", "deny-list entry for `sqlite3`"},
		{"INV-485", "PROMPT.md", "the privacy warning", "redaction tooling is deferred"},
		{"INV-471", "PROMPT.md", "state-only-through-verbs", "never hand-edit `dump.sql`"},
		{"INV-473", "PROMPT.md", "agents never answer PO", "never answered by an agent"},
		{"INV-474", "PROMPT.md", "the three-column PO pairing", "three status surfaces"},
		{"INV-244", "PROMPT.md", "block --reason for any blocker", "the tool for *any* blocker, not only owner questions"},
		{"INV-247", "PROMPT.md", "the PO: literal is locale-fixed", "deliberately locale-fixed"},
		{"#21", "PROMPT.md", "prime named as the session-start read", "Start every session with `prime`"},
		{"#21", "PROMPT.md", "explicit staging for the bookkeeping commit", "git add .selftracked/dump.sql STATE.md"},
		{"#21", "PROMPT.md", "the empty-index rationale", "hook-staged content alone does not count"},
		{"#21", "PROMPT.md", "the task status vocabulary", "`DUPLICATE` (requires `--dup-of`"},
		{"#21", "PROMPT.md", "the story status vocabulary", "One `IN-PROGRESS` story per epic"},
		{"#21", "PROMPT.md", "the epic status vocabulary", "plus `PAUSED` and `DISSOLVED`"},
		{"#21", "PROMPT.md", "set-status rewrites the note", "call REWRITES the status note"},
		{"#21", "PROMPT.md", "verb signatures in the catalog", "set-status <id> <STATUS> [--note N] [--dup-of ID]"},
		{"#55", "PROMPT.md", "the task-versus-story section exists", "## Where new work goes"},
		{"#55", "PROMPT.md", "epic-goal work is a story", "advances an **ACTIVE epic's goal** is a **story**"},
		{"#55", "PROMPT.md", "the owner authorizes a story", "the owner authorizes it, and an implementing agent"},
		{"#55", "PROMPT.md", "everything else is a task", "anything standalone — is a **task**"},
		{"#55", "PROMPT.md", "unmatched work is named before the first write", "no existing task, story or epic"},
		{"#55", "PROMPT.md", "prime mechanizes the epic-scoped case", "`no-workable-story` notice; the rest is your own"},
		{"#55", "PROMPT.md", "the rule's honest force", "no verb refuses on it"},
		{"#55", "PROMPT.md", "unenforced is not optional", "unenforced is not optional"},
		{"#55", "PROMPT.md", "a self-classification never settles it", "never what settles it"},
		{"#55", "SKILL.md", "the loop branch for work outside ready[]", "**Work that did not come from `ready[]`**"},
		{"#55", "SKILL.md", "the branch points at the one rule statement", `PROMPT.md's "Where new work goes"`},
		{"#55", "SKILL.md", "the story branch stops for the owner", "**stops for the owner**"},
		{"#55", "SKILL.md", "the branch names the drift rule", "is the drift rule below"},
		{"#55", "SKILL.md", "the drift rule is scoped to work in progress", "discovered while a story is in progress"},
		{"#55", "SKILL.md", "the drift rule names the branch", "step 3's classification"},
		{"INV-415", "AGENTS.md", "AGENTS points at prime", "selftracked prime --json"},
		{"INV-415", "AGENTS.md", "AGENTS points at the rule/verbs", "Change state only through verbs"},
		{"INV-404", "work/README.md", "opt-in class runbook", "`runbook`"},
		{"INV-404", "work/README.md", "opt-in class guide", "`guide`"},
		{"INV-404", "work/README.md", "opt-in class rfc", "`rfc`"},
		{"INV-404", "work/README.md", "opt-in class src", "`src`"},
		{"INV-404", "work/README.md", "opt-in class external", "`external`"},
		{"INV-406", "work/README.md", "the cleanup/gc note", "manual for now"},
		{"INV-404", "work/README.md", "opt-in classes are NOT pre-registered", "**not** pre-registered"},
		{"INV-476", "SKILL.md", "the loop's divergence stop-first", "stop and reconcile first"},
		{"#25", "SKILL.md", "reconcile names load --force", "`selftracked load --force` replaces the local"},
		{"#25", "PROMPT.md", "divergence reconcile names load --force", "`load --force` replaces the local database"},
		{"#25", "PROMPT.md", "plain load refusal documented", "it refuses"},
		// #45: the recovery is one of two opposite, irreversible moves, so
		// each half of the decision is asserted separately — a text that
		// kept only the dump-side branch would still satisfy #25 above.
		{"#45", "PROMPT.md", "divergence has two directions", "two directions"},
		{"#45", "PROMPT.md", "the wrong branch is total loss", "total loss in\nexactly one of them"},
		{"#45", "PROMPT.md", "the git-log half of the test", "git log -1 --stat -- .selftracked/dump.sql"},
		{"#45", "PROMPT.md", "the git-status half of the test", "git status --short .selftracked/dump.sql"},
		{"#45", "PROMPT.md", "the rule that selects a branch", "the one holding\nwork that exists nowhere else"},
		{"#45", "PROMPT.md", "the dump-is-good branch is labelled", "The tracked dump is the good side"},
		{"#45", "PROMPT.md", "the database-is-good branch is labelled", "The local database is the good side"},
		{"#45", "PROMPT.md", "the database-side recovery discards nothing", "Discard nothing; re-derive"},
		{"#45", "PROMPT.md", "the database-side recovery re-derives the dump", "selftracked dump\nselftracked verify"},
		{"#45", "PROMPT.md", "the expected R1 line is stated", "run selftracked state"},
		{"#45", "PROMPT.md", "R1 red is part of the procedure", "part of this procedure, not\na new failure"},
		{"#45", "PROMPT.md", "the recovery ends in a bookkeeping commit", "The commit is not optional"},
		{"#45", "PROMPT.md", "the two-writer case stops the recipe", "Both sides hold unique work"},
		{"#45", "SKILL.md", "divergence is a decision, not a command", "decision,\n   not a command"},
		{"#45", "SKILL.md", "the wrong branch is total loss", "total loss\n     in exactly one of them"},
		{"#45", "SKILL.md", "the git test that selects the branch", "git log -1 --stat -- .selftracked/dump.sql"},
		{"#45", "SKILL.md", "the rule that selects a branch", "work that exists nowhere else"},
		{"#45", "SKILL.md", "the database-is-good branch exists", "Local database is the good side"},
		{"#45", "SKILL.md", "the database-side recovery", "`selftracked dump`, then `selftracked verify`"},
		{"#45", "SKILL.md", "the expected R1 line is stated", "run\n     selftracked state"},
		{"#45", "SKILL.md", "R1 red is part of the procedure", "part of the procedure, not a new failure"},
		{"#45", "SKILL.md", "the recovery ends in a bookkeeping commit", "without which the divergence is unresolved"},
		{"#45", "SKILL.md", "the two-writer case stops the loop", "Both sides hold unique work"},
		{"INV-477", "SKILL.md", "backlog refinement re-prime", "re-`prime` between passes"},
		{"INV-478", "SKILL.md", "the drift rule", "`create` + park, one command"},
		{"INV-479", "SKILL.md", "the bookkeeping-commit rule", "bookkeeping commit"},
		{"#19", "SKILL.md", "explicit staging for the bookkeeping commit", "git add .selftracked/dump.sql STATE.md"},
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
	// #45: a branch's LABEL must be tied to the recovery printed under it.
	// The rows above are independent whole-file `Contains` checks, so
	// swapping the two labels — which makes the text prescribe `load
	// --force` for the case where it destroys every unsynced local write —
	// satisfies all of them. These cases read the segment BETWEEN one label
	// and the next and assert what may and may not stand inside it: a swap
	// then either strands the second label before the first or drags the
	// wrong recovery into the segment, and fails. The `must` half doubles as
	// the pin for each step of the recovery, so deleting the divergence
	// recovery's own bookkeeping commit — a literal that also appears in the
	// end-of-session instruction, and therefore survives in the file — fails
	// here rather than passing a re-generated golden.
	segment := func(doc, label, next string) string {
		s := strings.Index(doc, norm(label))
		if s < 0 {
			t.Errorf("#45: branch label %q not found", label)
			return ""
		}
		rest := doc[s+len(norm(label)):]
		e := strings.Index(rest, norm(next))
		if e < 0 {
			t.Errorf("#45: branch %q is not followed by %q", label, next)
			return ""
		}
		return rest[:e]
	}
	for _, b := range []struct {
		doc, label, next string
		must, mustNot    []string
	}{
		{
			doc: "PROMPT.md", label: "**The tracked dump is the good side**",
			next: "**The local database is the good side**",
			must: []string{"selftracked load --force"},
			mustNot: []string{
				"selftracked dump", "selftracked verify",
				"selftracked state", "git add .selftracked/dump.sql STATE.md",
			},
		},
		{
			doc: "PROMPT.md", label: "**The local database is the good side**",
			next: "**Both sides hold unique work**",
			must: []string{
				"Discard nothing", "selftracked dump", "selftracked verify",
				"run selftracked state",
				`git add .selftracked/dump.sql STATE.md && git commit`,
			},
			mustNot: []string{"load --force"},
		},
		{
			doc: "SKILL.md", label: "**Tracked dump is the good side**",
			next: "**Local database is the good side**",
			must: []string{"selftracked load --force"},
			mustNot: []string{
				"selftracked dump", "selftracked verify",
				"selftracked state", "git add .selftracked/dump.sql STATE.md",
			},
		},
		{
			doc: "SKILL.md", label: "**Local database is the good side**",
			next: "**Both sides hold unique work**",
			must: []string{
				"discard nothing",
				"`selftracked dump`, then `selftracked verify`",
				"run selftracked state",
				`git add .selftracked/dump.sql STATE.md && git commit`,
			},
			mustNot: []string{"load --force"},
		},
	} {
		seg := segment(docs[b.doc], b.label, b.next)
		for _, want := range b.must {
			if !strings.Contains(seg, norm(want)) {
				t.Errorf("#45 (%s): the %q branch does not carry %q",
					b.doc, b.label, want)
			}
		}
		for _, bad := range b.mustNot {
			if strings.Contains(seg, norm(bad)) {
				t.Errorf("#45 (%s): the %q branch carries %q, which belongs to the other branch",
					b.doc, b.label, bad)
			}
		}
	}

	// The rule file carries the no-raw-SQL and no-PO rules too (INV-471/473).
	for _, want := range []string{
		"run `sqlite3`", "answer a product-owner decision",
		"git add .selftracked/dump.sql STATE.md",
	} {
		if !strings.Contains(rule, norm(want)) {
			t.Errorf(".claude rule missing %q", want)
		}
	}
	// The deny-list entry §11.2 names must actually be in the settings file
	// (INV-472), not merely described in PROMPT.md.
	if !strings.Contains(settings, `Bash(sqlite3:*)`) {
		t.Error("INV-472: .claude/settings.json missing the sqlite3 deny-list entry")
	}

	// Each seeded class README documents its class contract (INV-403), not
	// just its existence — asserted per file so a garbled one is caught
	// beyond the tautological golden.
	readmes := map[string]string{
		"docs/research/README.md":  "the `research` class",
		"docs/decisions/README.md": "the `adr` class",
		"work/runs/README.md":      "the `run` class",
		"work/reports/README.md":   "the `report` class",
	}
	for rel, want := range readmes {
		if !strings.Contains(read(rel), norm(want)) {
			t.Errorf("INV-403: %s does not document its class contract (%q)", rel, want)
		}
	}
}

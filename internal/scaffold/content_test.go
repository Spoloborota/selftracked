package scaffold

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// absPathToken matches a token that begins an absolute path, at a position
// where it is a path rather than punctuation. Four shapes are absolute:
//
//   - `/` followed by a letter — a POSIX absolute path;
//   - `~` followed by a letter or a slash — `~/x` and the tilde-username
//     form `~name/x`, both of which the shell expands to an absolute path;
//   - a drive letter, a colon and a separator followed by a letter
//     (`C:\Users\…`, `C:/Users/…`) — a Windows absolute path, which this
//     project's Windows CI job would run against;
//   - two backslashes followed by a letter (`\\server\share`) — a UNC path.
//
// The preceding character decides whether the token is a path at all:
// start of line, whitespace, a quote, a backtick, a bracket, or one of the
// separators that glue a path to what names it (`=`, `:`, `|`, `,`, `;`) —
// `PATH=/x`, `see:/x`, a `|/x|` table cell. `>` is deliberately NOT in that
// class: `2>/dev/null` in the generated settings file is a shell
// redirection to a device, not a machine-identifying path.
//
// What must keep passing: a URL (`https://x` — the slash after the colon is
// followed by another slash, and the scheme is more than one letter, so
// neither the POSIX nor the Windows shape fires), and a repo-relative path
// such as `./bin/selftracked`, `.selftracked/dump.sql` or `docs/research/`
// (the character before the slash is a letter or a dot).
var absPathToken = regexp.MustCompile(`(?m)(^|[\s"'` + "`" + `(\[<=:|,;])` +
	`(/[A-Za-z]|~[A-Za-z/]|[A-Za-z]:[\\/][A-Za-z]|\\\\[A-Za-z])`)

// settingsRel is the one non-.md file this check covers; see the scope note
// on TestGeneratedDocsCarryNoAbsolutePaths.
var settingsRel = filepath.Join(".claude", "settings.json")

// TestGeneratedDocsCarryNoAbsolutePaths enforces section 14's rule —
// "selftracked's own verbs never write hostnames, usernames, or absolute
// paths" — over the text init writes. `init` is a verb and these files are
// what it writes, so the rule reaches them; PROMPT.md's "Running the tool"
// section is the passage most exposed to it, because a literal install path
// is exactly what a well-meaning edit would substitute for the shape stated
// there.
//
// Scope, stated rather than implied by a file extension — section 14's rule
// is not scoped by one. Checked: every generated `.md` document, plus
// `.claude/settings.json`, which init writes from a template and which
// carries commands that an edit could pin to one machine. Excluded, each
// for a reason:
//
//   - `.selftracked/hooks/pre-commit` and `post-commit` — a generated shell
//     hook may legitimately carry an interpreter path;
//   - `.gitignore` — its lines are ignore patterns, and a leading slash
//     there means "repository root", not "filesystem root";
//   - `.selftracked/dump.sql`, `dump.hash`, `db.sqlite` — derived database
//     surfaces, not authored text: what they may contain is governed by the
//     verbs that write the rows, not by a template review.
func TestGeneratedDocsCarryNoAbsolutePaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := writeScaffold(context.Background(), root, false); err != nil {
		t.Fatal(err)
	}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if filepath.Ext(p) != ".md" && rel != settingsRel {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for line := range strings.SplitSeq(string(b), "\n") {
			if absPathToken.MatchString(line) {
				t.Errorf("%s carries what reads as an absolute path, which no generated document may: %q", rel, line)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

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
		// #54/#51/#56: the three operating sections. The heading of each is
		// asserted here so a deleted section fails on its own row as well as
		// on the segment lookups below.
		{"#54", "PROMPT.md", "the invocation section exists", "## Running the tool"},
		{"#54", "PROMPT.md", "the binary and its alias are named", "The binary is `selftracked`; `strk`"},
		{"#54", "PROMPT.md", "PATH is where the binary is expected", "It is expected on `PATH`"},
		{"#54", "PROMPT.md", "the invocations are shapes, not paths", "as shapes, never as literal paths"},
		{"#54", "PROMPT.md", "the rule that makes them shapes", "never write hostnames, usernames, or absolute paths"},
		{"#54", "PROMPT.md", "the one-line check that replaces probing", "replaces\nprobing the environment"},
		{"#54", "PROMPT.md", "prime answers or names its refusal", "either **answers**"},
		{"#51", "PROMPT.md", "the terminal-refusal section exists", "## When a verb refuses because the epic is closed"},
		{"#51", "PROMPT.md", "the class is named by its code", `{"code":"terminal"}`},
		{"#51", "PROMPT.md", "the post-close vocabulary is a closed list", "a **closed list**"},
		{"#51", "PROMPT.md", "there is no epic reopen", "there is no `epic reopen`, ever"},
		{"#51", "SKILL.md", "the loop meets the class where prime shows it", "neither active nor paused"},
		{"#51", "SKILL.md", "the loop names the code", `{"code":"terminal"}`},
		{
			"#51", "SKILL.md", "the loop points at PROMPT.md rather than restating",
			`PROMPT.md's "When a verb refuses because the epic`,
		},
		{"#56", "PROMPT.md", "the language section exists", "## The language of what you write"},
		// Quoted from §9, preposition included: the spec says "chosen once
		// per repository" and asks this clause to be quoted rather than
		// paraphrased, so the wording is pinned as the spec writes it.
		{"#56", "PROMPT.md", "one language, chosen once", "one language, chosen once per repository"},
		{"#56", "PROMPT.md", "English is the default", "**English is the\ndefault this contract ships with**"},
		{"#56", "PROMPT.md", "the dump-is-permanent reason", "published and permanent"},
		{"#56", "PROMPT.md", "the greppability reason", "splits its own greppability"},
		{"#56", "PROMPT.md", "the one-repository-one-language reason", "should not need a second language"},
		{"#56", "PROMPT.md", "a default, not a prohibition", "a **default, not a prohibition**"},
		{
			"#56", "PROMPT.md", "the choice is recorded and applied uniformly",
			"records that choice in its own project memory and applies it\nuniformly",
		},
		{
			"#56", "PROMPT.md", "the default is ungated by construction",
			"no verb refuses a non-English title and\nnone is proposed",
		},
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
	// Used by the #45 branch cases below and by the #54/#51/#56 operating
	// sections after them, which have the same failure mode: a statement
	// that is present in the file but has drifted out of the passage it
	// governs. The error prefix is neutral because the helper is shared.
	//
	// Both anchors must be UNAMBIGUOUS, and an ambiguous one is a failure
	// rather than a first-match guess. A first-match lookup is exploitable:
	// a decoy copy of a heading inserted earlier in the document widens the
	// computed window until it spans decoy AND real section, and every
	// `must` string is then satisfied by bait text inside the decoy while
	// the real section underneath is corrupted — a green suite over a
	// broken contract. So a label that does not occur exactly once in the
	// document, or a `next` anchor that does not occur exactly once after
	// it, fails here.
	segment := func(doc, label, next string) string {
		l, n := norm(label), norm(next)
		if c := strings.Count(doc, l); c != 1 {
			t.Errorf("segment: label %q occurs %d times, want exactly 1 — "+
				"an ambiguous anchor cannot delimit a section", label, c)
			return ""
		}
		rest := doc[strings.Index(doc, l)+len(l):]
		if c := strings.Count(rest, n); c != 1 {
			t.Errorf("segment: the anchor %q occurs %d times after label %q, "+
				"want exactly 1 — an ambiguous anchor cannot delimit a section",
				next, c, label)
			return ""
		}
		// The count above is 1, so this index is never -1.
		end := strings.Index(rest, n)
		return rest[:end]
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

	// confined pins a token that a section may name only inside one
	// sanctioned phrase. A `mustNot` on a single literal is the wrong tool
	// for a route that does not exist: it forbids one surface form and
	// leaves every other phrasing free to present the route as available,
	// which is the dangerous direction — a contract promising what the code
	// refuses. Confinement is the assertion that matches the claim: EVERY
	// occurrence of the token in the section is part of the negation.
	type confined struct{ token, within string }
	// #54/#51/#56: the three operating sections, each read as the passage
	// between its own heading and the next one. The rows above assert that
	// the mandated sentences exist SOMEWHERE in PROMPT.md; these assert that
	// they still stand inside the section that governs them — the failure
	// the #45 round found, where a statement survives the file while the
	// passage it qualifies has lost it. Deleting a whole section fails the
	// lookup itself, so each section is pinned twice over.
	for _, b := range []struct {
		name, doc, label, next string
		must, mustNot          []string
		confined               []confined
	}{
		{
			name: "#54 Running the tool",
			doc:  "PROMPT.md", label: "## Running the tool", next: "## The working loop",
			must: []string{
				"`strk`", "on `PATH`",
				"as shapes, never as literal paths",
				"A repo-local build output", "An absolute install path",
				"selftracked prime",
			},
			// The section's whole point is that the two invocations are
			// shapes; a machine-specific literal substituted for either is
			// the mutation it must not survive. The absolute-path forms are
			// caught wholesale by TestGeneratedDocsCarryNoAbsolutePaths; a
			// home-relative one is caught here, where it would read as a
			// plausible "shape" and is not one.
			mustNot: []string{"~/", "$HOME"},
		},
		{
			name:  "#51 terminal refusal",
			doc:   "PROMPT.md",
			label: "## When a verb refuses because the epic is closed",
			next:  "## The language of what you write",
			must: []string{
				`{"code":"terminal"}`, "exit 1",
				"a **closed list**",
				"worklog add SLUG --story V-N",
				"worklog add … --corrects N",
				"`link` on an `epic:` target",
				"`edit --detach`",
				"`story dissolve SLUG SID --why …` of a non-terminal story",
				"there is no `epic reopen`, ever",
			},
			// The one route that does not exist must not be described as one
			// anywhere in the section — in ANY phrasing, not merely the one
			// a literal `mustNot` would pin. A bullet reading "**`epic
			// reopen`** — reopening a closed epic to correct a mistaken
			// close" contradicts the section's own closing sentence while
			// dodging any single literal; it does not dodge this.
			confined: []confined{{
				token:  "epic reopen",
				within: "there is no `epic reopen`, ever",
			}},
		},
		{
			// #51: the two carve-outs sit in adjacent bullets and each is a
			// carve-out for its OWN reason — detach removes a homing, dissolve
			// satisfies a close condition. Asserting both justifications over
			// the whole section passes when they are swapped between the
			// bullets, which leaves each route explained by the other's
			// reasoning. So each bullet is read alone.
			name:  "#51 the detach carve-out keeps its own justification",
			doc:   "PROMPT.md",
			label: "**`edit --detach`**",
			next:  "**`story dissolve SLUG SID --why …` of a non-terminal story,",
			must: []string{
				"un-homing a task",
				"removes a violation rather than creating one",
				"the repair the `reopen` refusal names",
			},
			mustNot: []string{"`DISSOLVED` is the one story target"},
		},
		{
			// The carve-out and its restriction must live in the same
			// bullet: "CLOSED epic only" stated anywhere else in the section
			// leaves the bullet itself reading as unrestricted.
			name:  "#51 the dissolve carve-out keeps its restriction",
			doc:   "PROMPT.md",
			label: "**`story dissolve SLUG SID --why …` of a non-terminal story,",
			next:  "And the route that is not a route",
			must: []string{
				"on a `CLOSED` epic only",
				// This bullet's own justification, asserted here rather than
				// over the section, so a swap with the `edit --detach`
				// bullet's reasoning fails on both sides.
				"`DISSOLVED` is the one story target that *satisfies* a close condition",
				"a `DISSOLVED` epic never passed a close gate",
				// The guard is narrower than section 6.4's sentence: it also
				// requires that THIS tracker's `epic close` produced the
				// CLOSED status (internal/verb/terminal_epic.go's
				// refuseTerminalEpicExcept consults rules.EpicClosedByVerb).
				// An epic that arrived CLOSED by import is exactly the case
				// this section's reader has in front of them, so a text that
				// promised the route there would be read at the worst moment.
				"arrived `CLOSED` through `import`",
			},
			// The other bullet's justification, for the same reason.
			mustNot: []string{"the repair the `reopen` refusal names"},
		},
		{
			name: "#56 the language of what you write",
			doc:  "PROMPT.md", label: "## The language of what you write",
			next: "## Durable-doc authoring rules",
			must: []string{
				"one language, chosen once per repository",
				"**English is the default this contract ships with**",
				"a **default, not a prohibition**",
				"no verb refuses a non-English title and none is proposed",
				"records that choice in its own project memory",
				// The rule is asymmetric — English needs no declaration, any
				// other choice is recorded — so the section says which of its
				// own reasons actually argue for English. Dropping that
				// sentence leaves a public contract whose stated grounds
				// overreach what they support.
				"the first and the third argue for *one* language",
				"only the second — the locale-fixed `PO:` token — argues for English",
			},
			// A default restated as a rule is the mutation this section is
			// most exposed to: it reads like a tightening, and it would make
			// the contract claim an enforcement no verb performs.
			mustNot: []string{
				"must be written in English",
				"only English", "English only",
			},
		},
		{
			name: "#51 the skill meets the class and points home",
			doc:  "SKILL.md", label: "**Work touching an epic `prime` reports as",
			next: "## Drift rule",
			must: []string{
				`{"code":"terminal"}`,
				`PROMPT.md's "When a verb refuses because the epic is closed"`,
			},
			// One statement, one place: the skill names the class and sends
			// the reader to PROMPT.md; it does not carry a second copy of
			// the closed list, which would be the next thing to go stale.
			mustNot: []string{
				"worklog add SLUG --story V-N", "--corrects N",
				"edit --detach", "story dissolve",
			},
		},
	} {
		seg := segment(docs[b.doc], b.label, b.next)
		for _, want := range b.must {
			if !strings.Contains(seg, norm(want)) {
				t.Errorf("%s (%s): the section under %q does not carry %q",
					b.name, b.doc, b.label, want)
			}
		}
		for _, bad := range b.mustNot {
			if strings.Contains(seg, norm(bad)) {
				t.Errorf("%s (%s): the section under %q carries %q, which it must not",
					b.name, b.doc, b.label, bad)
			}
		}
		for _, c := range b.confined {
			tok, within := norm(c.token), norm(c.within)
			// The sanctioned phrase must itself name the token exactly once,
			// or the arithmetic below means nothing.
			if n := strings.Count(within, tok); n != 1 {
				t.Errorf("%s (%s): the sanctioned phrase %q names %q %d times, want exactly 1",
					b.name, b.doc, c.within, c.token, n)
				continue
			}
			sanctioned := strings.Count(seg, within)
			if sanctioned == 0 {
				t.Errorf("%s (%s): the section under %q does not carry %q",
					b.name, b.doc, b.label, c.within)
				continue
			}
			if total := strings.Count(seg, tok); total != sanctioned {
				t.Errorf("%s (%s): the section under %q names %q %d times but only %d "+
					"are part of %q — every mention must be the negation, in any phrasing",
					b.name, b.doc, b.label, c.token, total, sanctioned, c.within)
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

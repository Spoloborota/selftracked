# Change: a gate step fails when this repository's installed copies drift from the templates it ships

Target: `docs/v0-spec.md` §16 (the gate battery, which is enumerated
normatively — one step added); revision 3.40 → 3.41. `Makefile` — the
`gates` target and a new step; a script or a Go test implementing the
comparison; a fixture proving it fails on a planted divergence. No spec
change outside §16, no schema change, no verb change, no change to
anything `init` installs.
Status: **proposed** · raised 2026-07-26 by task #68 under epic
`adoption-contract`, story S1 · review tier **FULL** (plan §5, D-EP7) ·
awaiting owner review

## Why

Three files in this repository are copies of files in
`internal/scaffold/templates/`, and nothing asserts they stay copies:

| installed copy | template |
|---|---|
| `PROMPT.md` | `internal/scaffold/templates/PROMPT.md` |
| `.claude/skills/selftracked/SKILL.md` | `internal/scaffold/templates/claude_skill.md` |
| `.claude/rules/selftracked.md` | `internal/scaffold/templates/claude_rule.md` |

Verified 2026-07-26: all three pairs are byte-identical right now
(`diff -q` on each, silent). Also verified: `make gates` is
`build vet test lint fix-check vuln check-pins binaries`
(`Makefile`, the `gates` target) — no step compares them.

Two facts make the identity fragile rather than merely unasserted:

- **`init` never overwrites an existing generated document**, so editing
  a template propagates to nothing that already exists — this repository
  included. It is the same property the migration guide advertises as a
  feature ("never overwrites a file that already exists") and it is what
  makes self-hosting on your own scaffold a manual sync.
- **The existing tests pin the wrong tree.** Read 2026-07-26:
  `internal/scaffold/content_test.go:17-35` runs `writeScaffold` into a
  `t.TempDir()` and asserts against files *there*
  (`read(".claude/skills/selftracked/SKILL.md")` resolves under the temp
  root), and the golden fixtures under
  `internal/scaffold/testdata/golden/` pin the same fresh-tree output. So
  every existing assertion is over what `init` would write, which is the
  template's own content — it passes whether or not this repository's
  copies match. A test that compares the template to itself cannot see
  this drift.

This is not hypothetical. Task #55 was filed **because this repository's
own contract had drifted from what its templates said**; a story restored
the identity by hand, and nothing prevents the next template edit from
breaking it again the same way. The observation was raised by that story's
implementer as out-of-scope, deliberately: the implementer never decides
its own guard.

The exposure is immediate and this epic creates it. `contract-answers-the-first-hour`
and `divergence-recipe-covers-both-directions` both edit the templates
and both must edit this repository's copies alongside them; that is why
story S6 is sequenced **before** them. A guard installed afterwards
checks only what already happened.

## What changes

**`make gates` gains a step that fails when any of the three pairs
differ**, with a message naming the pair and the direction to reconcile.
It runs inside `gates`, not as a separate ritual, for the reason the
project's whole gate posture rests on: a check nobody is obliged to run
is not a gate.

Three properties are part of the change rather than left to the
implementer:

1. **The comparison is byte-for-byte.** Not "contains the same
   sections", not a normalized diff. The failure class is a template edit
   that did not propagate, and any tolerance is a hole shaped exactly
   like the next incident.
2. **The pair list is data, not scattered constants, and it is
   enumerated in the step itself.** Exactly three pairs are compared —
   `PROMPT.md`, `.claude/skills/selftracked/SKILL.md` and
   `.claude/rules/selftracked.md` against their templates, as tabulated
   above. The scaffold ships more files than three, so the step also
   **names what it does not compare and why**, with
   `.claude/settings.json` called out by name: it legitimately differs
   from `internal/scaffold/templates/claude_settings.json` (verified by
   `diff -q` 2026-07-26), because this repository's settings file carries
   local configuration an adopter's scaffold has no business receiving.
   Without that sentence in the step's own output, "the drift gate is
   green" reads as "every generated file matches", which is false. A
   later decision to guard a further pair adds a row, not a code path.
3. **A fixture proves it fails.** A planted one-character divergence must
   make the step exit non-zero, and identity must make it exit 0. §16's
   own standing rule — "Every rule ships with a red fixture; a gate that
   cannot fail is decoration" — is stated about `verify`'s rules, and it
   applies here by the same argument.

**Spec §16** carries the step in its gate enumeration. That section
enumerates the battery normatively, so a step that runs in `make gates`
and is absent from §16 is itself a documented-versus-actual drift, of
precisely the class this change exists to catch.

## What this deliberately does NOT do

**It does not make `init` overwrite anything.** The non-overwrite
property is load-bearing for adopters — it is what makes `init` safe to
re-run on a live repository — and a guard over *this* repository's copies
must not purchase its convenience by changing what every adopter's `init`
does.

**It does not propose a sync command.** "Copy the template over the
installed file" is one `cp` per pair, and a verb or a make target that
does it invites running it without reading the diff, which is how a
hand-edit to an installed copy gets silently discarded. The step reports;
the reconciliation is a human or agent decision about which side is
right — and it can be either, since a repository that discovers a defect
in its own contract fixes the template.

**It does not guard adopters' installed copies.** Nothing here runs on an
adopted repository. The drift this catches is the drift of the project
that ships the templates, which is the only place both sides exist.

## Alternative considered and rejected

**A test in the Go suite instead of a make step.** It would ride `make
test` and therefore `gates` for free, and it needs no new script.
**Rejected, and the working-directory resolution is the reason rather
than a detail left open.** `go test` runs each test binary with its
working directory set to the **package** directory, so a Go test for
these pairs would have to walk upward to find the repository root — the
same upward-resolution problem `resolution-names-the-root-it-found`
is spending a whole amendment on, imported into a gate for no benefit,
and with the same permission and symlink questions attached. A
`make` recipe runs at the Makefile's directory, which **is** the
repository root, so the question does not arise: the paths are literal
and relative, and there is nothing to resolve. **Decided: the check is a
step in `make gates`.** Recorded as the rejected branch because it is
cheaper and a reviewer will reach for it, and because "it would work if
we resolved the root" is exactly the reasoning that puts a second
root-walk in the codebase.

**A pre-commit hook step.** Rejected: the pre-commit gate is `verify
--fast`, whose partition is defined by cost and by what a commit boundary
can afford (§7), and whose rules are the V-rules. Adding a repository
file comparison to it would put a project-specific check into a code path
every adopter's commits run through.

**Do nothing and rely on review.** This is the status quo, and it is what
produced #55. Recorded because the honest comparison is not
"guard versus no guard" but "guard versus the review that already missed
it once".

## Consequences accepted with this change

`make gates` gets slower by three file comparisons, which is not a real
cost, and gains one more way to fail, which is. A contributor who edits a
template and not the copy is now stopped at the gate instead of at a
review — that is the point, and it will feel like friction the first time.

The step is a check on *this* repository's discipline, not on the tool's
behaviour. It is the first entry in the §16 battery of that kind. Named
because §16's other entries are about the built artifact, and a reader
should know the section now carries both kinds rather than assuming a
misfile.

Three pairs are guarded and the rest of the scaffold is not, and the
boundary is not arbitrary — it is measured. Two more pairs exist:

- `AGENTS.md` vs `internal/scaffold/templates/AGENTS.md` — **currently
  identical** (`diff -q`, 2026-07-26). It is a candidate for the list; it
  is left out because it is a pointer file, not the contract #55 was
  about, and adding a row later is one line.
- `.claude/settings.json` vs
  `internal/scaffold/templates/claude_settings.json` — **currently
  differ** (`diff -q`, 2026-07-26). This one must stay out: this
  repository's settings file carries local configuration that an
  adopter's scaffold has no business receiving, so the two are
  legitimately different and a byte-equality guard over them would be a
  gate that is red by design.

That second pair is why property 3 above requires the step to state what
it does *not* check: "the gate is green" must not be read as "every
generated file matches", and the one pair that is genuinely divergent is
the reason a reader would otherwise draw exactly that conclusion.

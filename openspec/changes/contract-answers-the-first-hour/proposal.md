# Change: the generated contract answers a fresh agent's first hour

Target: `docs/v0-spec.md` §9 (the paragraph enumerating what `init`'s
`PROMPT.md` carries — three additions), §11.3 (the skill's loop gains the
refusal class it can meet) and §6.2 (the `create` row's signature line);
revision 3.33 → 3.34. `internal/scaffold/templates/PROMPT.md` — three new
sections; `internal/scaffold/templates/claude_skill.md` — one branch;
`internal/verb/tasks.go:123` — the `create` usage string; this
repository's own `PROMPT.md` and `.claude/skills/selftracked/SKILL.md`,
which are byte-identical copies of those templates (verified by `diff`
2026-07-26). Content tests. No schema change, no behavioural change to
any verb.
Status: **proposed** · raised 2026-07-26 by tasks #54, #51, #56 and #26
under epic `adoption-contract`, story S1 · review tier **FULL** (plan §5,
D-EP7) · awaiting owner review

## Why

Four holes in the installed materials, each observed in practice, each
paid for by an agent's first hour rather than by a reader of this
repository.

**The tool's invocation is undocumented (#54).** After `init` the
repository carries `PROMPT.md`, the skill and the rules file, all of
which describe verbs; none states where the binary lives or that it is
expected on `PATH`. Verified 2026-07-26: `grep -n "PATH\|install\|binary"`
over `internal/scaffold/templates/{PROMPT.md,claude_skill.md,claude_rule.md,AGENTS.md}`
returns nothing on any of those tokens. An agent working from the
installed materials alone discovers the invocation by probing its
environment — seen twice in fresh-install exercises, each time recorded
as a guess.

**The terminal-epic refusal class is undocumented (#51).** Writes against
a CLOSED or DISSOLVED epic are refused `{"code":"terminal"}`, exit 1, at
**ten** guard call sites — counted 2026-07-26 by
`grep -rn refuseTerminalEpic internal/verb/*.go` excluding tests:
`criteria.go:93,126`, `tasks.go:141,660,855,880,939`,
`stories.go:122,186,240`. The site count understates the verb surface:
`stories.go:186` is `moveStory`'s shared call, serving `ready`, `block`,
`unblock`, `done` and `dissolve` at once. §6.4 enumerates the class
normatively and names its post-close vocabulary; nothing that `init`
installs mentions either. The exposure is largest immediately after an
import, where terminal epics arrive in bulk — which is exactly the
adopter's first hour. Corroborated from the discovery side: in a
fresh-install exercise an agent reconstructed the V-row route from the
DDL comments inside the generated `dump.sql` plus the verb grammar and
precedent among existing rows.

**No language policy for tracker content (#56).** The generated contract
addresses language exactly once, and only about one token:
`internal/scaffold/templates/PROMPT.md:89-92` fixes the literal `PO:` as
English "for greppability" and tells a non-English crew to adapt the
literal in its prompt config. So the contract already anticipates a
non-English crew and still says nothing about the language of the titles,
notes, DoD text and worklog prose that crew will write — the content that
`dump.sql` publishes permanently (§14). An agent with no written default
mirrors whatever the existing rows use, which on a fresh instance is
nothing. This repository carries such a rule outside the generated
surface, which is precisely the shape of the gap: the rule exists and the
shipped contract does not carry it.

**`create --help` does not state its default (#26).** Verified
2026-07-26: `selftracked create --help` prints
`usage: create --title T [--status OPEN|IN-REVIEW|NEEDS-TRIAGE] [--note N] [--epic SLUG] [--label] [--json]`
(`internal/verb/tasks.go:123`). Bare `create` lands `NEEDS-TRIAGE` — a
deliberate choice, because an untriaged item's status is unknown and
unknown is data — but the bracketed alternation names three statuses in
an order that reads as a default and is not one. Two agents in a row
assumed `OPEN` and had to correct course. `PROMPT.md` has documented the
real default since #21; the usage line, which is what an agent actually
reads at the moment of the call, does not.

## What changes

**`PROMPT.md` gains three sections.**

1. **"Running the tool"** — the binary is `selftracked` (alias `strk`,
   §6.1), expected on `PATH`; if it is not, the contract states the two
   invocations that work without one (a repo-local build output, an
   absolute install path) *as shapes*, never as a literal path, because
   §14 forbids this project's generated text from carrying absolute
   paths. The section also states the one-line check an agent runs
   instead of probing: `selftracked prime` either answers or names its
   refusal.

2. **"When a verb refuses because the epic is closed"** — the terminal
   refusal class named by its code (`terminal`), what produces it (any
   write that would re-open one of §6.4's close conditions), and the
   sanctioned routes, quoted from §6.4's closed list rather than
   paraphrased:
   - `worklog add SLUG --story V-N …` — post-close validation on a
     **CLOSED** epic;
   - `worklog add … --corrects N` — a correction row on any story;
   - `link` — artifact links stay open (a retrospective attached to a
     closed epic breaks no condition);
   - `edit <id> --detach` — un-homing a task, the repair the `reopen`
     refusal already names;
   - `story dissolve SLUG SID --why …` of a **non-terminal** story on a
     **CLOSED** epic — the carve-out accepted with
     `r16-reports-only-what-a-verb-can-clear`, which is why this section
     cannot be written from §6.4's pre-amendment text;
   - and the route that is *not* a route: there is no `epic reopen`, ever
     (§17) — a revived goal is a new epic.
   The DISSOLVED/CLOSED asymmetry is stated once: `story dissolve`'s
   carve-out is CLOSED-only.

3. **"The language of what you write"** — tracker content (titles, notes,
   goals, DoD, `consumes`/`produces`, worklog notes, resolutions,
   criteria text) is written in **one language, chosen once per
   repository, and English is the default** the generated contract ships
   with. The stated reasons are the ones that are true of this artifact
   and not general advocacy: `dump.sql` is published and permanent
   (§14), the `PO:` token is already locale-fixed
   (`PROMPT.md:89-92`) so a mixed-language tracker splits its own
   greppability, and a reader of a published dump should not need a
   second language to follow one repository. A crew that chooses
   otherwise records the choice in its own project memory and applies it
   uniformly — the contract states a default, not a prohibition, because
   no verb can enforce it.

**The skill's loop gains one branch** naming the terminal refusal where
an agent meets it (after `prime`, when the work in hand touches an epic
`prime` reports as neither active nor paused), pointing at `PROMPT.md`'s
section rather than restating the route list — one statement, one place.

**`create`'s usage line states the default**: the `--status` alternation
becomes `[--status OPEN|IN-REVIEW|NEEDS-TRIAGE (default NEEDS-TRIAGE)]`.
The §6.2 `create` row's signature is updated to match, because that row
is the normative source the usage string implements.

**This repository's own copies are updated in the same change**, for the
reason `contract-says-where-new-work-goes` established: `init` never
overwrites an existing generated document
(`internal/scaffold/scaffold.go:270-272`), so a template-only edit closes
the tasks without fixing the repository that raised them.

## Alternative considered and rejected

**Ship the refusal-class documentation as a `--help` addition on each
guarded verb instead of a contract section.** Rejected on the measured
shape of the surface: the class spans ten call sites across three files
and more verb paths than that, so the documentation would be written ten
times and drift at the first amendment — and this class has already been
amended twice in two days (`terminal-epics-refuse-reopening-writes`,
`r16-reports-only-what-a-verb-can-clear`). The refusals themselves
already name their routes at the point of failure
(`worklog-refusals-name-the-routes`); what the contract adds is the
agent's ability to know the class exists *before* meeting it, which is a
document's job, not a usage string's. `create --help` is treated
differently and does change, because there the missing fact is one word
about one flag on one verb, and the flag's default is unreachable from
anywhere else at the moment of the call.

**State the language policy as "the language of the existing rows".**
Rejected: it is exactly what an agent already infers, it is undefined on
a fresh instance (the only rows are `init`'s own), and it makes the first
row written a permanent unreviewed decision.

## Relationship to `resolution-names-the-root-it-found`

Both proposals amend **§9**, in different paragraphs and four revisions
apart: this one adds to the paragraph enumerating what `init`'s
`PROMPT.md` carries; that one adds the working-directory precondition to
the layout paragraph above it. They do not overlap textually, and the
cross-reference exists so that whichever is applied second is applied to
a §9 its author has re-read rather than to the §9 this proposal was
written against.

## Consequences accepted with this change

`PROMPT.md` grows by three sections in a file the epic's other stories
also edit; the drift guard proposed by `gates-catch-installed-copy-drift`
is what keeps this repository's copies honest afterwards, and story S6
is sequenced before this one for that reason.

The language default is guidance in a generated file, of the same class
as the durable-doc authoring rules `PROMPT.md` already carries and
already labels as ungated. No verb refuses a non-English title, and this
change does not propose that one should — a text-language check is not
mechanically sound (identifiers, quoted output, proper nouns) and would
be a new refusal class with no recovery path. Stated plainly so a later
reader does not mistake the section for an enforced rule.

Every adopter's next `init` installs the sections; existing installs do
not update themselves. That gap is inherent to the scaffold model and is
named again here rather than assumed.

The `create` usage line is asserted verbatim by fixtures; the change
touches them, and a fixture that still asserts the old line is the
change's own failure signal rather than a silent partial application.

# Change: from a subdirectory, a verb names the root it found instead of advising `init`

Target: `docs/v0-spec.md` §6.1 (a new convention: where `.selftracked/`
is resolved, what a verb does when it is not here, and what it does when
it cannot tell) and §9 (the layout paragraph states the
working-directory contract it has always assumed); revision 3.38 → 3.39.
Five refusal sites, enumerated below. New fixtures. No schema change;
**no verb changes which database it operates on.**
Status: **proposed** · raised 2026-07-26 by task #47 under epic
`adoption-contract`, story S1 · review tier **FULL** (plan §5, D-EP7) ·
**this is a design fork the epic could not settle for itself; the branch
below is the researched recommendation and the ratification is the
owner's** · **revised 2026-07-26 against a critic round: the refusal
inventory was incomplete, the permission-error class was unnamed, the
symlink claim was unproven, and ADR 0001 was cited as governing
precedent when it does not govern this** · awaiting owner review

## Why

The CLI resolves `.selftracked/` as a bare relative path against the
working directory. Verified 2026-07-26: `instanceDir = ".selftracked"` is
a package constant in **five** packages — `internal/verb/pipeline.go:27`,
`internal/dump/verb.go:20`, `internal/load/verb.go:18`,
`internal/verify/verify.go:26`, and (as a parameter)
`internal/scaffold/scaffold.go:28` — and `requireInstance` stats
`filepath.Join(instanceDir, dbFile)` relative to the process's working
directory (`internal/verb/pipeline.go:115-126`). There is no upward
search.

The consequence, reproduced 2026-07-26 by running `selftracked list` from
this repository's own `internal/` directory:

```console
$ selftracked list
{"error":{"code":"not-found","message":"no .selftracked/db.sqlite here; run selftracked init first"}}
```

The message is accurate about the first clause and dangerous in the
second. A reader who follows the advice — an agent especially, since
following stated advice is what it is for — runs `init` and creates a
**second, nested tracker inside a repository that already has one**. The
repository then has two databases, two dumps, two `PROMPT.md` files and
one `.gitignore`. Observed while answering an owner's how-do-I-use-this
question, which is the first-hour condition this epic exists for.

This is the same identity-blindness family as #24: the tool knows where
it is looking and never says where that is, or where the thing it was
looking for actually lives.

### The complete refusal surface

This proposal's own stated risk is partial application, so the inventory
is normative rather than illustrative. Verified 2026-07-26 by
`grep -rn "run selftracked init first" internal/ cmd/` plus a sweep of
every `"not-found"` refusal in `internal/`:

| # | Site | Message today |
|---|---|---|
| 1 | `internal/verb/pipeline.go:115-126` — `requireInstance`, called at `:136` and `:261`, so it backs every verb going through the pipeline | `no .selftracked/db.sqlite here; run selftracked init first` |
| 2 | `internal/dump/verb.go:50-55` | byte-identical to (1) |
| 3 | `internal/verify/verify.go:167-172` — `notFoundError.Error()`, surfaced by `internal/verify/verb.go:45-47` | byte-identical to (1) |
| 4 | `internal/verb/gate.go:52-58` | **different string**: `no .selftracked here; run selftracked init first` — it stats the directory, not the database |
| 5 | `internal/load/verb.go:41-48` | `no .selftracked/dump.sql to load` — names the **dump**, carries **no** `init` advice |

Four independent copies of one refusal and a fifth with different
semantics. Site (3) is the one a first pass misses and the most costly to
miss: `verify` is the pre-commit gate (§9) and the verb an operator runs
by hand, so a fix that stops at the pipeline leaves the `init` advice
printing from the gate itself. Site (5) needs its own wording rather than
the shared one — a missing *dump* is a different fact from a missing
*database*, and `load`'s job is to build the latter from the former.
Site (4)'s divergent string is worth reconciling in the same pass or
deliberately leaving alone; either is defensible, silently doing one
while believing the other is not.

## What changes — and the boundary that is the whole of the decision

**A verb walks upward ONLY to produce a better refusal. It never operates
on a tracker it found above.**

- **Tracker in the working directory**: today's behaviour, unchanged, no
  walk. The successful path pays nothing.
- **None here, one in an ancestor**: the verb still refuses, exit 1, code
  `not-found`. The message **names that root** and **does not mention
  `init`**. The root is expressed **relative to the working directory**
  (`..`, `../..`, `../../..`) — never absolute, per §14 — and is
  accompanied by the instance digest that `a-tracker-carries-a-name`
  defines, so the refusal answers *where* and *which one* together.
- **None anywhere up the tree**: **the present message stands, verbatim**,
  `init` advice included. There it is correct advice, and this change
  must not degrade the one case that already works.
- **The walk cannot see**: below.

### The permission class — a new failure mode, named because it is new

Today's check is a **relative** `Stat` that the kernel resolves against
the already-open working directory. It requires no permission on
anything above, ever. Any walk that renders a root relative to the
working directory must traverse the parent, and that introduces a failure
this code has never had. Reproduced 2026-07-26 in a scratch tree, from
inside `outer/inner/deep` with `outer/inner` at mode `000`:

```
.selftracked/db.sqlite        -> FileNotFoundError  ENOENT
../.selftracked/db.sqlite     -> PermissionError    EACCES
../../.selftracked/db.sqlite  -> PermissionError    EACCES
```

The working-directory check still answers cleanly; the very first
ancestor candidate fails with `permission denied`.

**Specified behaviour: a permission error is indistinguishable from "no
tracker here".** The walk stops at the first candidate it cannot read and
the present message stands, `init` advice included. It does not report
the permission error, does not partially report what it saw below the
opaque directory, and does not exit differently. Two reasons: a walk that
surfaces `EACCES` teaches the reader about a directory above their
repository that is none of the tool's business, and an exit code that
varies with the permissions of an unrelated ancestor is a refusal
contract that depends on the machine rather than on the tracker. The cost
is accepted and stated: in this configuration the reader gets the old,
worse message. That is strictly no worse than today.

### The resolution basis — stated, because the two choices differ

The walk's distance is printed, so what it counts must be defined. Go's
`os.Getwd` documents that when a directory "can be reached via multiple
paths (due to symbolic links), Getwd may return any one of them" — so a
logical basis and a physical one give **different `../..` text for the
same layout**, and a symlink anywhere in the chain makes the printed
distance disagree with the real topology.

**Specified: the walk is physical.** The distance printed is the number
of real parent directories between the working directory and the tracker
root, and the tracker root the message names is the physically resolved
one — the same basis `a-tracker-carries-a-name` specifies for its digest
input, so the two surfaces cannot disagree about which directory they are
describing. When physical resolution fails, the permission rule above
applies and the present message stands.

**The symlink traversal is new and is not defended.** An earlier draft of
this proposal claimed the walk "follows no symlink it would not already
have followed"; that claim was wrong and is withdrawn. Today the code
never looks at an ancestor at all, so **every** ancestor `Stat` is
traversal it does not currently perform, and if an ancestor's
`.selftracked` is a symlink, a following `Stat` traverses it. What the
walk does not do is act: it opens no database, parses no dump, reads no
file content, and writes nothing — it stats candidates and stops at the
filesystem root. The claim being made is that reading is bounded and
harmless, not that nothing new is read.

**No accepted decision governs this class.** ADR 0001 was cited as
precedent in an earlier draft and does not apply: its decision is about
not defending a repository's layout against content *the repository
itself supplies*, and this walk reads ambient filesystem **above** the
repository, which that ADR never contemplated. The argument here stands
on its own terms — read-only, no database, no parse, bounded by the
filesystem root, failure-closed on permission — and a reviewer should
judge it as a new question rather than as an application of a settled
one.

**Spec §6.1** gains the convention explicitly, because the spec has never
stated it: `.selftracked/` is resolved against the process's working
directory; a verb operates on the tracker in the working directory or on
none; a verb may look upward to improve a refusal and never to choose a
target; and a candidate it cannot read is treated as absent. **Spec §9**
states the same as the layout's precondition — the `<repo>/` in that tree
is the working directory, which the diagram has always implied and never
said.

## Dependency on the sibling amendment, and what happens if only one lands

`a-tracker-carries-a-name` defines the instance digest this refusal
prints. The dependency is one-directional and is stated so this proposal
reads as a record rather than as half of one:

- **Both accepted** (the intent): the refusal reads as *where* plus
  *which one* — a relative path to the root, and the digest identifying
  it.
- **Only this one accepted**: the refusal names the root by relative path
  and prints no digest. Every other clause here is unaffected — the
  walk, the `init`-advice removal, the permission rule and the physical
  basis all stand. The message is weaker, not broken.
- **Only the identity accepted**: `prime` carries the digest and the
  subdirectory refusal keeps advising `init`. The nested-tracker hazard
  is untouched, which is the outcome to avoid.

The identity is defined once, there; nothing about it is restated here,
so the two cannot drift into two definitions.

## Alternative considered and rejected — recorded as a deferral, not a refutation

**Make verbs *operate* on the ancestor tracker, git-style.** Run
`selftracked create` from `internal/` and have it write to the
repository's tracker, the way `git commit` works from any subdirectory.

This is the better end state. It is not this change, and the reason is
measured rather than aesthetic: **every filesystem operation in the
codebase currently conflates the working directory with the tracker
root.** Verified 2026-07-26:

- `internal/verb/pipeline.go:78,117,234` resolve the database and the
  dump write against the bare constant;
- the path dictionary's roots and R2's existence check resolve against
  `filepath.Dir(dir)` — `internal/verify/rules_fs.go:157`, whose own
  comment says "roots resolve relative to the repo root, dir's parent" —
  which is the working directory when `dir` is `.selftracked`;
- R3 resolves every artifact the same way (`rules_fs.go:186`), and R5's
  git access takes the same base (`rules_fs.go:218`);
- `internal/verb/gate.go:53-86` stats and removes the skip marker against
  the bare constant;
- and the constant itself is declared five times, in five packages
  (inventory above), so "the tracker root" is not currently a value that
  can be threaded anywhere.

Switching the *write target* without switching all of them in the same
change produces a verb that writes to the parent's database while
resolving artifact paths, path roots and the skip marker against the
child directory: `link` would refuse files that exist, R2 and R3 would go
red against a correct dictionary, and the gate marker would be written
where nothing reads it. That is worse than the refusal it replaces,
because a refusal is loud and a silently mis-based artifact check is not.

So this is recorded as a **named deferral**: the ancestor-operating
behaviour is deferred until the tracker root is a single resolved value
threaded through every filesystem consumer, which is a refactor with its
own story and its own review. It is not rejected on merit, and a later
reader should not cite this proposal as a decision against it.

**Refuse harder — drop the `init` advice everywhere.** Rejected: on a
directory with no tracker anywhere above it, `init` is the correct
advice, and removing it would trade a dangerous message in one case for a
useless one in the common case — which is also, exactly, the
permission-error case above.

**Have `init` itself refuse inside a repository that already has a
tracker above.** A real guard, and adjacent: it closes the same hazard
from the other end. Not proposed here because it changes `init`'s
contract (`init [--force]`, §9) rather than a refusal's text, and because
a nested tracker is legitimate in at least one shape the project already
uses — a rehearsal copy under a gitignored path inside this repository's
own `work/local/`. Deciding when nesting is a mistake is a separate
question from telling a reader where the tracker they meant is.

## Relationship to `contract-answers-the-first-hour`

Both proposals amend **§9**, in different paragraphs and four revisions
apart: that one adds to the paragraph enumerating what `init`'s
`PROMPT.md` carries; this one adds the working-directory precondition to
the layout paragraph above it. They do not overlap textually, and the
cross-reference exists so the applier of the second one re-reads the
first's edit rather than applying to a §9 it has already changed.

## Consequences accepted with this change

A refusal now costs a bounded directory walk. It runs only on the failure
path, so no successful verb pays for it.

One refusal becomes two, across five sites, three of which currently
carry the same string by duplication rather than by sharing. Fixtures
must cover: the ancestor case, the nothing-anywhere case (the old message
must survive **unchanged** — this is the fixture that proves the change
did not degrade the working case), and the unreadable-ancestor case. The
`verify` site (3) deserves its own fixture for the reason it was missed
in the first place.

A verb run from a subdirectory still does nothing. An agent that reads
the new message and changes directory has done one extra step; an agent
that read the old one and ran `init` had created a second tracker. The
change buys the difference between those two outcomes and nothing more,
and it is worth stating plainly rather than describing it as
subdirectory support.

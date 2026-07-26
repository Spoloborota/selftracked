# Change: an import that inserted nothing refuses instead of exiting green

Target: `docs/v0-spec.md` §6.2 (the `import` row — one new refusal and the
counts line's scope) and §10 (the importer-obligations bullet); revision
3.35 → 3.36. `internal/verb/import.go:181` — the counts line;
`internal/verb/import.go` — the empty-corpus refusal, placed so it covers
both readers; `internal/verb/import_mdtable.go` — no parser change is
required and none is proposed. New red fixtures. No schema change.
Status: **accepted** · raised 2026-07-26 by task #39 under epic
`adoption-contract`, story S1 · review tier **FULL** (plan §5, D-EP7) ·
ratified by the coordinating agent 2026-07-26 under the owner's explicit 2026-07-26 grant of autonomy for this session · applied to the spec and the migration
guide the same day

## Why

`import` can consume a whole corpus, insert nothing, and exit 0. Two of
ten discovery iterations hit it. Re-measured 2026-07-26 against the
current binary, in a fresh instance, and the result is **wider than the
task that filed it says**:

```console
$ selftracked import --file bare.md --format md-table      # pipe table, no heading
imported 0 epic(s), 0 story(ies), 0 task(s), 0 worklog row(s) from bare.md      [exit 0]
$ selftracked import --file prose.md --format md-table     # prose only
imported 0 epic(s), 0 story(ies), 0 task(s), 0 worklog row(s) from prose.md     [exit 0]
$ selftracked import --file emptysec.md --format md-table  # '## tasks' + header row, no data rows
imported 0 epic(s), 0 story(ies), 0 task(s), 0 worklog row(s) from emptysec.md  [exit 0]
$ selftracked import --file empty.json --format json       # the two bytes '{}'
imported 0 epic(s), 0 story(ies), 0 task(s), 0 worklog row(s) from empty.json   [exit 0]
```

#39 narrowed the defect to md-table corpora with **no recognized heading
at all**, and that narrowing is correct as far as it goes — a heading
that is present but wrongly cased refuses loudly (`unknown section
"Tasks"`, exit 1, `internal/verb/import_mdtable.go:57-59`, reproduced).
But the silent-green shape is not md-table's: the fourth line above is
the JSON reader, and the third is a *recognized* md-table section. The
mechanism is common to both — `parseMdTable` returns an empty corpus
without error when `splitSections` yields nothing
(`import_mdtable.go:50-73`), `parseJSON` decodes `{}` into a zero-value
corpus and returns it (`import.go:208-220`: the only refusals there are
the leading-brace check and a decode error), and the insert path is happy
to insert nothing.
So the defect belongs to `import`, not to one of its readers, and a
per-parser fix would leave the JSON half standing.

A fifth measurement decides the rule's shape:

```console
$ selftracked import --file pathsonly.md --format md-table   # a '## paths' section with one row
imported 0 epic(s), 0 story(ies), 0 task(s), 0 worklog row(s) from pathsonly.md [exit 0]
$ selftracked paths ls
src -> internal
```

A paths-only corpus **imports successfully** and still prints four zeros,
because the counts line (`import.go:181`) names epics, stories, tasks and
worklog and not the fifth thing the importer writes. So "all four
counters are zero" is not a correct test for "nothing was imported", and
a refusal built on it would refuse a legitimate dictionary import. It is
also why the operator cannot presently distinguish a working paths import
from a total no-op: both print the same line.

Why this matters beyond tidiness: the guide's §8 makes "the `imported N
epic(s), …` counts line" a stated expectation of the agent-executed
handoff, and its sufficiency criterion is that a fresh-context agent
reaches `verify` green with the fidelity table matching. A green exit and
an all-zero counts line satisfy neither the fidelity table nor a reader's
attention — the agent's next step is to trust the exit code.

## What changes

**`import` refuses a corpus from which it inserted no row of any kind.**
The test is over **every** section the importer writes — path-dictionary
rows included, not the four counters — so the paths-only import above
stays green. The refusal is `{"code":"format"}`, exit 1, consistent with
the reader-level refusals it joins, and its message distinguishes the two
reasons a corpus can be empty, because they call for different actions:

- **nothing recognized** — the file yielded no section the reader knows.
  The message names the format that was used and the section headings
  that format accepts, so a reader who wrote `## Tasks`, or wrote a bare
  pipe table with no heading above it, is redirected rather than stopped.
- **recognized, but no rows** — every section the reader found was empty.
  The message says so plainly; an operator who genuinely meant to import
  an empty file learns that the tool treated it as a mistake, which is
  the correct default for a batch verb whose whole purpose is insertion.

**The counts line names path-dictionary rows.** It becomes
`imported N path(s), N epic(s), N story(ies), N task(s), N worklog row(s)
from F`. This is inside the change rather than deferred because after the
refusal lands, a paths-only import is the *only* remaining green exit
whose counts line is all zeros — the change would otherwise leave behind
exactly the output shape it exists to eliminate, in the one case it
deliberately does not refuse.

**Spec §6.2's `import` row** states the refusal and the counts line's
full scope. **Spec §10's importer-obligations bullet** states the rule as
an obligation of the verb: an import that inserts nothing is a refusal,
because a batch verb's green exit is a claim that the batch landed.

## Alternative considered and rejected

**Warn on stderr and keep exit 0.** Cheaper, breaks no caller, and
rejected: the whole failure is that the operator's next step is driven by
the exit code, and the migration guide's handoff shape makes the exit
code an asserted expectation. A warning beside a zero exit is the
condition the tool already has — the counts line *is* the warning, and it
was read past twice in ten iterations. §6.1's exit contract has a code
for "understood and correctly denied", and this is that.

**Fix it in `parseMdTable` — refuse a document with no `## ` heading.**
Rejected on the measurement: it addresses one of the four reproduced
shapes and leaves the recognized-but-empty section and the entire JSON
reader untouched, and it would put a rule about *import outcomes* inside
a *parser*, which is where the format-shape refusals live. The parser
stays as it is; the rule sits where the outcome is known.

**Refuse only when the four existing counters are all zero.** Rejected on
the paths-only measurement above: it refuses a corpus the importer
correctly imported.

## Consequences accepted with this change

An adopter's deliberately-empty corpus — a placeholder committed before
the real one, a delta round that turned out to have no delta — now
refuses. That is a behaviour change for a script that imports on a
schedule and tolerates empty rounds. It is accepted because such a script
cannot presently distinguish "no delta" from "my corpus stopped parsing",
which is the more expensive of the two failures, and because the refusal
message says which case it is.

The counts line changes shape, so every fixture and every document
quoting it changes with it — the guide's §8 expectation among them. Named
here so the application is not mistaken for a one-line edit; a fixture
still asserting the four-field line is this change's failure signal.

R5's existing honesty note is untouched: a sha-shaped string that does
not resolve still imports and fails later at `verify`. This change
addresses the corpus that inserted *nothing*, not the row that inserted
wrongly.

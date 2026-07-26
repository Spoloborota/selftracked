# Change: one field, one name — the criterion field across the verb and the corpus

Target: `docs/v0-spec.md` §6.2 (the `criteria` row's signature) and §10
(the importer's field vocabulary stated as a tool-wide rule); revision
3.39 → 3.40. `internal/verb/criteria.go:60-62,86,88` — the flag and its
refusals; `internal/verb/import.go:71-72` — the corpus field, or its
alias; `docs/migration-guide.md` §4's JSON example. Fixtures. No schema
change — the column is `epic_criteria.criterion` either way.
Status: **proposed** · raised 2026-07-26 by task #62 under epic
`adoption-contract`, story S1 · review tier **FULL** (plan §5, D-EP7) ·
awaiting owner review

## Why

One field carries two names on two surfaces of the same tool. Verified
2026-07-26:

- the verb: `criteria add SLUG --text T` — `internal/verb/criteria.go:60`
  (`Usage`), `:62` (`fs.StringVar(&text, "text", …)`), refusing
  `criteria add requires --text` at `:86`;
- the corpus: `type criterionRow struct { Criterion string
  \`json:"criterion"\` }` — `internal/verb/import.go:71-72`;
- the column both write: `epic_criteria.criterion`
  (`internal/verb/criteria.go:97`).

So the storage has one name, and the two doors an adopter walks through
have two different ones — neither of which is the other's.

The cost was paid twice on this repository, once in each direction: an
import corpus written with `text` was refused `unknown field` (the JSON
decoder runs `DisallowUnknownFields`, `import.go:214`), and a later
`criteria add` written with `--criterion` was refused `flag provided but
not defined`. Each refusal was produced by the *other* surface's name,
and neither refusal named the surface it came from — the second is Go's
`flag` package text, which knows nothing about the corpus.

**The second failure cannot be fixed by a better message, and that
decides this proposal's shape.** Verified 2026-07-26: the dispatcher
parses flags at `internal/cli/dispatch.go:77` (`fs.Parse(remainder)`, on
a `flag.ContinueOnError` set built at `internal/cli/cli.go:125`), which
runs **before** the verb's `Run` and therefore before `criteriaAdd`
exists to say anything. An unknown flag dies in `flag.Parse`. So the
improved `criteria add requires --text` refusal at
`internal/verb/criteria.go:86` — the obvious remedy — is on a code path
`--criterion` never reaches. Any fix that leaves `--criterion` undefined
leaves the second incident exactly where it is.

This is the same family as #43 (ref-prefix conventions differ per verb)
and #29 (flags accepted but ignored): the vocabulary is per-surface
rather than per-tool. It is the cheapest member of that family to fix and
the one an adopter meets first, because `criteria` is what an epic's
acceptance rows are written with and the corpus is what a migration
writes them with.

One further asymmetry, measured while reading: md-table has **no
criteria section at all** (`knownColumns`, `import_mdtable.go:264-271`),
so an adopter authoring in md-table meets neither name. That is scope for
`guide-documents-the-format-and-the-remote`, which states it where the
reader picks a format; it is noted here only so this change is not read
as making md-table's criteria expressible.

## What changes

**One name is chosen and the other becomes an accepted alias that
redirects.** The change is symmetric in structure and the choice between
the two directions is the ratification fork:

- **Recommended: the corpus field becomes `text`**, matching the verb.
  Reason: the verb's name is the one typed interactively, repeatedly, by
  every adopter from the first hour onward; the corpus field is written
  once per migration, usually by an agent generating JSON from a
  template. Changing the surface with fewer, more deliberate authors
  costs less. `criterion` stays accepted as an alias.
- The opposite direction — the flag becomes `--criterion`, `--text`
  aliased — is the other branch, and it has one real argument in its
  favour: `criterion` is the column's name and the domain word, and
  `--text` says nothing about what the text is.

Whichever direction is ratified, the same three obligations hold, and
they are what makes this a change rather than a rename:

1. **`criteria add` accepts BOTH `--text` and `--criterion`, behaving
   identically.** Not a better error — an accepted flag. This is forced
   by the dispatcher measurement above: an undefined flag dies in
   `flag.Parse` before any selftracked code runs, so the mistyped form
   cannot be given a good message and can only be given a *meaning*. The
   general principle is worth stating because it is the reason: **where a
   tool has one field under two historical names, accepting both beats
   teaching which one is canonical** — the redirect an adopter needs is
   the command working, and the alternative here is not a worse redirect
   but no redirect at all. Both flags set the same value; supplying both
   at once with different values is a `usage` refusal.
2. **The old spelling keeps working on the corpus side too**, and the
   mechanism is named rather than assumed. `dec.DisallowUnknownFields()`
   (`internal/verb/import.go:214`) requires every accepted key to be a
   declared field, and two plain `string` fields cannot distinguish
   "present but empty" from "absent" — so the stated refusal of a row
   carrying *both* spellings is not expressible in the current data
   shape. **Decided: `criterionRow`'s two fields become `*string`.**
   Absent decodes to `nil`, present-and-empty to a non-nil pointer at
   `""`, which is exactly the distinction the both-keys refusal needs;
   `json.RawMessage` and a custom `UnmarshalJSON` both work and both cost
   more. Named because a proposal that specifies an outcome its own data
   shape cannot express is a proposal that gets reinvented at
   implementation time.
3. **Both refusals name the other surface.** `criteria add` with neither
   flag, and the corpus's unknown-field refusal, each state the tool-wide
   name and the surface the reader may have been thinking of. With
   obligation 1 in place this is no longer what closes the second
   incident — the alias is — but it still closes the first, and it is
   what a reader meets when they guess a third spelling.

**Spec §6.2's `criteria` row** carries the chosen signature. **Spec §10**
states the rule the fix instantiates: a field the importer accepts
carries the same name as the flag that writes it, and where history left
two, the importer accepts both and the refusals name both. Stated as a
rule because #43 and #29 are the same defect in other fields, and a rule
is what makes them findable.

**The migration guide's §4 JSON example** is updated to the chosen
spelling in the same change — it is the one place an adopter copies the
corpus shape from, and leaving it on the old name would make the alias
the de-facto primary.

## Alternative considered and rejected

**Document the mapping instead of changing either surface.** A line in
the generated contract and in the guide saying "the flag is `--text`, the
corpus field is `criterion`". Cheapest, breaks nothing, and rejected on
what was actually observed: both failures happened at the moment of
writing, in the tool's own refusal, to a reader who had the other
surface's name in mind — a reader who was, in both cases, working from
documentation they had already read. Documentation of a mismatch does not
survive the moment the mismatch is met; a refusal that names both
spellings does, because it arrives exactly then. Recorded because it is
the branch the epic's criterion 6 explicitly permits ("or the mapping is
stated where an adopter meets it"), so it is a legitimate owner choice
and not a strawman.

**Rename with no alias.** Rejected: it breaks every corpus already
written against the current shape — including the fixture corpus the
migration guide says it was walked against — for no gain over the alias,
which costs one decoder tag.

## Consequences accepted with this change

The tool carries an alias, permanently. Aliases accumulate, and a second
one on a different field would be the point at which "the vocabulary is
per-tool" needs a stronger mechanism than goodwill. Naming it now is
cheaper than discovering it at the third alias.

The chosen direction touches fixtures and the guide's example; a fixture
still asserting the old primary spelling is this change's failure signal
rather than a silent partial application.

`epic_criteria.criterion` — the column — does not change under either
direction. The dump's DDL is byte-equal before and after (§8.5's parser
checks it), so no dump migrates and no schema version moves. Stated
because "rename the field" reads like a schema change and is not one.

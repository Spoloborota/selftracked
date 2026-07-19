# Change: the §5 preamble's list of nullable columns is incomplete

Target: `docs/v0-spec.md` §5 preamble (revision 3.9 → 3.10)
Status: **accepted and applied** · 2026-07-19 · spec rev 3.9 → 3.10

## The defect

§5's preamble states:

> All tables `STRICT`; text columns `NOT NULL DEFAULT ''` unless stated; the
> only nullable columns are `tasks.dup_of` and `tasks.epic`.

The schema the same section defines contradicts this in two places, verified
against a database built from the specification's own DDL:

| Column | Nullable | Where the specification says so |
|---|---|---|
| `worklog.corrects` | yes | §5.7: `corrects INTEGER,` with the comment "NULL normally; a correction row names the seq it corrects" |
| `tasks.id` | yes | §5.5: `id INTEGER PRIMARY KEY AUTOINCREMENT` — SQLite leaves an INTEGER PRIMARY KEY nullable on the way in, filling it on insert |

`worklog.corrects` is the substantive one: it is deliberately nullable, the
specification says as much, and the preamble forgot it. `tasks.id` is a
property of SQLite's rowid aliasing rather than a design decision, but it is
still a nullable column, and a claim written as an exhaustive list should say
which kind of exhaustive it means.

## Why it matters rather than being a typo

The inventory carries this preamble as an obligation with a fixture:
"assert no other column accepts NULL". Written against the current schema
that fixture fails — correctly, because the schema is right and the sentence
is wrong. The row therefore cannot be honestly closed at any stage until the
sentence is fixed, and closing it by weakening the fixture would invert which
artifact is authoritative.

Found while implementing S1a, by a reviewer comparing the fixture's wording
against the schema it would run on.

## Correction to this proposal, found while presenting it

The first draft treated this as §5 forgetting a column. Deeper reading found
**§8.1 already lists all three** — "NULL only in `tasks.dup_of`/`tasks.epic`
and `worklog.corrects`". So the specification did not disagree with itself
about the schema; one of two sentences describing the same fact had gone
stale, and the other said which. A full audit also found three more columns
SQLite reports as nullable — `tasks.id`, `artifacts.id`, `events.seq` — the
rowid aliases, which accept NULL on insert (verified: the insert succeeds and
SQLite fills the value) and never store one.

That changed the fix from "add a name" to "say the true thing", because a
sentence listing declared-nullable columns cannot be written without an
exception list, and an exception list inside a fixture is where a future
reader weakens the check without understanding why it was there.

## Applied wording

§5 preamble now speaks of **stored** values rather than column declarations,
which makes it true without exceptions:

> No column ever **stores** NULL except `tasks.dup_of`, `tasks.epic` and
> `worklog.corrects`, where absence is the meaning… (Columns declared
> `INTEGER PRIMARY KEY` accept NULL on the way in — SQLite fills the rowid —
> but no stored row carries one, so a check written against stored values
> needs no exception list.)

§8.1 was reworded to match, since it describes the same fact from the
serializer's side and would otherwise be the next sentence to go stale.

## Consequence for the inventory

INV-053's fixture becomes writable as stated: three named columns accept
NULL and no other does, with `tasks.id` excluded by name and reason.

## Ratification

Owner, 2026-07-19, choosing between a minimal name-addition, a name-addition
with a rowid caveat, and the stored-values rewording: **"apply the third"** — the
third. The reasoning offered and accepted was that a claim needing an
exception list in its fixture is a claim that will be weakened later by
someone who does not know why the exception is there.

# Change: the §5 preamble's list of nullable columns is incomplete

Target: `docs/v0-spec.md` §5 preamble (revision 3.9 → 3.10)
Status: **proposed — awaiting owner review.** Not applied.

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

## Proposed wording

> All tables `STRICT`; text columns `NOT NULL DEFAULT ''` unless stated. The
> nullable columns are `tasks.dup_of`, `tasks.epic` and `worklog.corrects` —
> each nullable because absence is meaningful there: no duplicate, no epic,
> no row being corrected. (`tasks.id` is nullable on the way in as SQLite's
> rowid alias, and is never null once a row exists.)

## Consequence for the inventory

INV-053's fixture becomes writable as stated: three named columns accept
NULL and no other does, with `tasks.id` excluded by name and reason.

## Ratification

Awaiting the owner. The specification is not edited until then; INV-053 stays
`planned` and S1a closes with that row open rather than by adjusting the
fixture to match a sentence the schema contradicts.

# Change: instance-scoped events carry tokens, and R8 skips them by type

Target: `docs/v0-spec.md` §5.9 comment + §7 R8 (revision 3.14 → 3.15),
`internal/rules` r8, the S7 R8 inventory row
Status: **accepted** · raised 2026-07-19 at S5b implementation · applied
under D-EP14 (owner post-review)

## Why

§5.9 says events.entity "uses §4 grammar (verb-validated; R8)", and R8
demands grammar **and existence**. But the §5.9 event vocabulary includes
`paths` and `config` — instance-level operations with no §4 entity at
all: a dictionary row (`workdir@backend`) and a configuration key
(`idle_days`) have no reference form, and §4 deliberately does not give
them one (they are not tracked work). As written, the first `paths set`
or `config set` would plant an events row that R8 must flag — the
specification obliges the verbs to write events it then forbids.

## What changes

**§5.9 comment** — states that `paths` and `config` events are
instance-scoped: their entity column carries the affected token verbatim
(the `CLASS[@SCOPE] -> root` pair, the `key=value` pair) for the human
trail, not a §4 reference.

**§7 R8** — gains the carve-out: R8 checks grammar and existence for
entity-bearing events, and skips `paths` and `config` events by event
type — the closed vocabulary makes the skip precise, not a loophole
(every other event type remains fully checked).

**`internal/rules` r8** — implements the skip. The S7 R8 row's statement
gains the same clause at its stage-open re-read.

## Ratification

Applied under the owner's standing pre-authorization (D-EP14); subject to
post-review and revert.

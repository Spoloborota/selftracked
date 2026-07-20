# Change: `gate-skip` events join the R8 instance-scoped carve-out

Target: `docs/v0-spec.md` §7 R8 (revision 3.16 → 3.17), `internal/rules` r8,
the R8 inventory rows (INV-302, INV-137); exercised first at S8b
Status: **accepted** · raised 2026-07-20 at S8b open · applied under D-EP14
(owner post-review)

## Why

R8 demands every `events.entity` resolve by §4 grammar **and existence**.
The already-accepted `instance-scoped-events-and-r8` carved out `paths` and
`config` events, whose entity is an instance-level token (a dictionary row,
a config key) with no §4 reference form. `gate-skip` (§6.2) is exactly that
class: it records that the pre-commit gate was skipped on this machine — a
machine-level fact, not tracked work, with no task or epic to name. §4 gives
it no reference form, deliberately.

As written, R8 would flag the first converted `gate-skip` events row: the
spec obliges `gate skip-mark` + the next write verb to plant an events row
that R8 then forbids — the identical defect the paths/config carve-out
fixed, one event type wider. The event vocabulary is a closed CHECK list
(§5.9), so extending the skip to `gate-skip` stays precise: every
entity-bearing event type remains fully checked.

The gap surfaces only at S8b, where `gate-skip` events first exist — the
prior carve-out predated the `gate` verb — so it is filed now rather than
folded into the earlier change.

## What changes

- **§7 R8** — the carve-out reads "except `paths`/`config`/`gate-skip`
  events, instance-scoped by design". Spec rev 3.16 → 3.17.
- **§5.9 comment** — notes that `gate-skip`, like `paths`/`config`, is
  instance-scoped: its entity carries a fixed instance token (the skip's
  recorded moment is in `detail`), not a §4 reference.
- **`internal/rules` r8** — the type skip becomes
  `event NOT IN ('paths','config','gate-skip')`. An S8b fixture asserts a
  converted `gate-skip` row leaves R8 green.
- **INV-302** (the R8 verify-rule) and **INV-137** (events.entity grammar
  on write) gain the `gate-skip` clause; both stay closed at S7 for
  paths/config, and S8b adds the gate-skip fixture.

## Ratification

Applied under D-EP14; subject to owner post-review and revert.

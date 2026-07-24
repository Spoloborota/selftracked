# Change: the migration's pre-rename battery is the §8.5 load battery

Target: `docs/v0-spec.md` §8.6 (one clause in the migration sequence)
Status: **accepted** · raised at the S11 open (2026-07-24, recorded as a
watch-item in `docs/stage-openings/s11.md`) · filed before the deviating
code landed · ratified by the owner 2026-07-24 · applied under D-EP14
(spec revision 3.21 → 3.22)

## Why

§8.6 orders the migration sequence: build a fresh DB from DDL(N) in a
temp file → "Stage-0 + full verify" → atomic rename → reopen → and only
then, §8.4 permitting, re-serialize and update the sidecar. Full §7
verify includes R1 (the tracked dump byte-equals the DB's serialization)
and R14 (STATE.md byte-equals its render). At the point the sequence
places the verify, the tracked `dump.sql` still carries the OLD schema
version — it is rewritten only by the re-serialize step after the
rename. R1 therefore fails against a correctly migrated database every
time: the letter is unimplementable as ordered, for the same structural
reason `load` (§8.5) runs "Stage-0 verify plus the DB-only rule set"
rather than full verify against a database that does not yet own the
instance's derived files.

## What changes

- **§8.6** — the sequence clause "→ Stage-0 + full verify → atomic
  rename" becomes "→ Stage-0 + the §8.5 load battery (the DB-only rule
  set) → atomic rename". Full verify still covers the migrated instance
  at its next invocation — the pre-commit hook and any `verify` run —
  once the re-dump tail has rewritten the derived files it compares.
- No inventory rows ride along (the inventory retired at S10); the
  tracker record is story v0-bootstrap/S18's worklog.

## Consequences

The migration engine reuses `load`'s build path verbatim — one battery,
one code path, exercised by both doors (§8.5 untrusted dumps, §8.6
rebuilds), which is also what "the load path" in §8.6's own hydration
sentence already promises. The alternative — running full verify before
the rename — would require either writing the dump before the atomic
rename (breaking §8.3's interrupted-build-never-lands guarantee) or
special-casing R1/R14 to a vacuous pass inside migrations (a rule that
lies about what it checked).

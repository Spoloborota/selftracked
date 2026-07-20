# Change: `prime`'s `migrated` field moves from S8c to S11

Target: plan §4 (S8c / S11 definitions of done); INV-464 stage reassignment
(S8c → S11)
Status: **accepted** · raised 2026-07-20 at S8c open · applied under D-EP14

## Why

INV-464 is the §11.1 contract slot for `prime`'s `migrated` field: "present
only when this invocation performed a migration (§8.6)". Its inventory
verification reads: "trigger a migrating `prime` invocation, assert `migrated`
field present with vK→vN value; assert absent on a non-migrating invocation."

The first clause cannot execute at S8c. The migration engine — versioned
DDL(k), the loader whitelist(k), the row transform chain `T_k`, the escalating
read verb that takes EXCLUSIVE and rebuilds — is built at **S11** (INV-379
through INV-389). Nothing at S8c migrates, so `prime` can never emit `migrated`
to assert on. A verb-contract row whose positive verification needs machinery a
later stage builds cannot honestly close on the earlier stage.

INV-387 (§8.6: "`prime` reports `migrated:vK→vN` in its JSON") already lives at
S11 — it is INV-464's sourcing twin. Homing INV-464 beside it co-locates the
field's verification with the engine that makes the value observable.

This is the same shape as `r14-rides-its-renderer-at-s8c` (R14 rides to the
stage that builds its renderer) and `prime-divergence-rides-prime-at-s8c`
(INV-361 rides to the stage that builds `prime`): an obligation whose
verification needs later-stage machinery rides to that stage.

## What does NOT move

The contract **slot** is still built at S8c: `prime`'s output struct carries
the `migrated` field with `omitempty`, so it is absent on every S8c invocation
(none migrate) and S11 only has to populate it. The field's *shape* is part of
the §11.1 contract `prime` implements at S8c; only the row that **proves the
value** (a real migration emitting `vK→vN`) closes at S11.

Contrast INV-463 (`dump_requires_newer_binary`), which stays at S8c: that field
is derived by reading the tracked dump's header `schema_version` against the
binary's `schema.Version` — no migration engine, fully testable at S8c by
forging a newer header. `migrated` has no such static source; it requires an
actual migration event.

## What changes

- **INV-464** (`prime` field `migrated`) moves **S8c → S11**, beside INV-387.
  Its fixture — trigger a migrating `prime`, assert `migrated:vK→vN` present;
  assert absent on a non-migrating invocation — runs there.
- The contract slot (the `omitempty` struct field) is built at S8c with the
  rest of the `prime` contract; it simply never populates until S11.
- **Plan §4**: the S8c DoD drops the `migrated`-field close; the S11 DoD gains
  "`prime` reports `migrated:vK→vN` (INV-387, INV-464)". Plan rev 15 → 16.
- **Inventory distribution**: S8c 30 → 29, S11 25 → 26.
- **Spec unchanged**: `migrated` stays in the §11.1 contract text; only the
  inventory row's owning stage moves (the plan+inventory shape of
  `prime-divergence-rides-prime-at-s8c`, which left the spec intact).

## Ratification

Applied under D-EP14; subject to owner post-review.

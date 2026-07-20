# Change: `prime`'s divergence report moves from S8b to S8c

Target: plan §4 (S8b / S8c definitions of done); INV-361 stage reassignment
(S8b → S8c)
Status: **accepted** · raised 2026-07-20 at S8b open · applied under D-EP14

## Why

INV-361 is a **`prime`** contract: "`prime` performs the same divergence
comparison read-only and reports `"dump_divergence": true` (§11.1)". Its
verification runs `prime` and inspects the JSON. But `prime` does not exist
until S8c — every other `prime` row (INV-022, INV-023, INV-273, INV-454
through INV-466, INV-469, INV-470) is homed at S8c, where the verb is built.
A verb-contract row cannot close on a stage where the verb has no code.

S8b owns the divergence *machinery*, not `prime`'s reading of it: the §8.4
decision core (`divergenceCore`) landed at S5a and S8b completes the matrix
(the two-writer conflict, the sidecar convention, the file-sync advice).
`prime`'s read-only re-use of that core — reporting `dump_divergence`
without touching the DB — is `prime`'s own surface, and it lands with the
verb at S8c. Splitting the machinery (S8b) from the one verb that reports it
read-only (S8c) is the same shape as `r14-rides-its-renderer-at-s8c`: the
detector precedes the surface that reads it.

## What changes

- **INV-361** (`prime` reports `dump_divergence` read-only) moves
  **S8b → S8c**, where `prime` is built. Its fixture — diverge the dump, run
  `prime`, assert `dump_divergence:true` and no file mutated — runs there.
- The §8.4 machinery INV-361 reads (INV-351 sidecar convention, INV-362
  two-writer conflict, INV-003 loud single-writer surfacing) stays at S8b.
- **Plan §4**: the S8b DoD drops the `prime` divergence clause; the S8c DoD
  gains "`prime` reports `dump_divergence` read-only (INV-361)". Plan
  rev 14 → 15.
- **Inventory distribution**: S8b 35 → 34, S8c 29 → 30.

## Ratification

Applied under D-EP14; subject to owner post-review.

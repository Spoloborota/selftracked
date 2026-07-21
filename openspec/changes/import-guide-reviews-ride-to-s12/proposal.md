# Change: the two migration-guide reviews move from S9 to S12

Target: plan §4 (S9 / S12 definitions of done, prose unchanged — see below);
INV-449 and INV-450 stage reassignment (S9 → S12)
Status: **accepted** · raised 2026-07-21 at S9 open · applied under D-EP14

## Why

S9 builds the `import` verb. Twenty-eight of its thirty rows verify importer
*behaviour* — a corpus round-trips, a synthesized timestamp is events-marked,
a future date is refused — each closable by a fixture or a code review of the
importer at S9. Two do not:

- **INV-449** (`process`): "review: confirm **the migration guide** instructs
  pre-import reconciliation for such inconsistencies rather than importing
  them as-is."
- **INV-450** (`process`): "review: confirm **the migration guide** states
  these are excluded from `import`."

Both verify against the content of *the generic migration guide* — a document
that does not exist at S9. The guide is a §16 deliverable authored at **S12**:
INV-512 ("Deliverable: the generic migration guide") and INV-437 ("confirm a
standalone migration-guide document exists distinct from §10's generic text")
both close at S12, and INV-453 ("confirm the migration guide contains an
explicit boundary/scope section") sits there too. Plan §4's S12 line lists
"the generic migration guide" among that stage's deliverables and its DoD ends
"migration guide walked against the S9 fixture corpus" — the guide is written
at S12 and validated against the corpus S9 produces. A review of guide text
cannot honestly close on a stage that predates the guide.

The distinction is sharp against the neighbours that stay. INV-451 ("pointers
to non-file targets degrade to notes") and INV-452 ("owner steers with no
BLOCKED story import as notes, not unblock events") describe the same §10
migration boundary, but each verifies by a **test** of what the importer does
with such a row — genuine S9 behaviour. INV-449/450 have no importer code path
to assert on: "resolve inconsistencies before import" is a human reconciliation
step, and "prose registries do not migrate" is the absence of a path. Their
only evidence is the guide's words.

This is the shape of `migrated-field-rides-migration-at-s11`: an obligation
whose verification needs an artifact a later stage builds rides to that stage.
Here the artifact is the migration guide (S12) rather than the migration engine
(S11), and the rows are doc-reviews rather than a contract slot, but the
principle and the resolution are identical — home the row beside the artifact
its verification reads.

## What changes

- **INV-449** and **INV-450** move **S9 → S12**, beside INV-437 / INV-453 /
  INV-512 (the migration-guide deliverable rows). Their `review:` verifications
  run there, against a guide that exists.
- **Inventory distribution**: S9 30 → 28, S12 11 → 13.
- **Plan §4 prose is unchanged.** The S9 DoD (line 206) never named a guide
  review — it reads "Synthetic legacy corpus round-trip → `verify` green →
  golden dump; date-priority matrix incl. calendar-day warn; source-map
  determinism fixture," all importer behaviour. The S12 DoD (line 209) already
  ends "migration guide walked against the S9 fixture corpus," which subsumes
  confirming what that guide instructs and excludes. Only the plan's revision
  bumps (16 → 17) with a history entry; no DoD sentence needs rewording.
- **Spec unchanged.** §10's normative text (pre-import reconciliation; the
  what-does-not-migrate boundary) is untouched; only the inventory rows that
  *review the guide restating that text* change owning stage.

## Consequence for the reader

At S9 close, a fresh reviewer can re-run every remaining S9 row's verification
without a migration guide in the tree. At S12, when the guide is written, its
pre-import-reconciliation instruction and its excluded-classes boundary are the
two things INV-449/450 confirm — the guide is reviewed against obligations that
already have homes, rather than the reviewer inventing them.

## Ratification

Applied under D-EP14; subject to owner post-review.

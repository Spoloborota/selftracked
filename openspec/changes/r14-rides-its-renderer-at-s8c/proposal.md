# Change: R14 (STATE.md byte-equals its render) moves from S7 to S8c

Target: plan §4 (S7 / S8c definitions of done); INV-275 and INV-293 stage reassignment (S7 → S8c)
Status: **accepted** · raised 2026-07-20 at S7 open · applied under D-EP14

## Why

Plan §4 gives S7 the whole rule set: "`verify`: R1–R15 full". R14 is
"STATE.md byte-equals its DB-derived render" (INV-275), folded into R1 as
its third check (INV-293). Both need the **deterministic STATE.md
renderer** to run: a rule that compares a file to its render cannot exist
before the render function does.

That renderer is **INV-274** — "`state` regenerates STATE.md via
deterministic rendering: fixed sections, fixed ordering, last 10 events"
— and the inventory homes it at **S8c**, after S7. `init` writes the
first STATE.md at S8a (INV-414), also using that renderer; at S7 there is
no renderer and no canonical STATE.md at all. A red fixture for R14 at S7
would have to fabricate both the file and the renderer — i.e. build
INV-274's substance inside S7 and close a row S7 does not own. A verify
rule cannot precede the thing it verifies.

This is the same shape as the deferrals the plan already carries: S6's T8
("needs prime, joins the replay at S8c"), S4's skip-marker (writer verb
arrives at S8b, the fixture writes the file by hand), S5b's `epic:SLUG`
split (targets re-verify at S6 close). The difference is only that plan §4
did not pre-state this one, so moving it is an amendment rather than a
ledger note.

## What changes

- **INV-275** (R14 checks STATE.md byte-matches its DB-derived render) and
  **INV-293** (R1 check 3: STATE.md byte-equals its render) move
  **S7 → S8c**, where their renderer (INV-274) is built.
- **R1 splits by construction**: checks 1 and 2 — dump regenerated from
  the DB byte-equals `dump.sql` (INV-291), and the tracked dump reloaded
  and re-dumped byte-equally (INV-292) — stay at S7. Check 3 (STATE.md)
  rides S8c. `--fast` skips R1 wholesale (§7), so the split does not touch
  the pre-commit partition.
- **Plan §4** S7 DoD reads "R1 (checks 1–2), R2–R13, R15 full + `--fast`
  partition"; the S8c DoD gains "R1 check 3 / R14 (STATE.md byte-equals
  render), landed with the renderer". Plan rev 13 → 14.
- **Inventory distribution**: S7 38 → 36, S8c 27 → 29.

## Ratification

Applied under D-EP14; subject to owner post-review.

# Change: R13 counts live homes only — an archived home is history, not a home

Target: `docs/v0-spec.md` §7 (R13 row; revision 3.25 → 3.26),
`internal/verify/rules_fs.go` (the `r13` query), its tests. No schema
change, no verb change, no inventory motion.
Status: **accepted** · raised 2026-07-25 by task #33 (S5 campaign, block
C) · ratified by the owner 2026-07-25 on
`docs/research/2026-07-25-terminal-epic-and-archived-home-semantics.md`
revision 2, option A2

## Why

R13's promise is that an OPEN task has somewhere its narrative lives.
Its query has no `archived` clause (`internal/verify/rules_fs.go:303-305`),
so any home row satisfies it. R3 — the rule that keeps links honest — is
scoped to *non-archived* artifacts (§7 R3), because retiring an artifact
from existence checking is exactly what archiving is for.

The two exemptions compose into a state nobody chose: archive a task's
home, delete the file, and the OPEN task keeps a "home" that points at
nothing, with no signal from R13, R3, `prime`, `list`, `show`, `epic
show` or `stale`. Drilled 2026-07-25; the transcript is in the research
document. The schema states the invariant this breaks — "a dangling home
is unrecordable (the artifact must exist to be linked)" (§5.8) — which is
true at link time and unpreserved afterwards.

`link archive` already refuses a live home without `--force` (§6.2), so
archiving a home is a deliberate, force-flagged act. That gate protects
the *act*; it says nothing about the state that outlives it.

## What changes

- **§7 R13 row**: "Advisory: OPEN tasks with no **live** `home` link (an
  archived home is history, not a home)". Spec rev 3.25 → 3.26.
- **`internal/verify/rules_fs.go`** — `r13` joins `artifacts` and
  requires `archived = 0`. Nothing else about the rule moves: same scan
  target, same message, same advisory class, same full-only partition.
- **Tests**: a task whose only home is archived enters the census; a task
  holding one archived and one live home does not.

## Consequences accepted with this change

An existing repository that archived a home as routine hygiene sees new
advisory lines on its first `verify` after upgrading, with no state
change of its own. R13 never blocks anything, so the cost is attention,
not breakage. This repository holds no archived artifacts (dump
`c554c1c7aec2`), so its census does not move.

## What this change does NOT do

It does not make an archived home an error, does not touch `link
archive`'s `--force` gate, and does not extend R3 to archived artifacts —
that exemption stays, and is the reason this fix belongs on R13's side.

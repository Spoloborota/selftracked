# Change: the divergence recipe covers both directions and says how to tell them apart

Target: `docs/v0-spec.md` §8.4 (the refusal's named fix stops being
one-directional) and §11.3 (the skill's loop step 1); revision 3.36 →
3.37. `internal/scaffold/templates/PROMPT.md` — the working-loop
paragraph; `internal/scaffold/templates/claude_skill.md` — loop step 1;
this repository's byte-identical copies of both. Content tests. No schema
change, no verb change, no new verb call in the recipe.
Status: **accepted** · raised 2026-07-26 by task #45 under epic
`adoption-contract`, story S1 · review tier **FULL** (plan §5, D-EP7) ·
ratified by the coordinating agent 2026-07-26 under the owner's explicit 2026-07-26 grant of autonomy for this session · applied to the spec the same day

## Why

Divergence has two directions and every shipped text describes one.

`prime` reports `dump_divergence: true` when the tracked dump no longer
matches the sidecar (§8.4). What it does **not** report is which side is
authoritative, and the answer decides which of two irreversible moves is
correct:

- the **tracked dump** is the good side — a `git pull` brought another
  machine's newer state, and the local database is stale. The recovery
  discards the database: `load --force`.
- the **local database** is the good side — the working-tree dump was
  clobbered (a `git checkout` of the file, a merge resolution, a hand
  edit) while the database holds writes that never reached git. The
  recovery discards nothing and re-derives the dump from the database:
  `dump`.

Take the wrong branch and the loss is total in exactly one direction:
`load --force` on a good database destroys every unsynced local write.

Every shipped text names only the first. `internal/scaffold/templates/PROMPT.md:12-16`
(quoted verbatim):
"If it reports dump divergence, stop and reconcile before any write:
`load --force` replaces the local database with the tracked dump (it
prints what it discards first) — then re-apply any unsynced local writes
through verbs". `internal/scaffold/templates/claude_skill.md:8-13` says the
same in the loop's step 1. And the one-sidedness is not only in the generated text: **§8.4
itself** states the fix as `load --force` without qualification — "the
tracked dump changed under us (a `git pull`, a checkout) and writing
would clobber it with stale-DB-derived content — the verb **refuses**,
naming the fix: `selftracked load --force`". So this is a spec-level
statement, not a scaffold slip, which is why the amendment targets §8.4
rather than only the templates.

"Re-apply any unsynced local writes through verbs" is the sentence that
carries the whole hazard, and it is doing no work: by the time the reader
gets there, `load --force` has already discarded them.

Measured in a sandbox (#45, re-measured independently 2026-07-26 by the
coordinating agent because the task carried a conflict-of-interest flag —
it is the successor of #25, whose fix the same campaign authored): with
the tracked dump corrupted and the local database the good side, the
shipped recipe's first step **refuses** — `load --force` prints
`discarding local database: 1 task(s), 0 epic(s), 1 event(s)` and only
then fails with code `refused` on the dump it was asked to load. The
reader following the shipped recipe therefore learns, in this order, what
they were about to lose and that the recipe does not apply. Nothing
shipped tells them what to do next.

**One half of #45's claim did not reproduce and is deliberately not
proposed here.** The task originally claimed that recovering from the
database side leaves `STATE.md` stale and R1 red, so the recovery needs
an unscripted `state` call. Re-measured 2026-07-26 in a fresh sandbox:

```console
# database is the good side; only .selftracked/dump.sql was clobbered
$ selftracked dump && selftracked verify
verify full: 0 violation(s), 2 advisory        [exit 0]
```

`dump` alone is sufficient there, because `STATE.md` was already in step
with the database — the §6.1 write pipeline refreshes dump, then
`STATE.md`, then the sidecar
(`internal/verb/pipeline.go`), so the last local write left the pair
consistent. The withdrawn half stays withdrawn.

What the same session *did* measure, and what this change states instead:

```console
# both tracked files clobbered together — the git-checkout / merge shape,
# since dump.sql and STATE.md are both tracked and move together
$ selftracked dump && selftracked verify
FAIL R1: STATE.md does not match the database (stale projection); run selftracked state   [exit 1]
$ selftracked state && selftracked verify
verify full: 0 violation(s), 2 advisory        [exit 0]
```

R1 **already names its own repair** in the message. So the recipe needs
no extra scripted call and this change adds none; what it needs is for
the reader to run `verify` after the recovery and to be told that R1's
message is authoritative when it fires. That is the honest shape: one
step, plus a check whose failure carries its own instruction.

## What changes

**The generated recipe becomes two branches with a stated test between
them.** The test is the one an operator can actually run, and it is not a
verb — it is git:

- `git log -1 --stat -- .selftracked/dump.sql` and `git status
  --short .selftracked/dump.sql` say whether the tracked dump moved
  because someone else's commit arrived, or because this working tree
  changed it;
- `prime` (read-only, safe while diverged) says what the *database*
  holds — if it shows work the incoming dump could not know about, the
  database is the good side.

Stated as the rule the reader applies: **the good side is the one holding
work that exists nowhere else.** If the incoming dump carries another
machine's commits and this database holds nothing unsynced → the dump
wins. If this database holds writes that never reached a commit → the
database wins. If **both** hold unique work, that is the two-writer
accident §8.4 already calls out as loud-by-construction, and the recipe
stops there and says so rather than pretending either branch is safe.

- **Dump-is-authoritative branch** (unchanged): `load --force`, which
  prints what it discards before doing it.
- **Database-is-authoritative branch** (new): `selftracked dump` to
  re-derive the tracked dump from the database, then `selftracked verify`
  — and if R1 reports `STATE.md` stale, run the `state` the message names.
  Then the bookkeeping commit that stages the tracked pair (`git add
  .selftracked/dump.sql STATE.md`), because until that lands the
  divergence is unresolved on the git side.

**Spec §8.4** stops naming `load --force` as *the* fix and names the
decision first: the refusal names both recoveries and the test that
selects between them. The refusal text itself is not re-specified here —
what §8.4 gains is the statement that the choice exists and belongs to
the operator.

**Spec §11.3's loop step 1** gains the branch, pointing at the contract
rather than restating it.

## Alternative considered and rejected

**Have the tool decide.** `prime` could compare the tracked dump's
content against the database and report which side has rows the other
lacks. Rejected for this change: it is a new analysis surface on the
read path (a full parse of an untrusted dump — §8.5 — inside the
session-start hook), and "which side is authoritative" is not always a
function of row counts; a legitimate `load --force` discards rows on
purpose. The operator's judgement is what the recipe is for, and the
measured defect is that they were never told a judgement was needed.
Recorded because it is the branch that removes the human step, and
because a future version may want it once the two-writer detection
§8.4 mentions is a first-class thing rather than a described symptom.

**Add a scripted `state` call to the database-side branch.** This is
#45's own second half, and it is rejected on the measurement above: it is
unnecessary in the single-file case and redundant in the two-file case,
where R1's message already names it. Adding it would put a step in the
recipe whose failure mode (running `state` when nothing needed it) is
silent and whose success is indistinguishable from doing nothing.

## Consequences accepted with this change

The recipe stops being one command and becomes a decision. That is a real
cost: the previous text could be followed without understanding, and the
new one cannot. It is accepted because the previous text could also be
followed into total loss of unsynced work, and because the decision is
one an operator is uniquely able to make and the tool is not.

The database-side branch's `verify` step can report R1 red on a first
run, by design. A reader who treats a red `verify` as failure rather than
as the next instruction will stop one step early; the recipe therefore
states the expected R1 line and its repair inline, so the red is
recognized as part of the procedure and not as a new problem.

The two-writer case is named and not solved. §8.4 already accepts that
posture — no merge driver ships, deliberately — and this change does not
change it; it stops the recipe from silently walking into it.

Corroborated in passing during the same measurement and **not** in scope
here: `load --force` announces the discard before validating the dump it
was asked to load (task #44). This change makes the recipe avoid the
situation more often; it does not fix the ordering.

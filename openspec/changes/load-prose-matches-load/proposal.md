# Change: the prose about `load` matches the built `load`

Target: `docs/v0-spec.md` §8.4 + §11.1 (revision 3.18 → 3.19); the §8.4
refusal message in `internal/verb/pipeline.go`; a stale comment in
`internal/load/verb.go`; inventory row INV-456 (re-worded, re-verified)
Status: **accepted** · raised at the S8c close (spec-fidelity critic,
2026-07-20) and widened by the 2026-07-24 critic round · ratified by the
owner 2026-07-24 (fork analysis F2 → α, expanded scope) · applied under
D-EP14

## Why

Two spec sentences say the fallback `load` "fast-forwards" a behind DB
(§8.4's refusal advice and §11.1's chain description). The built `load`
builds a DB only when none exists and refuses ANY existing DB without
`--force`; a present-but-behind DB is surfaced by `prime`'s
`dump_divergence` flag and never reaches `load`. The chain is correct —
the prose is not, and the 2026-07-24 round showed the falsehood is live,
not latent:

- The §8.4 divergence refusal (`internal/verb/pipeline.go`) instructs the
  agent to "run selftracked load (fast-forward to the pulled dump)" — a
  command that then refuses because the DB exists. A live two-refusal
  loop on the common post-pull path.
- INV-456 (S8c, verified-by-command) restates the fast-forward claim, and
  its fixture only ever exercised the refusal half.
- `internal/load/verb.go` still carries "the full §8.4 divergence matrix
  arrives at S8b" — that matrix landed in the write pipeline, and the
  comment now misdirects readers.

The alternative — teaching `load` to actually fast-forward — was rejected
at the S8c adjudication and again here: it duplicates what the chain does
via `prime` and opens a second sync path beside the §8.4 matrix.

## What changes

- **§8.4** — the refusal advice names the real command:
  `selftracked load --force` (the §8.3 discard floor, which prints what
  it discards first), then re-apply unsynced local writes through verbs.
- **§11.1** — the chain description reads: `load` builds the DB when none
  exists and refuses any existing one; a behind DB is `prime`'s
  `dump_divergence` case and never reaches `load`. Spec rev 3.18 → 3.19.
- **`internal/verb/pipeline.go`** — the divergence refusal message
  matches the new §8.4 sentence (names `--force`).
- **`internal/load/verb.go`** — the stale "arrives at S8b" comment states
  where the matrix actually lives.
- **INV-456** — re-worded to the built behavior; same fixture obligation,
  re-verified (`make gates`) at the applying commit.
- Post-v0 note recorded in the fork-analysis document, not spec scope:
  `dump_divergence` is a boolean and does not distinguish
  behind-with-nothing-to-lose from true divergence.

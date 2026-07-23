# Change: the §5 backfill parenthetical names what `--legacy` actually relaxes

Target: `docs/v0-spec.md` §5 preamble (revision 3.20 → 3.21); inventory
row INV-056 (re-worded, same obligation)
Status: **accepted** · raised as S9 close escalation E1 (2026-07-21) ·
resolved by the 2026-07-24 critic round (fork analysis D2, expanded to the
spec after the spec-fidelity critic found the ambiguity's source) ·
ratified by the owner 2026-07-24 · applied under D-EP14

## Why

S9's escalation E1 asked whether an explicit `date` field requires
`--legacy`. §6.2 answers no — its relaxation list is closed (synthesized
timestamps, `legacy:` commits, terminal INSERTs), the explicit field is
ordinary source (2) in the date chain for any import, and the restrictive
reading was empirically tried during S9 and broke legitimate non-legacy
batch creation (RC-1). The fork analysis first located the ambiguity in
INV-056 alone; the critic round showed INV-056 merely copies the spec's
own §5 preamble sentence — "the one backfill path is `import --legacy`,
whose backfilled timestamps — git-derived or explicit-field, §6.2 — are
events-marked" — which reads as if git-derived and explicit-field dates
were `--legacy` features. The fix must land at the source.

## What changes

- **§5 preamble** — the parenthetical states the actual split: the one
  backfill door is `import` (§6.2); a row's date may derive from cited
  commits or an explicit date field on any import; `--legacy` alone
  additionally admits synthesized timestamps; imported rows are
  events-marked either way. Spec rev 3.20 → 3.21.
- **INV-056** — re-worded to the same split (git-derived, explicit-field,
  or — under `--legacy` only — synthesized; all events-marked); the
  obligation and its fixture are unchanged, re-verified (`make gates`) at
  the applying commit.
- S9 escalations close on the ledger: E1 resolved by this amendment;
  E2 accepted as shipped (fork analysis D3); E3 stands as the S10
  authoring rule (fork analysis D4).

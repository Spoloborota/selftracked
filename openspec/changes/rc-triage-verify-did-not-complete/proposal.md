# Change: the pre-commit's rc-triage tells the truth about an incomplete verify

Target: `docs/v0-spec.md` §9 generated pre-commit script (revision
3.19 → 3.20); `internal/scaffold/templates/hooks/pre-commit` + its golden;
`internal/scaffold/hookscript_test.go` (one new case); inventory rows
INV-428/INV-429 (re-worded, re-verified)
Status: **accepted** · raised at the S8b close (shell-robustness critic,
2026-07-20; parked as a spec-wording note) · reshaped by the 2026-07-24
critic round · ratified by the owner 2026-07-24 (fork analysis F3 → β,
honest-message form) · applied under D-EP14

## Why

The §9 script triages `verify --fast` exit codes as: rc=2 → "could not
run (busy/corrupt/env)", any other non-zero → "verify RED …
SELFTRACKED_SKIP=1 bypasses ONCE". The binary itself only ever exits
0, 1, or 2 — so a shell-observed rc outside that set (130/137/143, a
signal death) can only mean the run did not complete. Today that lands in
the RED branch: a false diagnosis with a bypass hint attached, shown to
operators who act on messages. The 2026-07-24 round also caught the naive
fix lying in the other direction — routing an interrupt to
"busy/corrupt/env — fix the environment" misdiagnoses rc=130 just as
badly. Hence the honest-message form: RED is exactly rc=1; everything
else non-zero "did not complete", is not bypassable, and says re-run.
An undocumented exit code landing in a not-bypassable re-run branch is
the safe failure mode; landing in a bypassable RED is not.

## What changes

- **§9 script** — the triage becomes: `rc=1` → the RED branch (message
  unchanged: points at `selftracked verify`, notes the recorded one-shot
  bypass); any other non-zero rc → "verify did not complete (rc=$rc —
  interrupted or environment failure) — re-run; not bypassable as RED".
  Both branches still exit 1. Spec rev 3.19 → 3.20.
- **`internal/scaffold/templates/hooks/pre-commit`** and the golden —
  the same script verbatim (§9 ships verbatim by design).
- **`internal/scaffold/hookscript_test.go`** — the rc=2 case keeps its
  "not bypassable as RED" assertion (the phrase survives); a new rc=130
  case asserts the did-not-complete branch.
- **INV-428** — re-worded: any rc outside {0,1} (environment failure or
  an interrupted/killed run) exits 1 with the did-not-complete message,
  not bypassable as RED.
- **INV-429** — re-worded: rc=1 (RED) exits 1 with the bypass-hint
  message. Both rows re-verified (`make gates`) at the applying commit.

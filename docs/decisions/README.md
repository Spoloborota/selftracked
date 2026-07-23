# docs/decisions/ — the `adr` class

Architecture Decision Records. One file per decision, numbered, immutable
once accepted (a reversal is a NEW ADR that supersedes, never an edit).

- **Class:** `adr` (default scope), root `docs/decisions`.
- **Contract:** durable, tracked, non-ephemeral — linked ADRs must resolve
  on disk (`verify` R3).
- **Template:** copy `_template.md` to `NNNN-short-title.md`.

Link an ADR with
`selftracked link <id|epic:SLUG> adr:<relpath> --role adr` (or
`decision-package`).

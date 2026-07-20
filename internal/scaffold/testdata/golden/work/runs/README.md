# work/runs/ — the `run` class

Per-run output: logs, generated artifacts, transcripts from a single
execution. Ephemeral by design.

- **Class:** `run` (default scope), root `work/runs`, **ephemeral** —
  `verify` does not require these to resolve.
- **Cleanup:** manual until `gc` ships (see `work/README.md`).

Link with `selftracked link <id|epic:SLUG> run:<relpath> --role run`.

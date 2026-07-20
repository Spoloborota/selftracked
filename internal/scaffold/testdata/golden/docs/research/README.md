# docs/research/ — the `research` class

The evidence base. Every non-trivial decision should be able to cite a
document here: benchmarks, comparisons, spikes, external reading distilled.

- **Class:** `research` (default scope), root `docs/research`.
- **Contract:** durable, tracked, non-ephemeral — `verify` (R3) expects
  every linked research artifact to resolve on disk.
- **Naming:** date-bearing filenames take their date from `date`, never
  from session narrative (a wrong date baked into a filename is permanent).

Link a research doc to a task or epic with
`selftracked link <id|epic:SLUG> research:<relpath> --role origin-research`
(or `evidence`, `grounding`).

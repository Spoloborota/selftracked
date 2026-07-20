# work/reports/ — the `report` class

Reports meant to be read: analyses, reviews, summaries produced during the
work and kept as a durable output.

- **Class:** `report` (default scope), root `work/reports`, non-ephemeral
  — a linked report must resolve on disk (`verify` R3).
- Distinct from `run` (raw per-run output): a report is a curated artifact.

Link with `selftracked link <id|epic:SLUG> report:<relpath> --role report`
(or `output`).

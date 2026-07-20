# AGENTS.md

This repository tracks its own work with **selftracked**. Whatever harness
you run under:

- **Read the state first.** Run `selftracked prime --json` at the start of a
  session for the current epics, ready work, blocked questions, and totals.
  `STATE.md` is the human-readable projection.
- **Change state only through verbs.** Never touch `.selftracked/db.sqlite`
  or hand-edit `.selftracked/dump.sql`. See `PROMPT.md` for the full rule
  and the verb catalog.
- **Never answer product-owner questions.** Raise them (`story block
  --reason`, an IN-REVIEW task) instead.

Full instructions: `PROMPT.md`.

# selftracked: state changes only through verbs

This repository's work is tracked in `.selftracked/`. Obey these rules:

- **Never** run `sqlite3` against `.selftracked/db.sqlite`, and **never**
  hand-edit `.selftracked/dump.sql` or `STATE.md`. All state changes go
  through `selftracked` verbs (see `PROMPT.md`). Raw reads are allowed but
  the read verbs (`list`, `show`, `log`, `prime`) are cheaper.
- **Never answer a product-owner decision yourself.** Move the question
  task to `IN-REVIEW` and `story block --reason` the affected story; the
  owner answers.
- **End every session with a bookkeeping commit** so the dump refreshed by
  your last write reaches git. Stage it explicitly — `git add
  .selftracked/dump.sql STATE.md && git commit` — because when the index
  starts empty, git refuses the commit even though the pre-commit hook
  stages the refreshed pair.

Configuration lives in `meta` rows edited via the `config` verb — there is
no config file.

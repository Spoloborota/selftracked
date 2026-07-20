# work/ — the `workdir` class (and its siblings)

Scratch space and outputs of the work: working directories, run outputs,
reports. Two of the three classes seeded here are **ephemeral** — `verify`
does not require an ephemeral class's artifacts to resolve on disk, because
they come and go — while `report` is durable, so a linked report must
still resolve.

- **Classes seeded here:** `workdir` (root `work`, ephemeral), `run` (root
  `work/runs`, ephemeral), `report` (root `work/reports`, non-ephemeral).
- **Cleanup:** removing stale ephemeral artifacts is **manual for now** —
  the `gc` verb that would prune them is deferred (see the roadmap). Delete
  what you no longer need; nothing here is a durable record.

## Opt-in classes

The seed is deliberately minimal (five classes). Register more with
`selftracked paths set <class>[@scope] <root>` only when a project needs
them. Recommended opt-in classes with conventional roots — **not**
pre-registered:

- `runbook` — operational runbooks.
- `guide` — how-to guides and onboarding docs.
- `rfc` — request-for-comments / design proposals.
- `src` — a source root, so `stale` can watch code files.
- `external` — vendored or third-party references.

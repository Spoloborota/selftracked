# Change: task and epic readers print their linked artifacts

Target: `docs/v0-spec.md` §6.2 (`show` verb row) (revision 3.24 → 3.25)
Status: **accepted and applied** · 2026-07-24 · owner-directed (task #18)

## The gap

R3 guarantees every artifact link resolves on disk, and `link` refuses an
escaping relpath so retention can reason about roots — the write half is
guarded three ways. The read half did not exist: neither `show <id>` nor
`epic show SLUG` printed linked artifacts, in text or in `--json`
(verified empirically 2026-07-24: `epic show --json` keys were
`criteria/epic/goal/status/stories/worklog`; `task show --json` keys were
`epic/note/parked/ref/status/title`). A linked ADR or research document
was reachable only by scanning `log <ref>` events or browsing the tracked
directories — ADR 0001, linked to the v0-bootstrap epic as `adr`, was
invisible to every reader the tracker offers.

The spec asked for the reverse lookup only (`show <artifact>` lists its
tasks/epics); it never asked for the forward one. Under the governance
rule that unrequested scope needs an amendment, this proposal adds the
forward reader to the spec before the code lands.

## The addition

The §6.2 `show` row gains the forward lookup: `show <id>` and
`epic show SLUG` (text and `--json`) list the entity's linked artifacts as
`class[@scope]:relpath (role)`, archived links marked
`(role, archived)`, ordered by class, scope, relpath (deterministic). An
entity with no links prints nothing in text and an empty array in JSON —
the same posture as `epic show`'s other sections.

## Why it matters for the pilot

A reader that hides an entity's grounding documents undercuts the tracker's purpose.

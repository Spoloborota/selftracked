# Change: the migration gate's mechanics — lock posture, sentinel, and the snapshot

Target: `docs/v0-spec.md` §3.1 (one carve-out sentence), §8.6 (three
mechanics clauses)
Status: **accepted** · raised by the S11 close review (three-critic round,
2026-07-24) · ratified by the owner 2026-07-24 · applied under D-EP14
(spec revision 3.22 → 3.23)

## Why

S11 built §8.6's escalation and the close critics found four mechanics
the spec either contradicts or does not cover. Each is already in the
shipped code, each was forced by evidence, and none may live as a code
comment (non-negotiable 1):

1. **§3.1 says write connections use `locking_mode(EXCLUSIVE)` +
   `synchronous(FULL)` and every connection carries `busy_timeout(5000)`.
   The migration-lock connection deliberately uses neither.** Empirically
   (reproduced twice during S11, at the driver level): SQLite's busy
   handler holds the SHARED lock it already acquired while waiting, so
   two racing gates — or a gate against a committer — deadlock until a
   timeout; and under `locking_mode=EXCLUSIVE` every lock a connection
   touches is sticky-until-close, so a FAILED `BEGIN` keeps its SHARED
   half and two racing gates starve each other permanently. The shipped
   gate therefore acquires with fail-fast `BEGIN EXCLUSIVE` attempts
   (`busy_timeout=0`, jittered retries, the 5000 ms budget preserved at
   the loop level, final BUSY = exit 2 with a retry hint). `synchronous`
   stays at the driver default: the only durable write this connection
   makes is the sentinel mark, whose loss on crash is safe (the next
   verb re-migrates).
2. **The sentinel protocol is how "the loser re-checks user_version
   after the wait" actually works.** The winner marks the superseded
   database file with a negative `user_version` (`-from`) inside its
   transaction before releasing the lock and renaming; a loser that
   blocked on the lock sees the mark on its own handle. A persisting
   mark (crash between commit and rename, or a swap in flight) makes
   every verb refuse exit 2 with a retryable message naming
   `load --force` as the last-resort rebuild; a FAILED rename restores
   the mark to `from` so the next verb simply re-migrates —
   non-destructive recovery, never a forced discard.
3. **A database ahead of the binary refuses forward-only, same as a
   dump.** §8.6 names only the dump header as the refusal trigger; a
   `user_version` above the binary's N (a crashed newer-binary
   migration plus a downgrade) fails closed with the same
   needs-newer-binary refusal rather than operating a schema this
   binary has never seen.
4. **The migration tail's "§8.4 check" is the sidecar arm only,
   snapshotted before the swap.** §8.4's second arm
   (regenerate-and-compare) needs the OLD version's serializer, which
   the binary does not carry — after the swap the old serialization is
   gone, and the current serializer would call every migration "an
   external change". So: tracked-dump-matches-sidecar, read before the
   migration starts, decides the re-dump. A crash-residue mismatch
   therefore stays DB-side too — safe (nothing is overwritten), and it
   reconciles through `load --force` plus the next migration's re-dump.

## What changes

- **§3.1** — one carve-out sentence after the write-connection posture:
  the §8.6 migration-lock connection uses the base pragmas with
  fail-fast lock acquisition (why: busy-handler deadlock and
  EXCLUSIVE-mode lock stickiness, observed empirically; the 5000 ms
  budget and the BUSY-exits-2 contract are preserved).
- **§8.6** — the escalation sentence gains the sentinel mechanics (the
  mark, the retryable refusal, the rename-failure restore); the
  forward-only paragraph gains the DB-ahead refusal; the tail sentence
  names the §8.4 check as the pre-snapshotted sidecar arm with the
  serializer(k) reason.
- No inventory rows (retired at S10); the tracker record is story
  v0-bootstrap/S18's worklog.

## Consequences

The spec stops promising a lock posture the engine cannot use and a
divergence check the binary cannot compute, and the two new user-visible
states (the sentinel refusal, the DB-ahead refusal) become contract
rather than surprise. The alternative — keeping §3.1/§8.4 as written —
would leave the shipped, empirically-forced behavior documented only in
code comments, which is the exact failure mode non-negotiable 1 exists
to prevent.

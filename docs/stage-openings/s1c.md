# S1c opening record

Stage: S1c — driver behaviour (plan §4). Opened 2026-07-19, per D-EP13.
Spec revision at open: 3.13. Plan revision at open: 12.
Rows owned at open: 10. After the open: 9 at S1c — one moved (below).

S1c's machinery is the schema package plus the pinned driver
(modernc.org/sqlite v1.54.0), driven directly; there are still no verbs, no
serializer, no CLI. The §16 re-verification items' documented methods are in
`docs/research/2026-07-18-sqlite-advanced-features.md`; the Serialize API
was located in the driver source before this record was written (unexported
`*conn` methods, reachable via `sql.Conn.Raw`).

**Resolved verification command.** Unless a row's line says otherwise, the
fixture resolves to `go test ./internal/schema -run 'TestDriver/<fixture>'`.

## Rows moved (placement defects)

| Row | To | Why |
|---|---|---|
| INV-010 | S7 | Its obligation is "`verify` additionally runs `foreign_key_check`" — a verify-rule needing the `verify` verb, built at S7. The DSN half (FKs on for every driver connection) is already `verified-by-command` at S1a (`TestForeignKeysAreOnForEveryPooledConnection`), and the FK-off tier is INV-170's fixture here. |

## Per-row verdicts

| Row | Content | Placement | Verification (resolved) |
|---|---|---|---|
| INV-010 | ok | moved → S7 | carried with the row |
| INV-011 | ok — the two bypass vectors §1.1 names | ok | trigger-bypass-vectors-reproducible: (a) INSERT-path write of a terminal state lands (no trigger on INSERT by design); (b) with `recursive_triggers` unset on a session, `INSERT OR REPLACE` deletes through the delete trigger. Documents the limit — both writes must SUCCEED |
| INV-028 | ok | ok — S1a's TestEveryConnectionCarriesTheRequiredPragmas covers the assertion | `go test ./internal/schema -run 'TestEveryConnectionCarriesTheRequiredPragmas'`; the close review confirms all five values are individually asserted there, else the gap is filled here |
| INV-168 | ok — the stated gap is the obligation | ok | raw-update-leaves-stale-note-passes-trigger: IN-REVIEW exit via raw UPDATE keeping the stale non-empty note SUCCEEDS — non-emptiness is the trigger's whole tooth |
| INV-170 | ok | ok | fk-violations-pass-when-pragma-foreign_keys-off: an orphan FK insert SUCCEEDS on a pragma-free connection (the tier boundary §1.1 states); the FK-on rejection is S1b's orphan fixtures |
| INV-173 | ok — same stated-gap shape as INV-168 | ok | deliberate-raw-insert-bypasses-history-invariants: a terminal-state task INSERTed with no events row SUCCEEDS; the detection half is R12's rows at S7 |
| INV-502 | ok | ok | serialize-deserialize-roundtrip-full-schema: Serialize a populated DB, Deserialize into a second connection, assert object list and row contents match; then Serialize again and assert byte-identity |
| INV-504 | ok | ok | extended-result-codes-distinguishable: violate an FK and a CHECK, unwrap to `*sqlite.Error`, assert `.Code()` yields the distinct extended constraint codes the §6.1 exit-mapper will need |
| INV-505 | ok | ok | recursive-triggers-replace-regression: with the DSN's `recursive_triggers(1)`, `INSERT OR REPLACE` on a no-delete table is REFUSED by the delete trigger; the OFF half is INV-011's vector (b) |
| INV-506 | ok | ok | returning-via-query-path: `QueryRow(... RETURNING id)` yields the inserted id on the pinned driver (S1b's seed helpers already lean on this; the probe asserts it explicitly and on its own) |

## What this open did not do

No code was written; the close review should sample these verdicts rather
than trust them. INV-028's "already covered" claim is exactly the kind of
assertion the close review re-checks against the S1a test's actual body.

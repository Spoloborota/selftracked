# Change: sanity bounds on imported dates that cannot be derived

Target: `docs/v0-spec.md` §6.2 `import` (revision 3.10 → 3.11)
Status: **accepted** · raised 2026-07-19 · applied same day

## Why

The import contract dates worklog rows from git where a commit is cited.
Task rows carry no commit, so their dates come from an explicit field or from
the import time — and the specification states that as a limitation.

The obvious way to close it is to derive a task's date the way the worklog's
is derived: ask git when the task's line first appeared in the source file
being imported. That was tried against a real corpus before proposing it, and
it does not work.

On the first repository tested, twenty tasks out of twenty produced a date —
and all twenty produced the *same* date, because the source file's path had
been created by a vocabulary-rename commit. Git's history for that path
begins there; `--follow` does not bridge the gap, because the content changed
too much for rename detection. The failure is not particular to that
repository: a project that keeps a markdown backlog well enough to import has
sweeps — renames, reformatting, terminology changes — and every sweep resets
the apparent age of the lines it touches, sometimes of the whole file. The
method is systematically biased late exactly where it would be used.

So the derivation is abandoned. What survives is cheaper and honest: the
importer cannot compute the right date, but it can refuse an impossible one.

## What changes

§6.2's `import` entry gains two bounds on any date the importer did not
derive from git:

- **A date later than the import moment is refused.** The modal contamination
  this guards against is documented: a session that crossed midnight wrote
  tomorrow's date into records all day. A future date is not a judgement
  call — it is provably not an event date.
- **A date earlier than the earliest commit touching the source file is
  reported.** It is not refused, because a genuinely older record can be
  transcribed into a new file, but it is stated rather than absorbed
  silently, along with the bound it violated.

Neither computes a correct date. Both are stated as bounds, so nobody reads
them as the derivation this change explicitly abandons.

## Consequence for the inventory

Two obligations, each independently fixturable, so each becomes a row: the
future-date refusal, and the too-old report.

## Ratification

Owner, 2026-07-19, on the recommendation to accept the limitation and add the
two bounds rather than pursue a derivation the evidence had just falsified:
"do the recommended thing".

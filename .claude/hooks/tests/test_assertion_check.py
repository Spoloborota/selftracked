#!/usr/bin/env python3
"""Corpus test for the assertion-provenance hook.

Every FLAG case below is a claim this project actually made without running a
command first, or an exact shape of one. A detector that does not catch its
own history has no right to nag about anyone else's.

Run:  python3 .claude/hooks/tests/test_assertion_check.py
"""
import datetime
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from assertion_check import analyse  # noqa: E402

TODAY = datetime.date(2026, 7, 19)
NO_TOOLS: list[str] = []
WITH_TOOLS = ["Bash"]


class RealFailures(unittest.TestCase):
    """Claims published unverified. Each must be flagged when nothing ran."""

    def test_unverified_finding_count(self):
        # Went into a document's revision history; no count was ever produced.
        self.assertIsNotNone(analyse(
            "Five-lens critic round producing 45 findings, 12 of them HIGH.",
            NO_TOOLS, TODAY))

    def test_row_total_from_memory(self):
        self.assertIsNotNone(analyse(
            "The inventory holds 545 rows and every stage is covered.",
            NO_TOOLS, TODAY))

    def test_file_line_citation(self):
        self.assertIsNotNone(analyse(
            "The bucket check lives at scripts/check-inventory.py:88.",
            NO_TOOLS, TODAY))

    def test_existence_claim(self):
        self.assertIsNotNone(analyse(
            "A progress ledger exists at the repository root.",
            NO_TOOLS, TODAY))

    def test_counted_obligations(self):
        # The shape of "all seven items are placed correctly" — which was wrong.
        self.assertIsNotNone(analyse(
            "All 7 obligations are placed on the stage that can run them.",
            NO_TOOLS, TODAY))

    def test_future_event_date_flagged_even_with_tools(self):
        # A date past the wall clock is provably wrong; tools cannot rescue it.
        self.assertIsNotNone(analyse(
            "Ratified by the owner on 2026-07-25.", WITH_TOOLS, TODAY))


class BypassesFoundByReview(unittest.TestCase):
    """Defeats found by a review of the first version. Each was silent then."""

    def test_question_mark_elsewhere_does_not_silence_the_claim(self):
        # A trailing question once suppressed every claim in the reply.
        self.assertIsNotNone(analyse(
            "The inventory holds 545 rows and every stage is covered. "
            "Shall I proceed to S1?", NO_TOOLS, TODAY))

    def test_second_question_form(self):
        self.assertIsNotNone(analyse(
            "There are 612 rows in the inventory now. Why does that matter?",
            NO_TOOLS, TODAY))

    def test_existence_verb_outside_the_first_verb_list(self):
        self.assertIsNotNone(analyse(
            "The dispatcher sits at cmd/selftracked/main.go, ready to wire up.",
            NO_TOOLS, TODAY))

    def test_file_line_with_unlisted_extension(self):
        self.assertIsNotNone(analyse(
            "The bug is in src/handler.rs:42, in the retry loop.",
            NO_TOOLS, TODAY))

    def test_small_count_with_unlisted_noun(self):
        self.assertIsNotNone(analyse(
            "3 bugs were found during the walkthrough.", NO_TOOLS, TODAY))

    def test_spelled_out_count(self):
        self.assertIsNotNone(analyse(
            "Twelve rules remain unverified.", NO_TOOLS, TODAY))


class MustStaySilent(unittest.TestCase):
    """Legitimate replies. A nag here would train the reader to ignore it."""

    def test_same_claim_with_evidence(self):
        self.assertIsNone(analyse(
            "The inventory holds 545 rows and every stage is covered.",
            WITH_TOOLS, TODAY))

    def test_explicitly_marked_unverified(self):
        self.assertIsNone(analyse(
            "The inventory holds 545 rows ? (from earlier context).",
            NO_TOOLS, TODAY))

    def test_identifiers_are_not_measurements(self):
        self.assertIsNone(analyse(
            "INV-503 moved to S4 under D-EP6; see §8.4 and rule R11.",
            NO_TOOLS, TODAY))

    def test_versions_and_years(self):
        self.assertIsNone(analyse(
            "Plan revision 6 supersedes rev 5; gitleaks v8.30.1 shipped in 2026.",
            NO_TOOLS, TODAY))

    def test_dated_filename_is_a_citation(self):
        self.assertIsNone(analyse(
            "See docs/research/2026-07-25-notes.md for the write-up.",
            NO_TOOLS, TODAY))

    def test_past_dates_pass(self):
        self.assertIsNone(analyse(
            "The amendment was accepted on 2026-07-19.", NO_TOOLS, TODAY))

    def test_git_sha_is_not_a_measurement(self):
        self.assertIsNone(analyse(
            "Published at 7de275b on the main branch.", NO_TOOLS, TODAY))

    def test_prose_without_claims(self):
        self.assertIsNone(analyse(
            "The amendment flow puts the proposal before the code, always.",
            NO_TOOLS, TODAY))


if __name__ == "__main__":
    unittest.main(verbosity=2)

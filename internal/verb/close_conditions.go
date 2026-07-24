package verb

import (
	"context"
	"database/sql"
	"fmt"
)

// closeBlockers evaluates §6.4's six refusal conditions and returns the
// COMPLETE list — a failed close names every blocker at once, not the
// first. It also runs `criteria check` (condition 3), whose per-criterion
// output rides back for printing on success.
func closeBlockers(ctx context.Context, tx *sql.Tx, slug string) ([]string, []string, error) {
	blockers, err := storyConditions(ctx, tx, slug)
	if err != nil {
		return nil, nil, err
	}

	// (2) the LAST non-correction worklog row of every story terminal.
	rows, err := tx.QueryContext(ctx, `
		SELECT w.story FROM worklog w
		WHERE w.epic = ?1 AND w.corrects IS NULL AND w.story GLOB 'S[0-9]*'
		  AND w.seq = (SELECT MAX(w2.seq) FROM worklog w2
		               WHERE w2.epic = w.epic AND w2.story = w.story AND w2.corrects IS NULL)
		  AND w.state NOT IN ('DONE','DISSOLVED')`, slug)
	if err != nil {
		return nil, nil, fmt.Errorf("close conditions: %w", err)
	}
	blockers, err = drainStrings(rows, blockers, "(2) story %s's last episode is not terminal")
	if err != nil {
		return nil, nil, err
	}

	// (3) criteria: runnables re-executed now, non-runnables met=1.
	attested, failedRuns, output, err := runCriteria(ctx, tx, slug)
	checkBlockers := make([]string, 0, len(attested)+len(failedRuns))
	checkBlockers = append(checkBlockers, attested...)
	checkBlockers = append(checkBlockers, failedRuns...)
	if err != nil {
		return nil, nil, err
	}
	blockers = append(blockers, checkBlockers...)

	// (4) no open task homed to the epic.
	trows, err := tx.QueryContext(ctx, `
		SELECT '#' || id || ' (' || status || ')' FROM tasks
		WHERE epic = ?1 AND status IN ('OPEN','IN-REVIEW','NEEDS-TRIAGE')`, slug)
	if err != nil {
		return nil, nil, fmt.Errorf("close conditions: %w", err)
	}
	blockers, err = drainStrings(trows, blockers, "(4) task %s is homed here")
	if err != nil {
		return nil, nil, err
	}

	// (5) every DONE story has a DONE worklog row with non-empty commits
	// (`legacy:` passes, visibly — it IS non-empty).
	drows, err := tx.QueryContext(ctx, `
		SELECT s.id FROM stories s WHERE s.epic = ?1 AND s.status = 'DONE'
		AND NOT EXISTS (SELECT 1 FROM worklog w
			WHERE w.epic = s.epic AND w.story = s.id AND w.state = 'DONE'
			  AND w.commits <> '' AND w.corrects IS NULL)`, slug)
	if err != nil {
		return nil, nil, fmt.Errorf("close conditions: %w", err)
	}
	blockers, err = drainStrings(drows, blockers, "(5) DONE story %s has no DONE worklog row with commits")
	if err != nil {
		return nil, nil, err
	}
	return blockers, output, nil
}

// storyConditions covers (1) every story terminal and (6) the ratified
// cardinality floor.
func storyConditions(ctx context.Context, tx *sql.Tx, slug string) ([]string, error) {
	var blockers []string
	var stories, terminal int64
	err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*), COUNT(CASE WHEN status IN ('DONE','DISSOLVED') THEN 1 END)
		FROM stories WHERE epic = ?`, slug).Scan(&stories, &terminal)
	if err != nil {
		return nil, fmt.Errorf("close conditions: %w", err)
	}
	const minStories = 2
	if stories < minStories {
		blockers = append(blockers, fmt.Sprintf(
			"(6) the epic has %d stor(y/ies); a goal that never decomposed past one closes as a task, not an epic",
			stories))
	}
	if terminal < stories {
		nrows, err := tx.QueryContext(ctx,
			`SELECT id FROM stories WHERE epic = ? AND status NOT IN ('DONE','DISSOLVED') ORDER BY id`, slug)
		if err != nil {
			return nil, fmt.Errorf("close conditions: %w", err)
		}
		blockers, err = drainStrings(nrows, blockers, "(1) story %s is not terminal")
		if err != nil {
			return nil, err
		}
	}
	return blockers, nil
}

// runCriteria is condition (3)'s engine, shared with the standalone
// `criteria check`: every `$ `-prefixed criterion executes (repo root,
// inherited env, per-command timeout, stop at first failure), pass/fail
// plus timestamp lands as evidence, and a failing re-run flips met 1→0
// so regressions cannot stay green. The two blocker classes return
// SEPARATELY because the two callers weigh them differently: the epic
// close blocks on both, the standalone check exits 1 only for a failed
// RUNNABLE criterion — an unmet owner-attested criterion is close
// condition (3)'s business, not a check failure (§6.2; found by the S10
// dogfood as task #7, where the shared list made the standalone verb
// report "a runnable criterion failed" with every runnable green).
func runCriteria(ctx context.Context, tx *sql.Tx, slug string) ([]string, []string, []string, error) {
	var attested, failedRuns, output []string
	crits, err := loadCriteria(ctx, tx, slug)
	if err != nil {
		return nil, nil, nil, err
	}
	stopped := false
	for _, c := range crits {
		cmd, isRunnable := runnableCommand(c.criterion)
		if !isRunnable {
			if c.met != 1 {
				attested = append(attested,
					fmt.Sprintf("(3) criterion %d is owner-attested and not met", c.seq))
			}
			continue
		}
		if stopped {
			continue // stop at first failure (§6.2): later runnables are not executed
		}
		line, blocker, err := runOne(ctx, tx, slug, c.seq, cmd)
		if err != nil {
			return nil, nil, nil, err
		}
		output = append(output, line)
		if blocker != "" {
			failedRuns = append(failedRuns, blocker)
			stopped = true
		}
	}
	return attested, failedRuns, output, nil
}

type crit struct {
	seq       int64
	criterion string
	met       int64
}

func loadCriteria(ctx context.Context, tx *sql.Tx, slug string) ([]crit, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT seq, criterion, met FROM epic_criteria WHERE epic = ? ORDER BY seq`, slug)
	if err != nil {
		return nil, fmt.Errorf("criteria: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var crits []crit
	for rows.Next() {
		var c crit
		if err := rows.Scan(&c.seq, &c.criterion, &c.met); err != nil {
			return nil, fmt.Errorf("criteria: %w", err)
		}
		crits = append(crits, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("criteria: %w", err)
	}
	return crits, nil
}

// runOne executes a single runnable criterion, records its evidence, and
// applies the regression flip (met 1→0 on failure).
func runOne(ctx context.Context, tx *sql.Tx, slug string, seq int64, cmd string) (string, string, error) {
	pass, detail := executeCriterion(ctx, cmd)
	evidence := fmt.Sprintf("%s %s @ %s", map[bool]string{true: "PASS", false: "FAIL"}[pass], detail, now())
	line := fmt.Sprintf("criterion %d: %s", seq, evidence)
	met := int64(0)
	if pass {
		met = 1
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE epic_criteria SET met = ?, evidence = ? WHERE epic = ? AND seq = ?`,
		met, evidence, slug, seq); err != nil {
		return "", "", err //nolint:wrapcheck // constraint codes ride to the mapper
	}
	blocker := ""
	if !pass {
		blocker = fmt.Sprintf("(3) criterion %d failed: %s", seq, cmd)
	}
	return line, blocker, nil
}

func runnableCommand(criterion string) (string, bool) {
	const prefix = "$ "
	if len(criterion) > len(prefix) && criterion[:len(prefix)] == prefix {
		return criterion[len(prefix):], true
	}
	return "", false
}

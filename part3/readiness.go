package main

import (
	"context"
	"database/sql"
	"fmt"
	"math"
)

type attemptResult struct {
	isCorrect bool
	trapType  *string
}

func UpdateReadinessScore(ctx context.Context, userID string, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Ensure a row exists before we lock it — a new student won't have one yet.
	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_readiness (user_id, score, grade, calculated_at)
		VALUES ($1, 0, 'D', NOW())
		ON CONFLICT (user_id) DO NOTHING
	`, userID)
	if err != nil {
		return fmt.Errorf("ensure readiness row: %w", err)
	}

	var lockCheck int
	if err := tx.QueryRowContext(ctx,
		`SELECT 1 FROM user_readiness WHERE user_id = $1 FOR UPDATE`,
		userID,
	).Scan(&lockCheck); err != nil {
		return fmt.Errorf("acquire row lock: %w", err)
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT is_correct, trap_type
		FROM attempts
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 15
	`, userID)
	if err != nil {
		return fmt.Errorf("query attempts: %w", err)
	}
	defer rows.Close()

	var attempts []attemptResult
	for rows.Next() {
		var a attemptResult
		if err := rows.Scan(&a.isCorrect, &a.trapType); err != nil {
			return fmt.Errorf("scan attempt row: %w", err)
		}
		attempts = append(attempts, a)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate attempts: %w", err)
	}

	if len(attempts) == 0 {
		return tx.Commit()
	}

	score, grade := computeScore(attempts)

	_, err = tx.ExecContext(ctx, `
		UPDATE user_readiness
		SET score         = $1,
		    grade         = $2,
		    calculated_at = NOW()
		WHERE user_id = $3
	`, score, grade, userID)
	if err != nil {
		return fmt.Errorf("update readiness: %w", err)
	}

	return tx.Commit()
}

func computeScore(attempts []attemptResult) (score float64, grade string) {
	var points float64
	for _, a := range attempts {
		if a.isCorrect {
			points++
		} else if a.trapType != nil {
			points -= 0.5
		}
	}

	score = math.Max(0, (points/15)*100)
	grade = scoreToGrade(score)
	return
}

func scoreToGrade(score float64) string {
	switch {
	case score >= 85:
		return "A+"
	case score >= 70:
		return "A"
	case score >= 55:
		return "B"
	case score >= 40:
		return "C"
	default:
		return "D"
	}
}

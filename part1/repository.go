package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type SessionInfo struct {
	UserID    string
	PaperType string
	Status    string
}

func GetSessionByID(ctx context.Context, tx *sql.Tx, sessionID string) (*SessionInfo, error) {
	var s SessionInfo
	err := tx.QueryRowContext(ctx, `
		SELECT user_id, paper_type, status
		FROM sessions
		WHERE id = $1
		FOR UPDATE
	`, sessionID).Scan(&s.UserID, &s.PaperType, &s.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	return &s, nil
}

type QuestionInfo struct {
	ID            string
	SubtopicID    string
	CorrectAnswer string
	TrapType      *string
	Marks         int
	Difficulty    string
}

func FetchQuestions(ctx context.Context, tx *sql.Tx, questionIDs []string) (map[string]QuestionInfo, error) {
	if len(questionIDs) == 0 {
		return map[string]QuestionInfo{}, nil
	}

	placeholders := make([]string, len(questionIDs))
	args := make([]interface{}, len(questionIDs))
	for i, id := range questionIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT id, subtopic_id, correct_answer, trap_type, marks, difficulty
		FROM questions
		WHERE id IN (%s)
	`, strings.Join(placeholders, ", "))

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("fetch questions: %w", err)
	}
	defer rows.Close()

	result := make(map[string]QuestionInfo, len(questionIDs))
	for rows.Next() {
		var q QuestionInfo
		if err := rows.Scan(&q.ID, &q.SubtopicID, &q.CorrectAnswer, &q.TrapType, &q.Marks, &q.Difficulty); err != nil {
			return nil, fmt.Errorf("scan question: %w", err)
		}
		result[q.ID] = q
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate questions: %w", err)
	}

	return result, nil
}

type AttemptRow struct {
	UserID           string
	QuestionID       string
	SessionID        string
	ChoiceID         *string
	IsCorrect        bool
	CorrectAnswer    string
	StudentAnswer    string
	Marks            int
	TimeStartedAt    time.Time
	TimeEndedAt      time.Time
	TimeSpentSeconds int
}

func BatchInsertAttempts(ctx context.Context, tx *sql.Tx, attempts []AttemptRow) error {
	if len(attempts) == 0 {
		return nil
	}

	const colsPerRow = 11
	placeholders := make([]string, len(attempts))
	args := make([]interface{}, 0, len(attempts)*colsPerRow)

	for i, a := range attempts {
		base := i * colsPerRow
		placeholders[i] = fmt.Sprintf(
			"($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			base+1, base+2, base+3, base+4, base+5,
			base+6, base+7, base+8, base+9, base+10, base+11,
		)
		args = append(args,
			a.UserID,
			a.QuestionID,
			a.SessionID,
			a.ChoiceID,
			a.IsCorrect,
			a.CorrectAnswer,
			a.StudentAnswer,
			a.Marks,
			a.TimeStartedAt,
			a.TimeEndedAt,
			a.TimeSpentSeconds,
		)
	}

	query := fmt.Sprintf(`
		INSERT INTO attempts (
			user_id, question_id, session_id, choice_id,
			is_correct, correct_answer, student_answer, marks,
			time_started_at, time_ended_at, time_spent_in_seconds
		)
		VALUES %s
	`, strings.Join(placeholders, ", "))

	_, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("batch insert attempts: %w", err)
	}

	return nil
}

func CompleteSessionRecord(ctx context.Context, tx *sql.Tx, sessionID string) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE sessions
		SET status = 'completed', ended_at = NOW()
		WHERE id = $1 AND status = 'in_progress'
	`, sessionID)
	if err != nil {
		return fmt.Errorf("complete session: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check session update: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("session %s not found or already completed", sessionID)
	}

	return nil
}

func CalcWeaknessesForSubtopics(ctx context.Context, tx *sql.Tx, userID string, subtopicIDs []string) ([]WeaknessSummary, error) {
	if len(subtopicIDs) == 0 {
		return nil, nil
	}

	seen := make(map[string]bool, len(subtopicIDs))
	unique := make([]string, 0, len(subtopicIDs))
	for _, id := range subtopicIDs {
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}

	var weaknesses []WeaknessSummary

	for _, subtopicID := range unique {
		var name string
		var total int
		var errorRate, trapErrorRate float64
		err := tx.QueryRowContext(ctx, `
			WITH recent AS (
				SELECT a.is_correct, q.trap_type, q.subtopic_id
				FROM attempts a
				JOIN questions q ON q.id = a.question_id
				WHERE a.user_id = $1
				  AND q.subtopic_id = $2
				ORDER BY a.created_at DESC
				LIMIT 20
			)
			SELECT
				s.name,
				COUNT(*),
				COALESCE(100.0 * COUNT(*) FILTER (WHERE NOT is_correct) / NULLIF(COUNT(*), 0), 0),
				COALESCE(100.0 * COUNT(*) FILTER (WHERE NOT is_correct AND trap_type IS NOT NULL) / NULLIF(COUNT(*), 0), 0)
			FROM recent
			JOIN subtopics s ON s.id = recent.subtopic_id
			GROUP BY s.name
		`, userID, subtopicID).Scan(&name, &total, &errorRate, &trapErrorRate)

		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("calc weakness for subtopic %s: %w", subtopicID, err)
		}

		if total == 0 {
			continue
		}

		weaknesses = append(weaknesses, WeaknessSummary{
			SubtopicID:    subtopicID,
			SubtopicName:  name,
			ErrorRate:     errorRate,
			TrapErrorRate: trapErrorRate,
		})
	}

	return weaknesses, nil
}

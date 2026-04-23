package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

func CompleteSession(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB

		var req CompleteSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}

		if err := req.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		for i, a := range req.Answers {
			if err := a.Validate(); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("answers[%d]: %s", i, err.Error()))
				return
			}
		}

		userID := r.Header.Get("X-User-ID")
		if userID == "" {
			writeError(w, http.StatusUnauthorized, "missing user identity")
			return
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			writeInternalError(w, "begin tx", err)
			return
		}
		defer tx.Rollback()

		session, err := GetSessionByID(ctx, tx, req.SessionID)
		if err != nil {
			writeInternalError(w, "fetch session", err)
			return
		}
		if session == nil {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		if session.UserID != userID {
			writeError(w, http.StatusForbidden, "session does not belong to this user")
			return
		}
		if session.Status != "in_progress" {
			writeError(w, http.StatusConflict, "session is already completed")
			return
		}

		questionIDs := make([]string, len(req.Answers))
		for i, a := range req.Answers {
			questionIDs[i] = a.QuestionID
		}

		questions, err := FetchQuestions(ctx, tx, questionIDs)
		if err != nil {
			writeInternalError(w, "fetch questions", err)
			return
		}

		for i, a := range req.Answers {
			if _, ok := questions[a.QuestionID]; !ok {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("answers[%d]: question_id %s not found", i, a.QuestionID))
				return
			}
		}

		attemptRows := make([]AttemptRow, len(req.Answers))
		subtopicIDs := make([]string, len(req.Answers))

		for i, a := range req.Answers {
			q := questions[a.QuestionID]

			isCorrect := strings.EqualFold(
				strings.TrimSpace(a.StudentAnswer),
				strings.TrimSpace(q.CorrectAnswer),
			)

			timeSpent := int(a.TimeEndedAt.Sub(a.TimeStartedAt).Seconds())

			attemptRows[i] = AttemptRow{
				UserID:           userID,
				QuestionID:       a.QuestionID,
				SessionID:        req.SessionID,
				ChoiceID:         a.ChoiceID,
				IsCorrect:        isCorrect,
				CorrectAnswer:    q.CorrectAnswer,
				StudentAnswer:    a.StudentAnswer,
				Marks:            q.Marks,
				TimeStartedAt:    a.TimeStartedAt,
				TimeEndedAt:      a.TimeEndedAt,
				TimeSpentSeconds: timeSpent,
			}

			subtopicIDs[i] = q.SubtopicID
		}

		if err := BatchInsertAttempts(ctx, tx, attemptRows); err != nil {
			writeInternalError(w, "insert attempts", err)
			return
		}

		if err := CompleteSessionRecord(ctx, tx, req.SessionID); err != nil {
			writeInternalError(w, "complete session", err)
			return
		}

		weaknesses, err := CalcWeaknessesForSubtopics(ctx, tx, userID, subtopicIDs)
		if err != nil {
			writeInternalError(w, "update weaknesses", err)
			return
		}

		if err := tx.Commit(); err != nil {
			writeInternalError(w, "commit tx", err)
			return
		}

		writeJSON(w, http.StatusOK, CompleteSessionResponse{
			SessionID:  req.SessionID,
			Weaknesses: weaknesses,
		})
	}
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("error writing response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

func writeInternalError(w http.ResponseWriter, context string, err error) {
	log.Printf("internal error [%s]: %v", context, err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}

package main

import (
	"time"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"
)

type CompleteSessionRequest struct {
	SessionID string          `json:"session_id"`
	Answers   []AnswerPayload `json:"answers"`
}

type AnswerPayload struct {
	QuestionID    string    `json:"question_id"`
	ChoiceID      *string   `json:"choice_id"`
	StudentAnswer string    `json:"student_answer"`
	TimeStartedAt time.Time `json:"time_started_at"`
	TimeEndedAt   time.Time `json:"time_ended_at"`
}

func (r CompleteSessionRequest) Validate() error {
	return validation.ValidateStruct(&r,
		validation.Field(&r.SessionID, validation.Required, is.UUIDv4),
		validation.Field(&r.Answers,
			validation.Required,
		),
	)
}

func (a AnswerPayload) Validate() error {
	return validation.ValidateStruct(&a,
		validation.Field(&a.QuestionID, validation.Required, is.UUIDv4),
		validation.Field(&a.ChoiceID, validation.NilOrNotEmpty, is.UUIDv4),
		validation.Field(&a.StudentAnswer, validation.Required, validation.Length(1, 2000)),
		validation.Field(&a.TimeStartedAt, validation.Required),
		validation.Field(&a.TimeEndedAt, validation.Required, validation.Min(a.TimeStartedAt).
			Error("time_ended_at must be after time_started_at")),
	)
}

type CompleteSessionResponse struct {
	SessionID  string            `json:"session_id"`
	Weaknesses []WeaknessSummary `json:"weaknesses"`
}

type WeaknessSummary struct {
	SubtopicID    string  `json:"subtopic_id"`
	SubtopicName  string  `json:"subtopic_name"`
	ErrorRate     float64 `json:"error_rate"`
	TrapErrorRate float64 `json:"trap_error_rate"`
}

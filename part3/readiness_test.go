package main

import (
	"math"
	"testing"
)

func TestScoreToGrade(t *testing.T) {
	cases := []struct {
		score float64
		grade string
	}{
		{100, "A+"},
		{85, "A+"},
		{84.9, "A"},
		{70, "A"},
		{69.9, "B"},
		{55, "B"},
		{54.9, "C"},
		{40, "C"},
		{39.9, "D"},
		{0, "D"},
	}
	for _, tc := range cases {
		got := scoreToGrade(tc.score)
		if got != tc.grade {
			t.Errorf("scoreToGrade(%.1f) = %q, want %q", tc.score, got, tc.grade)
		}
	}
}

func TestComputeScore_AllCorrect15Attempts(t *testing.T) {
	attempts := make([]attemptResult, 15)
	for i := range attempts {
		attempts[i] = attemptResult{isCorrect: true}
	}
	score, grade := computeScore(attempts)
	if score != 100 {
		t.Errorf("15/15 correct: got score %.1f, want 100", score)
	}
	if grade != "A+" {
		t.Errorf("15/15 correct: got grade %q, want A+", grade)
	}
}

func TestComputeScore_TrapAnswersDragsScore(t *testing.T) {
	trap := "Sign Error"
	// 13 correct, 2 trap errors → points = 13 - 1 = 12, score = (12/15)*100 = 80 → A
	attempts := []attemptResult{
		{isCorrect: false, trapType: &trap},
		{isCorrect: false, trapType: &trap},
	}
	for i := 2; i < 15; i++ {
		attempts = append(attempts, attemptResult{isCorrect: true})
	}

	score, grade := computeScore(attempts)
	expected := math.Max(0, (12.0/15.0)*100)
	if math.Abs(score-expected) > 0.01 {
		t.Errorf("got %.2f, want %.2f", score, expected)
	}
	if grade != "A" {
		t.Errorf("got grade %q, want A", grade)
	}
}

func TestComputeScore_WrongWithNoTrapIsZeroPoints(t *testing.T) {
	attempts := []attemptResult{
		{isCorrect: true},
		{isCorrect: false, trapType: nil}, // 0 points, no trap
	}
	for i := 2; i < 15; i++ {
		attempts = append(attempts, attemptResult{isCorrect: true})
	}

	score, grade := computeScore(attempts)
	expected := math.Max(0, (14.0/15.0)*100)
	if math.Abs(score-expected) > 0.01 {
		t.Errorf("got %.2f, want %.2f", score, expected)
	}
	if grade != "A+" {
		t.Errorf("got grade %q, want A+", grade)
	}
}

func TestComputeScore_ScoreFloorIsZero(t *testing.T) {
	trap := "Concept Confusion"
	// 15 trap errors → points = -7.5, MAX(0, -7.5/15*100) = 0
	attempts := make([]attemptResult, 15)
	for i := range attempts {
		attempts[i] = attemptResult{isCorrect: false, trapType: &trap}
	}
	score, grade := computeScore(attempts)
	if score != 0 {
		t.Errorf("all trap errors: got score %.1f, want 0", score)
	}
	if grade != "D" {
		t.Errorf("zero score: got grade %q, want D", grade)
	}
}

func TestComputeScore_NewStudentScoreDeflation(t *testing.T) {
	attempts := []attemptResult{
		{isCorrect: true},
		{isCorrect: true},
		{isCorrect: true},
	}
	score, grade := computeScore(attempts)
	// 3 correct out of a denominator of 15 → (3/15)*100 = 20
	if score != 20 {
		t.Errorf("got %.1f, want 20 — documenting score deflation for new students", score)
	}
	if grade != "D" {
		t.Errorf("got grade %q, want D — documenting deflation for new students", grade)
	}
}

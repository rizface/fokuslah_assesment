package main

import (
	"context"
	"errors"
	"testing"
)

const (
	testQuestionID    = "q-abc-123"
	testQuestion      = "Solve for x: 2x + 4 = 10"
	testCorrectAnswer = "x = 3"
	testSubtopic      = "Linear Equations"
)

func TestGenerateExplanation_LLMCalledOnCacheMiss(t *testing.T) {
	llm := &MockLLMClient{Response: "Step 1: subtract 4 from both sides..."}
	svc := NewExplanationService(llm, NewInMemoryCache())

	result, err := svc.GenerateExplanation(context.Background(), testQuestionID, testQuestion, testCorrectAnswer, testSubtopic)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != llm.Response {
		t.Fatalf("got %q, want %q", result, llm.Response)
	}
	if llm.CallCount != 1 {
		t.Fatalf("expected 1 LLM call, got %d", llm.CallCount)
	}
}

func TestGenerateExplanation_SecondCallHitsCache(t *testing.T) {
	llm := &MockLLMClient{Response: "Step 1: subtract 4 from both sides..."}
	svc := NewExplanationService(llm, NewInMemoryCache())

	ctx := context.Background()

	first, _ := svc.GenerateExplanation(ctx, testQuestionID, testQuestion, testCorrectAnswer, testSubtopic)
	second, err := svc.GenerateExplanation(ctx, testQuestionID, testQuestion, testCorrectAnswer, testSubtopic)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first != second {
		t.Fatalf("cache returned different value: first=%q second=%q", first, second)
	}
	if llm.CallCount != 1 {
		t.Fatalf("LLM should be called once; got %d", llm.CallCount)
	}
}

func TestGenerateExplanation_LLMDown_ReturnsFallback(t *testing.T) {
	llm := &MockLLMClient{Err: errors.New("provider unavailable")}
	svc := NewExplanationService(llm, NewInMemoryCache())

	result, err := svc.GenerateExplanation(context.Background(), testQuestionID, testQuestion, testCorrectAnswer, testSubtopic)

	if err != nil {
		t.Fatalf("error must not propagate to caller when LLM is down: %v", err)
	}
	if result != fallbackMessage {
		t.Fatalf("expected fallback message, got: %q", result)
	}

	if llm.CallCount != maxRetries+1 {
		t.Fatalf("expected %d LLM attempts, got %d", maxRetries+1, llm.CallCount)
	}
}

func TestGenerateExplanation_EmptyChoices_ReturnsFallback(t *testing.T) {
	llm := &MockLLMClient{EmptyChoices: true}
	svc := NewExplanationService(llm, NewInMemoryCache())

	result, err := svc.GenerateExplanation(context.Background(), testQuestionID, testQuestion, testCorrectAnswer, testSubtopic)

	if err != nil {
		t.Fatalf("error must not propagate to caller: %v", err)
	}
	if result != fallbackMessage {
		t.Fatalf("expected fallback message, got: %q", result)
	}
}

func TestGenerateExplanation_LLMDown_DoesNotCacheFallback(t *testing.T) {
	failLLM := &MockLLMClient{Err: errors.New("provider unavailable")}
	cache := NewInMemoryCache()
	svc := NewExplanationService(failLLM, cache)

	ctx := context.Background()

	svc.GenerateExplanation(ctx, testQuestionID, testQuestion, testCorrectAnswer, testSubtopic) //nolint

	healthyLLM := &MockLLMClient{Response: "real explanation"}
	svc.llm = healthyLLM

	result, err := svc.GenerateExplanation(ctx, testQuestionID, testQuestion, testCorrectAnswer, testSubtopic)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "real explanation" {
		t.Fatalf("expected real explanation after recovery, got: %q", result)
	}
	if healthyLLM.CallCount != 1 {
		t.Fatalf("expected 1 LLM call after recovery, got %d", healthyLLM.CallCount)
	}
}

func TestGenerateExplanation_CancelledContext_ReturnsFallback(t *testing.T) {
	llm := &MockLLMClient{Err: errors.New("provider slow")}
	svc := NewExplanationService(llm, NewInMemoryCache())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := svc.GenerateExplanation(ctx, testQuestionID, testQuestion, testCorrectAnswer, testSubtopic)

	if err != nil {
		t.Fatalf("error must not propagate to caller: %v", err)
	}
	if result != fallbackMessage {
		t.Fatalf("expected fallback on cancelled context, got: %q", result)
	}
}

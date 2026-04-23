package main

import (
	"context"
	"fmt"
	"log"
	"time"
)

const (
	explanationTTL = 30 * 24 * time.Hour
	llmTimeout     = 15 * time.Second
	maxRetries     = 2
	retryDelay     = 500 * time.Millisecond
	llmModel       = "gpt-4o-mini"
	maxTokens      = 400
)

const fallbackMessage = "We couldn't load an explanation right now. " +
	"Review the correct answer above and try working through it step by step — you'll get it!"

type ExplanationService struct {
	llm   LLMClient
	cache ExplanationCache
}

func NewExplanationService(llm LLMClient, cache ExplanationCache) *ExplanationService {
	return &ExplanationService{llm: llm, cache: cache}
}

func (s *ExplanationService) GenerateExplanation(
	ctx context.Context,
	questionID, question, correctAnswer, subtopic string,
) (string, error) {
	cacheKey := fmt.Sprintf("explanation:v1:%s", questionID)

	if cached, ok := s.cache.Get(ctx, cacheKey); ok {
		return cached, nil
	}

	llmCtx, cancel := context.WithTimeout(ctx, llmTimeout)
	defer cancel()

	var (
		explanation string
		lastErr     error
	)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return fallbackMessage, nil
			case <-time.After(retryDelay):
			}
		}

		explanation, lastErr = s.callLLM(llmCtx, question, correctAnswer, subtopic)
		if lastErr == nil {
			break
		}

		log.Printf("explanation: LLM attempt %d/%d failed: %v", attempt+1, maxRetries+1, lastErr)
	}

	if lastErr != nil {
		return fallbackMessage, nil
	}

	s.cache.Set(ctx, cacheKey, explanation, explanationTTL)

	return explanation, nil
}

func (s *ExplanationService) callLLM(
	ctx context.Context,
	question, correctAnswer, subtopic string,
) (string, error) {
	systemPrompt := `You are a patient and encouraging Maths tutor helping Malaysian secondary school students prepare for the SPM exam.

Rules:
- Write in simple, clear English for a 16-year-old student
- Explain HOW to solve the problem correctly, step by step
- Name the key concept or formula being tested in this question
- Be warm and encouraging — the student just got this wrong and needs confidence, not judgment
- Stay under 200 words
- End with one short sentence that states the single most important thing to remember
- Do not use university-level notation or advanced jargon; if you use a formula, write it plainly`

	userPrompt := fmt.Sprintf(
		"Subtopic: %s\n\nQuestion: %s\n\nCorrect Answer: %s\n\nExplain how to solve this correctly, step by step.",
		subtopic, question, correctAnswer,
	)

	resp, err := s.llm.CreateChatCompletion(ctx, ChatCompletionRequest{
		Model: llmModel,
		Messages: []ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		MaxTokens:   maxTokens,
		Temperature: 0.3, // lower temperature = more consistent, less creative variance
	})
	if err != nil {
		return "", fmt.Errorf("LLM call: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("LLM returned empty choices")
	}

	return resp.Choices[0].Message.Content, nil
}

package main

import (
	"context"
	"time"
)

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
}

type Choice struct {
	Message ChatMessage `json:"message"`
}

type ChatCompletionResponse struct {
	Choices []Choice `json:"choices"`
}

// LLMClient abstracts the LLM provider so it can be swapped or mocked in tests.
type LLMClient interface {
	CreateChatCompletion(ctx context.Context, req ChatCompletionRequest) (ChatCompletionResponse, error)
}

// ExplanationCache abstracts the cache layer (Redis in production, in-memory in tests).
type ExplanationCache interface {
	Get(ctx context.Context, key string) (string, bool)
	Set(ctx context.Context, key string, value string, ttl time.Duration)
}

package main

import (
	"context"
	"sync"
	"time"
)

// MockLLMClient lets tests control what the LLM returns without real network calls.
type MockLLMClient struct {
	mu           sync.Mutex
	Response     string
	Err          error
	EmptyChoices bool
	CallCount    int
}

func (m *MockLLMClient) CreateChatCompletion(_ context.Context, _ ChatCompletionRequest) (ChatCompletionResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallCount++

	if m.Err != nil {
		return ChatCompletionResponse{}, m.Err
	}
	if m.EmptyChoices {
		return ChatCompletionResponse{}, nil
	}
	return ChatCompletionResponse{
		Choices: []Choice{{Message: ChatMessage{Content: m.Response}}},
	}, nil
}

// InMemoryCache is a simple TTL cache used in tests.
// In production this would be backed by Redis.
type InMemoryCache struct {
	mu    sync.RWMutex
	store map[string]cacheEntry
}

type cacheEntry struct {
	value     string
	expiresAt time.Time
}

func NewInMemoryCache() *InMemoryCache {
	return &InMemoryCache{store: make(map[string]cacheEntry)}
}

func (c *InMemoryCache) Get(_ context.Context, key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	e, ok := c.store[key]
	if !ok || time.Now().After(e.expiresAt) {
		return "", false
	}
	return e.value, true
}

func (c *InMemoryCache) Set(_ context.Context, key, value string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.store[key] = cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}
}

package register

import (
	"fmt"
	"sync"
	"time"
)

// RateLimitInfo tracks rate limiting state for a specific endpoint+proxy combo.
type RateLimitInfo struct {
	LastHit    time.Time
	RetryAfter time.Duration
	Hits       int
}

// RateLimiter tracks and manages rate limiting across proxies and endpoints.
type RateLimiter struct {
	mu       sync.Mutex
	limits   map[string]*RateLimitInfo // key = "proxy|endpoint"
	backoffs []time.Duration           // progressive backoff durations
}

// NewRateLimiter creates a new rate limiter with default backoff schedule.
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		limits: make(map[string]*RateLimitInfo),
		backoffs: []time.Duration{
			30 * time.Second,
			1 * time.Minute,
			5 * time.Minute,
			15 * time.Minute,
			30 * time.Minute,
		},
	}
}

func (rl *RateLimiter) key(proxy, endpoint string) string {
	return fmt.Sprintf("%s|%s", proxy, endpoint)
}

// RecordHit records a rate limit hit for a proxy+endpoint combination.
func (rl *RateLimiter) RecordHit(proxy, endpoint string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	key := rl.key(proxy, endpoint)
	existing, ok := rl.limits[key]
	if !ok {
		rl.limits[key] = &RateLimitInfo{
			LastHit:    time.Now(),
			RetryAfter: rl.backoffs[0],
			Hits:       1,
		}
		return
	}

	existing.Hits++
	existing.LastHit = time.Now()

	// Progressive backoff: next level in backoffs array
	level := existing.Hits - 1
	if level >= len(rl.backoffs) {
		level = len(rl.backoffs) - 1
	}
	existing.RetryAfter = rl.backoffs[level]
}

// ShouldWait checks if we should wait before making a request.
// Returns 0 if no wait needed, or the duration to wait.
func (rl *RateLimiter) ShouldWait(proxy, endpoint string) time.Duration {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	key := rl.key(proxy, endpoint)
	existing, ok := rl.limits[key]
	if !ok {
		return 0
	}

	elapsed := time.Since(existing.LastHit)
	if elapsed < existing.RetryAfter {
		return existing.RetryAfter - elapsed
	}

	// Cooldown period has passed, reset
	delete(rl.limits, key)
	return 0
}

// Wait blocks until the cooldown period expires for a proxy+endpoint.
func (rl *RateLimiter) Wait(proxy, endpoint string) {
	wait := rl.ShouldWait(proxy, endpoint)
	if wait > 0 {
		fmt.Printf("⏳ Rate limited [%s], waiting %s...\n", endpoint, wait.Round(time.Second))
		time.Sleep(wait)
	}
}

// IsRateLimited checks if a status code indicates rate limiting.
func IsRateLimited(statusCode int) bool {
	return statusCode == 429 || statusCode == 409
}

// GetBackoffLevel returns the current backoff level for a proxy+endpoint.
func (rl *RateLimiter) GetBackoffLevel(proxy, endpoint string) int {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	key := rl.key(proxy, endpoint)
	existing, ok := rl.limits[key]
	if !ok {
		return 0
	}
	return existing.Hits
}

// Reset clears all rate limit tracking.
func (rl *RateLimiter) Reset() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.limits = make(map[string]*RateLimitInfo)
}

// Stats returns a formatted string of rate limit status.
func (rl *RateLimiter) Stats() string {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if len(rl.limits) == 0 {
		return "No rate limits active"
	}

	result := ""
	for key, info := range rl.limits {
		remaining := info.RetryAfter - time.Since(info.LastHit)
		if remaining < 0 {
			remaining = 0
		}
		result += fmt.Sprintf("  ⛔ %s | hits:%d cooldown:%s\n", key, info.Hits, remaining.Round(time.Second))
	}
	return result
}

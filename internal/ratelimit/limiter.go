// Package ratelimit provides a simple per-key token-bucket rate limiter, used to
// slow abusive bursts against the coordinator (e.g. mass join/register from a
// leaked invite). It bounds the *rate* of an action per key; combined with an
// invite's total-uses cap it raises the cost of Sybil registration.
package ratelimit

import (
	"sync"
	"time"
)

type bucket struct {
	tokens float64
	last   time.Time
}

// Limiter is a concurrency-safe per-key token-bucket rate limiter.
type Limiter struct {
	rate  float64 // tokens added per second
	burst float64 // maximum tokens (and initial fill)
	now   func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
}

// New returns a limiter allowing burst requests immediately per key and rate
// requests per second sustained.
func New(ratePerSec, burst float64) *Limiter {
	return &Limiter{
		rate:    ratePerSec,
		burst:   burst,
		now:     time.Now,
		buckets: make(map[string]*bucket),
	}
}

// Allow reports whether a request for key may proceed, consuming one token if so.
func (l *Limiter) Allow(key string) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
		l.pruneLocked(now)
	}
	// Refill based on elapsed time, capped at burst.
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens = min(l.burst, b.tokens+elapsed*l.rate)
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens -= 1
		return true
	}
	return false
}

// pruneLocked drops buckets that have fully refilled and been idle, so the map
// does not grow without bound. Cheap and only runs when a new key appears.
func (l *Limiter) pruneLocked(now time.Time) {
	if len(l.buckets) < 1024 {
		return
	}
	for k, b := range l.buckets {
		if now.Sub(b.last) > time.Minute && b.tokens >= l.burst {
			delete(l.buckets, k)
		}
	}
}

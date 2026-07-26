// Package bandwidth provides a per-key byte-rate limiter with a circuit-breaker
// query, used by an exit to cap how much traffic a single requester may push
// through the operator's (residential) link. Unlike the connection-count and
// connection-rate limits, this bounds *volume*: a requester that opens one
// allowed connection but streams gigabytes is caught here.
//
// It is a token bucket measured in bytes: tokens refill at bytesPerSec up to
// burst, and each Charge subtracts the bytes relayed. When the bucket goes
// negative the key is "over budget" (breaker tripped); it recovers on its own as
// tokens refill, so the cutoff is proportional to the overage.
package bandwidth

import (
	"sync"
	"time"
)

type bucket struct {
	tokens float64 // may go negative when over budget
	last   time.Time
}

// Limiter is a concurrency-safe per-key byte-rate limiter.
type Limiter struct {
	rate  float64 // bytes per second sustained
	burst float64 // maximum tokens (and initial fill)
	now   func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
}

// New returns a limiter allowing burst bytes immediately per key and rate bytes
// per second sustained.
func New(bytesPerSec, burst float64) *Limiter {
	return &Limiter{
		rate:    bytesPerSec,
		burst:   burst,
		now:     time.Now,
		buckets: make(map[string]*bucket),
	}
}

// refillLocked advances b's token count for elapsed time, capped at burst.
func (l *Limiter) refillLocked(b *bucket, now time.Time) {
	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens = min(l.burst, b.tokens+elapsed*l.rate)
		b.last = now
	}
}

// Charge records n bytes relayed for key and reports whether key is now over
// budget (breaker tripped).
func (l *Limiter) Charge(key string, n int64) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
		l.pruneLocked(now)
	}
	l.refillLocked(b, now)
	b.tokens -= float64(n)
	return b.tokens < 0
}

// Over reports whether key is currently over budget, accounting for refill since
// its last charge (so a recovered key reads false without needing a charge).
func (l *Limiter) Over(key string) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok {
		return false
	}
	l.refillLocked(b, now)
	return b.tokens < 0
}

// pruneLocked drops buckets that have fully refilled and been idle, bounding the
// map. Cheap and only runs when a new key first appears.
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

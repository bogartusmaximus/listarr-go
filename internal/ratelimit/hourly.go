package ratelimit

import (
	"sync"
	"time"
)

// HourlyBudget limits how many events may occur in a rolling one-hour window.
// Used for TorBox-oriented search-on-add pacing (default 60/hour).
type HourlyBudget struct {
	mu       sync.Mutex
	limit    int
	events   []time.Time
	now      func() time.Time
	maxTrack int
}

// NewHourlyBudget creates a budget. limit must be >= 1.
func NewHourlyBudget(limit int) *HourlyBudget {
	return NewHourlyBudgetWithClock(limit, time.Now)
}

// NewHourlyBudgetWithClock is for tests that need a synthetic clock.
func NewHourlyBudgetWithClock(limit int, now func() time.Time) *HourlyBudget {
	if limit < 1 {
		limit = 1
	}
	if now == nil {
		now = time.Now
	}
	return &HourlyBudget{
		limit:    limit,
		events:   make([]time.Time, 0, limit),
		now:      now,
		maxTrack: limit * 2,
	}
}

// Allow records one event if under budget. ok is false when deferred.
func (b *HourlyBudget) Allow() (remaining int, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pruneLocked(b.now())
	if len(b.events) >= b.limit {
		return 0, false
	}
	b.events = append(b.events, b.now())
	if len(b.events) > b.maxTrack {
		b.events = b.events[len(b.events)-b.limit:]
	}
	return b.limit - len(b.events), true
}

// Remaining returns how many events can still be accepted in the current window.
func (b *HourlyBudget) Remaining() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pruneLocked(b.now())
	left := b.limit - len(b.events)
	if left < 0 {
		return 0
	}
	return left
}

// Limit returns the configured hourly cap.
func (b *HourlyBudget) Limit() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.limit
}

// SetLimit updates the hourly cap (hot-reload). limit must be >= 1.
func (b *HourlyBudget) SetLimit(limit int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if limit < 1 {
		limit = 1
	}
	b.limit = limit
	b.maxTrack = limit * 2
}

func (b *HourlyBudget) pruneLocked(now time.Time) {
	cutoff := now.Add(-time.Hour)
	i := 0
	for i < len(b.events) && b.events[i].Before(cutoff) {
		i++
	}
	if i > 0 {
		b.events = append([]time.Time(nil), b.events[i:]...)
	}
}

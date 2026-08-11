package ratelimit_test

import (
	"testing"
	"time"

	"github.com/bogartusmaximus/listarr-go/internal/ratelimit"
)

func TestHourlyBudgetAllowsUpToLimit(t *testing.T) {
	b := ratelimit.NewHourlyBudget(3)
	for i := 0; i < 3; i++ {
		if _, ok := b.Allow(); !ok {
			t.Fatalf("allow %d failed", i)
		}
	}
	if _, ok := b.Allow(); ok {
		t.Fatal("expected denial past limit")
	}
	if b.Remaining() != 0 {
		t.Fatalf("remaining=%d", b.Remaining())
	}
}

func TestHourlyBudgetReleasesAfterWindow(t *testing.T) {
	start := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	now := start
	b := ratelimit.NewHourlyBudgetWithClock(1, func() time.Time { return now })

	if _, ok := b.Allow(); !ok {
		t.Fatal("first allow")
	}
	if _, ok := b.Allow(); ok {
		t.Fatal("second allow should fail within hour")
	}

	now = start.Add(time.Hour + time.Second)
	if _, ok := b.Allow(); !ok {
		t.Fatal("allow after window should succeed")
	}
	if b.Limit() != 1 {
		t.Fatalf("limit=%d", b.Limit())
	}
}

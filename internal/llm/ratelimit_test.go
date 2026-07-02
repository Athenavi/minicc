package llm

import (
	"context"
	"testing"
	"time"
)

func TestNewRateLimiter_Defaults(t *testing.T) {
	rl := NewRateLimiter(nil, 0, 0, 0)
	if rl.userLimit != 100 {
		t.Fatalf("expected default user limit 100, got %d", rl.userLimit)
	}
	if rl.modelLimit != 1000 {
		t.Fatalf("expected default model limit 1000, got %d", rl.modelLimit)
	}
	if rl.globalLimit != 10000 {
		t.Fatalf("expected default global limit 10000, got %d", rl.globalLimit)
	}
}

func TestNewRateLimiter_Custom(t *testing.T) {
	rl := NewRateLimiter(nil, 50, 200, 5000)
	if rl.userLimit != 50 {
		t.Fatalf("expected user limit 50, got %d", rl.userLimit)
	}
	if rl.modelLimit != 200 {
		t.Fatalf("expected model limit 200, got %d", rl.modelLimit)
	}
	if rl.globalLimit != 5000 {
		t.Fatalf("expected global limit 5000, got %d", rl.globalLimit)
	}
}

func TestRateLimiter_Allow_Local(t *testing.T) {
	rl := NewRateLimiter(nil, 100, 1000, 10000)
	// First request should always be allowed
	if !rl.Allow(context.Background(), "user1", "gpt-4") {
		t.Fatal("expected first request to be allowed")
	}
}

func TestRateLimiter_Allow_Multiple(t *testing.T) {
	rl := NewRateLimiter(nil, 10, 1000, 10000)
	// Allow 10 requests
	for i := 0; i < 10; i++ {
		if !rl.Allow(context.Background(), "user-fast", "model-x") {
			t.Fatalf("expected request %d to be allowed", i)
		}
	}
	// 11th should be denied (user limit exceeded)
	if rl.Allow(context.Background(), "user-fast", "model-x") {
		t.Fatal("expected 11th request to be denied")
	}
}

func TestRateLimiter_Allow_DifferentUsers(t *testing.T) {
	rl := NewRateLimiter(nil, 5, 1000, 10000)
	// Fill user1's bucket
	for i := 0; i < 5; i++ {
		rl.Allow(context.Background(), "user1", "model-x")
	}
	// user2 should still be allowed (separate counter)
	if !rl.Allow(context.Background(), "user2", "model-x") {
		t.Fatal("expected user2 to be allowed")
	}
	// user1 should be denied
	if rl.Allow(context.Background(), "user1", "model-x") {
		t.Fatal("expected user1 to be denied")
	}
}

func TestRateLimiter_LocalOnly(t *testing.T) {
	rl := NewRateLimiter(nil, 100, 1000, 10000)
	if !rl.localOnly {
		t.Fatal("expected localOnly to be true when rdb is nil")
	}
}

func TestRateLimiter_Cleanup(t *testing.T) {
	rl := NewRateLimiter(nil, 100, 1000, 10000)
	rl.Allow(context.Background(), "user-cleanup", "model-y")
	if len(rl.localCounts) == 0 {
		t.Fatal("expected local counts after allow")
	}
	// Cleanup should not panic
	rl.Cleanup(50 * time.Millisecond)
	time.Sleep(100 * time.Millisecond)
}

func TestNewTokenBucket(t *testing.T) {
	tb := NewTokenBucket(60, 10)
	if tb.rate != 60 {
		t.Fatalf("expected rate 60, got %d", tb.rate)
	}
	if tb.capacity != 10 {
		t.Fatalf("expected capacity 10, got %d", tb.capacity)
	}
}

func TestTokenBucket_Allow(t *testing.T) {
	tb := NewTokenBucket(60, 5)
	// First 5 should be allowed
	for i := 0; i < 5; i++ {
		if !tb.Allow("visitor1") {
			t.Fatalf("expected request %d to be allowed", i)
		}
	}
	// 6th should be denied (capacity exhausted)
	if tb.Allow("visitor1") {
		t.Fatal("expected 6th request to be denied")
	}
}

func TestTokenBucket_DifferentVisitors(t *testing.T) {
	tb := NewTokenBucket(60, 3)
	for i := 0; i < 3; i++ {
		tb.Allow("visitor-a")
	}
	// Different visitor should still be allowed
	if !tb.Allow("visitor-b") {
		t.Fatal("expected visitor-b to be allowed")
	}
	// Visitor-a should be denied
	if tb.Allow("visitor-a") {
		t.Fatal("expected visitor-a to be denied")
	}
}

func TestTokenBucket_CleanupVisitors(t *testing.T) {
	tb := NewTokenBucket(60, 5)
	tb.Allow("v1")
	tb.Allow("v2")
	if len(tb.visitors) != 2 {
		t.Fatalf("expected 2 visitors, got %d", len(tb.visitors))
	}
	tb.CleanupVisitors(50 * time.Millisecond)
	time.Sleep(100 * time.Millisecond)
}

func TestDefaultRateLimits(t *testing.T) {
	user, model, global := defaultRateLimits()
	if user != 100 || model != 1000 || global != 10000 {
		t.Fatalf("unexpected defaults: %d, %d, %d", user, model, global)
	}
}

func TestRateLimiter_Allow_EmptyIDs(t *testing.T) {
	rl := NewRateLimiter(nil, 100, 1000, 10000)
	if !rl.Allow(context.Background(), "", "") {
		t.Fatal("expected empty IDs to be allowed")
	}
}

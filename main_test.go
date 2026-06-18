package main

import (
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(10, 5)
	ip := "192.168.1.1"

	// Should allow 5 requests immediately
	for i := 0; i < 5; i++ {
		if !rl.Allow(ip) {
			t.Errorf("Expected request %d to be allowed", i)
		}
	}

	// 6th request should fail
	if rl.Allow(ip) {
		t.Error("Expected 6th request to be denied")
	}

	// Wait for token refill
	time.Sleep(200 * time.Millisecond)
	if !rl.Allow(ip) {
		t.Error("Expected request to be allowed after refill")
	}
}

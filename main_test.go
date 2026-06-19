package main

import (
"testing"
"time"
)

func TestRateLimiter(t *testing.T) {
rl := NewRateLimiter(10, 5)
ip := "192.168.1.1"

// Should allow 5 requests immediately (capacity = 5)
for i := 0; i < 5; i++ {
if !rl.Allow(ip) {
t.Errorf("Expected request %d to be allowed", i)
}
}

// 6th request should fail (bucket empty)
if rl.Allow(ip) {
t.Error("Expected 6th request to be denied")
}

// Wait 1.1 seconds — at rate=10/sec, 11 tokens are added
time.Sleep(1100 * time.Millisecond)

if !rl.Allow(ip) {
t.Error("Expected request to be allowed after refill")
}
}

func TestRateLimiterNewIP(t *testing.T) {
rl := NewRateLimiter(10, 3)
if !rl.Allow("10.0.0.1") {
t.Error("Expected first request from new IP to be allowed")
}
}

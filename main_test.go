package main

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(10, 5)
	defer rl.Close()
	for i := 0; i < 5; i++ {
		if !rl.Allow("192.168.1.1") {
			t.Errorf("request %d should be allowed", i)
		}
	}
	if rl.Allow("192.168.1.1") {
		t.Error("sixth request should be denied")
	}
	time.Sleep(150 * time.Millisecond)
	if !rl.Allow("192.168.1.1") {
		t.Error("request should be allowed after refill")
	}
}

func TestRateLimiterNewIP(t *testing.T) {
	rl := NewRateLimiter(10, 3)
	defer rl.Close()
	if !rl.Allow("10.0.0.1") {
		t.Error("first request should be allowed")
	}
}

func TestRateLimiterRejectsInvalidConfiguration(t *testing.T) {
	for _, test := range []struct{ rate, capacity int }{{0, 1}, {1, 0}, {-1, 1}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("configuration %+v should panic", test)
				}
			}()
			NewRateLimiter(test.rate, test.capacity)
		}()
	}
}

func TestRateLimiterObserver(t *testing.T) {
	rl := NewRateLimiter(1, 1)
	defer rl.Close()

	var allowed, denied atomic.Int64
	rl.SetObserver(func(_ string, ok bool, _ float64) {
		if ok {
			allowed.Add(1)
		} else {
			denied.Add(1)
		}
	})

	if !rl.Allow("203.0.113.10") {
		t.Fatal("first request should be allowed")
	}
	if rl.Allow("203.0.113.10") {
		t.Fatal("second request should be denied")
	}
	if got := allowed.Load(); got != 1 {
		t.Fatalf("allowed observer events = %d, want 1", got)
	}
	if got := denied.Load(); got != 1 {
		t.Fatalf("denied observer events = %d, want 1", got)
	}
}

func TestRateLimiterCloseIsIdempotent(t *testing.T) {
	rl := NewRateLimiter(1, 1)
	rl.Close()
	rl.Close()
}

func TestClientIP(t *testing.T) {
	if got := clientIP("192.0.2.10:1234"); got != "192.0.2.10" {
		t.Fatalf("got %q", got)
	}
	if got := clientIP("198.51.100.10"); got != "198.51.100.10" {
		t.Fatalf("got %q", got)
	}
}

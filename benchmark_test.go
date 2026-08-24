package main

import "testing"

func BenchmarkRateLimiterAllow(b *testing.B) {
	// Keep the benchmark focused on limiter admission overhead rather than on
	// intentionally exhausting the token bucket during fast CI runners.
	rl := NewRateLimiter(1_000_000_000, 1_000_000_000)
	defer rl.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !rl.Allow("198.51.100.42") {
			b.Fatal("unexpected denial")
		}
	}
}

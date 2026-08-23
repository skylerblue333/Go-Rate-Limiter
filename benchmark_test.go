package main

import "testing"

func BenchmarkRateLimiterAllow(b *testing.B) {
	// Keep benchmark capacity far above the timed iteration count so this
	// measures the hot allow path instead of eventually benchmarking denials.
	rl := NewRateLimiterWithLimit(1<<30, 1<<30, 16)
	defer rl.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !rl.Allow("198.51.100.42") {
			b.Fatal("unexpected denial")
		}
	}
}

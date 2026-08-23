package main

import "testing"

func BenchmarkRateLimiterAllow(b *testing.B) {
	rl := NewRateLimiter(1000000, 1000000)
	defer rl.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !rl.Allow("198.51.100.42") {
			b.Fatal("unexpected denial")
		}
	}
}

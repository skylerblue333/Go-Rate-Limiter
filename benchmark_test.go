package main

import "testing"

func BenchmarkRateLimiterAllow(b *testing.B) {
	// Benchmark the complete admission-decision path. Benchmarks can execute an
	// implementation-defined number of iterations, so correctness assertions
	// about never exhausting a bucket belong in unit tests, not timing loops.
	rl := NewRateLimiter(1_000_000, 1_000_000)
	defer rl.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rl.AllowDecision("198.51.100.42")
	}
}

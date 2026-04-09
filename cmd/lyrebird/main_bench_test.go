package main

import (
	"testing"
)

// BenchmarkRun measures command routing performance.
func BenchmarkRun(b *testing.B) {
	args := []string{"help"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = run(args)
	}
}

// BenchmarkRunVersion measures version command performance.
func BenchmarkRunVersion(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = runVersion()
	}
}

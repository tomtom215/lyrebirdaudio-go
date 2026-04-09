//go:build linux

package lock

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// BenchmarkFileLockAcquireRelease measures lock acquisition performance.
func BenchmarkFileLockAcquireRelease(b *testing.B) {
	lockPath := filepath.Join(b.TempDir(), "bench.lock")

	lock, err := NewFileLock(lockPath)
	if err != nil {
		b.Fatalf("NewFileLock() error = %v", err)
	}
	defer func() { _ = lock.Close() }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := lock.Acquire(5 * time.Second); err != nil {
			b.Fatalf("Acquire() error = %v", err)
		}
		if err := lock.Release(); err != nil {
			b.Fatalf("Release() error = %v", err)
		}
	}
}

// BenchmarkFileLockAcquireContextRelease measures context-aware lock acquisition performance.
func BenchmarkFileLockAcquireContextRelease(b *testing.B) {
	lockPath := filepath.Join(b.TempDir(), "bench.lock")

	lock, err := NewFileLock(lockPath)
	if err != nil {
		b.Fatalf("NewFileLock() error = %v", err)
	}
	defer func() { _ = lock.Close() }()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := lock.AcquireContext(ctx, 5*time.Second); err != nil {
			b.Fatalf("AcquireContext() error = %v", err)
		}
		if err := lock.Release(); err != nil {
			b.Fatalf("Release() error = %v", err)
		}
	}
}

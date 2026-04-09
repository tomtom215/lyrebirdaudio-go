//go:build linux

package lock

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestFileLockConcurrentGoroutines verifies thread safety with multiple goroutines.
func TestFileLockConcurrentGoroutines(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	const numGoroutines = 10
	const iterations = 5

	var (
		wg           sync.WaitGroup
		successCount int32 // Atomic counter
	)

	// Start multiple goroutines trying to acquire the same lock
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				lock, err := NewFileLock(lockPath)
				if err != nil {
					t.Logf("Goroutine %d: NewFileLock() error = %v", id, err)
					continue
				}

				// Try to acquire with short timeout
				if err := lock.Acquire(100 * time.Millisecond); err == nil {
					// Successfully acquired
					atomic.AddInt32(&successCount, 1)

					// Hold lock briefly
					time.Sleep(10 * time.Millisecond)

					// Release
					if err := lock.Release(); err != nil {
						t.Logf("Goroutine %d: Release() error = %v", id, err)
					}
				}

				_ = lock.Close()
			}
		}(i)
	}

	wg.Wait()

	// At least some goroutines should have succeeded
	if successCount == 0 {
		t.Error("No goroutines successfully acquired lock")
	}

	t.Logf("Successful lock acquisitions: %d / %d attempts", successCount, numGoroutines*iterations)
}

// TestFileLockRaceDetector runs with -race flag to detect race conditions.
func TestFileLockRaceDetector(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	lock1, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}
	defer func() { _ = lock1.Close() }()

	lock2, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}
	defer func() { _ = lock2.Close() }()

	var wg sync.WaitGroup

	// Goroutine 1: Acquire and release repeatedly
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			if err := lock1.Acquire(100 * time.Millisecond); err == nil {
				time.Sleep(5 * time.Millisecond)
				_ = lock1.Release()
			}
		}
	}()

	// Goroutine 2: Acquire and release repeatedly
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			if err := lock2.Acquire(100 * time.Millisecond); err == nil {
				time.Sleep(5 * time.Millisecond)
				_ = lock2.Release()
			}
		}
	}()

	wg.Wait()
}

// TestFileLockConcurrentAcquireReleaseSingleInstance exercises concurrent Acquire/Release
// on a single FileLock instance to verify that fl.file is protected by a mutex.
// This test is designed to trigger the race detector if mutex protection is missing.
func TestFileLockConcurrentAcquireReleaseSingleInstance(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	fl, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}
	defer func() { _ = fl.Close() }()

	const goroutines = 4
	const iterations = 20

	var wg sync.WaitGroup
	var successCount int32

	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				if err := fl.Acquire(200 * time.Millisecond); err == nil {
					atomic.AddInt32(&successCount, 1)
					time.Sleep(time.Millisecond)
					_ = fl.Release()
				}
			}
		}()
	}

	wg.Wait()

	// At least some acquisitions should succeed
	if successCount == 0 {
		t.Error("No goroutines successfully acquired the lock")
	}
	t.Logf("Successful acquisitions: %d / %d attempts", successCount, goroutines*iterations)
}

// TestFileLockConcurrentAcquireContextReleaseSingleInstance exercises concurrent
// AcquireContext/Release on a single FileLock instance to verify thread safety.
func TestFileLockConcurrentAcquireContextReleaseSingleInstance(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "test.lock")

	fl, err := NewFileLock(lockPath)
	if err != nil {
		t.Fatalf("NewFileLock() error = %v", err)
	}
	defer func() { _ = fl.Close() }()

	const goroutines = 4
	const iterations = 20

	ctx := context.Background()

	var wg sync.WaitGroup
	var successCount int32

	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				if err := fl.AcquireContext(ctx, 200*time.Millisecond); err == nil {
					atomic.AddInt32(&successCount, 1)
					time.Sleep(time.Millisecond)
					_ = fl.Release()
				}
			}
		}()
	}

	wg.Wait()

	if successCount == 0 {
		t.Error("No goroutines successfully acquired the lock")
	}
	t.Logf("Successful acquisitions: %d / %d attempts", successCount, goroutines*iterations)
}

// TestFileLockConcurrentCloseRelease exercises concurrent Close and Release
// on a single FileLock instance to verify mutex protection of fl.file reads.
func TestFileLockConcurrentCloseRelease(t *testing.T) {
	const iterations = 20

	for i := 0; i < iterations; i++ {
		lockPath := filepath.Join(t.TempDir(), fmt.Sprintf("test-%d.lock", i))

		fl, err := NewFileLock(lockPath)
		if err != nil {
			t.Fatalf("NewFileLock() error = %v", err)
		}

		if err := fl.Acquire(1 * time.Second); err != nil {
			t.Fatalf("Acquire() error = %v", err)
		}

		var wg sync.WaitGroup
		wg.Add(2)

		// One goroutine calls Release, the other calls Close
		go func() {
			defer wg.Done()
			_ = fl.Release()
		}()

		go func() {
			defer wg.Done()
			_ = fl.Close()
		}()

		wg.Wait()
	}
}

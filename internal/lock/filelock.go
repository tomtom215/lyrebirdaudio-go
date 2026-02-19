// SPDX-License-Identifier: MIT

//go:build linux

package lock

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// FileLock represents a file-based lock using flock(2).
//
// Provides exclusive locking with:
//   - Stale lock detection (dead process, old age)
//   - Timeout support
//   - PID tracking
//   - Thread-safe operations
//
// Reference: mediamtx-stream-manager.sh acquire_lock() lines 837-906
type FileLock struct {
	mu   sync.Mutex
	path string
	file *os.File
	pid  int
}

const (
	// DefaultStaleThreshold is the age threshold for considering a lock stale.
	// Matches bash: LOCK_STALE_THRESHOLD=300
	DefaultStaleThreshold = 300 * time.Second

	// DefaultAcquireTimeout is the default timeout for lock acquisition.
	// Matches bash: LOCK_ACQUISITION_TIMEOUT=30
	DefaultAcquireTimeout = 30 * time.Second
)

// NewFileLock creates a new file-based lock.
//
// The lock file is created if it doesn't exist. The parent directory
// is created if needed.
//
// Parameters:
//   - path: Absolute path to lock file (e.g., "/run/myapp.lock")
//
// Returns:
//   - FileLock instance
//   - Error if path is invalid or directory can't be created
func NewFileLock(path string) (*FileLock, error) {
	if path == "" {
		return nil, fmt.Errorf("lock path cannot be empty")
	}

	// Create parent directory if needed
	dir := filepath.Dir(path)
	// #nosec G301 - Lock directory needs 0755 for multi-user access
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create lock directory: %w", err)
	}

	return &FileLock{
		path: path,
		pid:  os.Getpid(),
	}, nil
}

// Acquire attempts to acquire the exclusive lock with a timeout.
//
// Acquisition process:
//  1. Check for stale lock (dead process, old age)
//  2. Remove stale lock if found
//  3. Open/create lock file
//  4. Call flock(2) with timeout
//  5. Write our PID to lock file
//
// Parameters:
//   - timeout: Maximum time to wait for lock (0 = try once, no wait)
//
// Returns:
//   - nil on success
//   - error on timeout or other failure
//
// Reference: mediamtx-stream-manager.sh acquire_lock() lines 837-906
func (fl *FileLock) Acquire(timeout time.Duration) error {
	// Check for stale lock and remove if found
	if stale, _ := isLockStale(fl.path, DefaultStaleThreshold); stale {
		_ = os.Remove(fl.path) // Explicitly ignore error - file might not exist
	}

	// Open lock file (create if doesn't exist)
	// #nosec G302 - Lock file needs 0644 for multi-process coordination
	file, err := os.OpenFile(fl.path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to open lock file: %w", err)
	}

	// Try to acquire lock with timeout
	deadline := time.Now().Add(timeout)
	for {
		// Try non-blocking flock
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			// Lock acquired!
			break
		}

		// Check if timeout expired
		if time.Now().After(deadline) {
			_ = file.Close()
			return fmt.Errorf("failed to acquire lock after %v: %w", timeout, err)
		}

		// Wait a bit before retrying
		time.Sleep(100 * time.Millisecond)
	}

	// Write our PID to lock file
	if err := file.Truncate(0); err != nil {
		_ = file.Close()
		return fmt.Errorf("failed to truncate lock file: %w", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		_ = file.Close()
		return fmt.Errorf("failed to seek lock file: %w", err)
	}
	if _, err := fmt.Fprintf(file, "%d\n", fl.pid); err != nil {
		_ = file.Close()
		return fmt.Errorf("failed to write PID to lock file: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("failed to sync lock file: %w", err)
	}

	fl.mu.Lock()
	fl.file = file
	fl.mu.Unlock()
	return nil
}

// AcquireContext attempts to acquire the exclusive lock with context cancellation support.
//
// Similar to Acquire() but respects context cancellation. This allows graceful shutdown
// when the calling goroutine needs to terminate.
//
// Acquisition process:
//  1. Check for stale lock (dead process, old age)
//  2. Remove stale lock if found
//  3. Open/create lock file
//  4. Call flock(2) with timeout, checking context.Done() in loop
//  5. Write our PID to lock file
//
// Parameters:
//   - ctx: Context for cancellation
//   - timeout: Maximum time to wait for lock (0 = try once, no wait)
//
// Returns:
//   - nil on success
//   - context.Canceled if context was cancelled
//   - context.DeadlineExceeded if timeout expired
//   - error on other failure
func (fl *FileLock) AcquireContext(ctx context.Context, timeout time.Duration) error {
	// Check if context is already cancelled
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Check for stale lock and remove if found
	if stale, _ := isLockStale(fl.path, DefaultStaleThreshold); stale {
		_ = os.Remove(fl.path) // Explicitly ignore error - file might not exist
	}

	// Open lock file (create if doesn't exist)
	// #nosec G302 - Lock file needs 0644 for multi-process coordination
	file, err := os.OpenFile(fl.path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to open lock file: %w", err)
	}

	// Try to acquire lock with timeout and context cancellation
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		// Try non-blocking flock
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			// Lock acquired!
			break
		}

		// Check if context was cancelled
		select {
		case <-ctx.Done():
			_ = file.Close()
			return ctx.Err()
		case <-ticker.C:
			// Check if timeout expired
			if time.Now().After(deadline) {
				_ = file.Close()
				return fmt.Errorf("failed to acquire lock after %v: %w", timeout, err)
			}
			// Continue loop to retry
		}
	}

	// Write our PID to lock file
	if err := file.Truncate(0); err != nil {
		_ = file.Close()
		return fmt.Errorf("failed to truncate lock file: %w", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		_ = file.Close()
		return fmt.Errorf("failed to seek lock file: %w", err)
	}
	if _, err := fmt.Fprintf(file, "%d\n", fl.pid); err != nil {
		_ = file.Close()
		return fmt.Errorf("failed to write PID to lock file: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("failed to sync lock file: %w", err)
	}

	fl.mu.Lock()
	fl.file = file
	fl.mu.Unlock()
	return nil
}

// Release releases the lock.
//
// Returns:
//   - nil on success
//   - error if lock not held or release fails
func (fl *FileLock) Release() error {
	fl.mu.Lock()
	defer fl.mu.Unlock()

	if fl.file == nil {
		return fmt.Errorf("lock not held")
	}

	// Release flock
	if err := syscall.Flock(int(fl.file.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("failed to unlock: %w", err)
	}

	// Close file
	if err := fl.file.Close(); err != nil {
		return fmt.Errorf("failed to close lock file: %w", err)
	}

	fl.file = nil
	return nil
}

// Close closes the lock file if held and releases the lock.
func (fl *FileLock) Close() error {
	fl.mu.Lock()
	held := fl.file != nil
	fl.mu.Unlock()

	if held {
		return fl.Release()
	}
	return nil
}

// isLockStale checks if a lock file is stale.
//
// A lock is considered stale if:
//  1. Lock file doesn't exist (not stale, just absent)
//  2. Lock file is empty or has invalid PID (stale)
//  3. PID process is not running (stale)
//  4. Lock file is older than threshold (stale)
//
// Parameters:
//   - lockPath: Path to lock file
//   - threshold: Age threshold for staleness
//
// Returns:
//   - true if lock is stale (should be removed)
//   - false if lock is valid or doesn't exist
//   - error if unable to determine (treat as not stale to be safe)
//
// Reference: mediamtx-stream-manager.sh is_lock_stale() lines 765-805
func isLockStale(lockPath string, threshold time.Duration) (bool, error) {
	// Check if lock file exists.
	// threshold is retained in the signature for API compatibility but is no
	// longer used after the C-1 fix: see comment below.
	_ = threshold

	_, err := os.Stat(lockPath)
	if os.IsNotExist(err) {
		return false, nil // No lock file = not stale
	}
	if err != nil {
		return false, err // Can't stat = assume not stale (safe default)
	}

	// Read PID from lock file
	// #nosec G304 - Lock path is controlled by application configuration
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return true, nil // Can't read = assume stale
	}

	pidStr := strings.TrimSpace(string(data))
	if pidStr == "" {
		return true, nil // Empty file = stale
	}

	// Parse PID
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return true, nil // Invalid PID = stale
	}

	// Check if process exists
	// Send signal 0 (no-op) to check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return true, nil // Process not found = stale
	}

	// On Unix, FindProcess always succeeds, so we need to send signal 0
	err = process.Signal(syscall.Signal(0))
	if err == nil {
		// Process is alive. The lock is valid regardless of the lock file's
		// modification time — a long-running stream (hours, days) always has
		// a lock file whose mtime is older than DefaultStaleThreshold.
		// Applying an age check here would steal the lock from a healthy
		// process, causing two managers to run concurrently on the same device.
		return false, nil
	}

	// Process is dead or unreachable; the lock is stale.
	// (Age is not checked here: if signal(0) failed the process is gone.)
	return true, nil
}

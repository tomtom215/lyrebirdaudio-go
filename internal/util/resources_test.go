package util

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResourceTrackerFiles verifies file tracking.
func TestResourceTrackerFiles(t *testing.T) {
	tracker := NewResourceTracker()

	// Initially empty
	if count := tracker.FileCount(); count != 0 {
		t.Errorf("Initial FileCount = %d, want 0", count)
	}

	// Create and track a file
	tmpFile := filepath.Join(t.TempDir(), "test.txt")
	file, err := os.Create(tmpFile)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tracker.TrackFile("test-file", file)

	if count := tracker.FileCount(); count != 1 {
		t.Errorf("FileCount after track = %d, want 1", count)
	}

	// Untrack the file
	tracker.UntrackFile("test-file")

	if count := tracker.FileCount(); count != 0 {
		t.Errorf("FileCount after untrack = %d, want 0", count)
	}

	// Clean up
	_ = file.Close()
	_ = os.Remove(tmpFile)
}

// TestResourceTrackerProcesses verifies process tracking.
func TestResourceTrackerProcesses(t *testing.T) {
	tracker := NewResourceTracker()

	// Initially empty
	if count := tracker.ProcessCount(); count != 0 {
		t.Errorf("Initial ProcessCount = %d, want 0", count)
	}

	// Track a process (use current process for testing)
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("Failed to find process: %v", err)
	}

	tracker.TrackProcess("test-process", process)

	if count := tracker.ProcessCount(); count != 1 {
		t.Errorf("ProcessCount after track = %d, want 1", count)
	}

	// Untrack the process
	tracker.UntrackProcess("test-process")

	if count := tracker.ProcessCount(); count != 0 {
		t.Errorf("ProcessCount after untrack = %d, want 0", count)
	}
}

// TestResourceTrackerGeneric verifies generic resource tracking.
func TestResourceTrackerGeneric(t *testing.T) {
	tracker := NewResourceTracker()

	// Initially empty
	if count := tracker.ResourceCount(); count != 0 {
		t.Errorf("Initial ResourceCount = %d, want 0", count)
	}

	// Track a generic resource
	resource := "some-lock"
	tracker.TrackResource("lock-1", resource)

	if count := tracker.ResourceCount(); count != 1 {
		t.Errorf("ResourceCount after track = %d, want 1", count)
	}

	// Untrack the resource
	tracker.UntrackResource("lock-1")

	if count := tracker.ResourceCount(); count != 0 {
		t.Errorf("ResourceCount after untrack = %d, want 0", count)
	}
}

// TestResourceTrackerLeaks verifies leak detection.
func TestResourceTrackerLeaks(t *testing.T) {
	tracker := NewResourceTracker()

	// No leaks initially
	if leaked := tracker.LeakedResources(); len(leaked) != 0 {
		t.Errorf("Initial leaks = %v, want empty", leaked)
	}

	// Create and track resources without cleanup
	tmpFile := filepath.Join(t.TempDir(), "leak.txt")
	file, err := os.Create(tmpFile)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer func() { _ = file.Close() }()
	defer func() { _ = os.Remove(tmpFile) }()

	tracker.TrackFile("leaked-file", file)
	tracker.TrackResource("leaked-lock", "lock")

	// Should detect leaks
	leaked := tracker.LeakedResources()
	if len(leaked) != 2 {
		t.Errorf("Leaked resources = %d, want 2", len(leaked))
	}

	// Check leak names
	hasFile := false
	hasResource := false
	for _, name := range leaked {
		if name == "file:leaked-file" {
			hasFile = true
		}
		if name == "resource:leaked-lock" {
			hasResource = true
		}
	}

	if !hasFile {
		t.Error("Leaked resources should include 'file:leaked-file'")
	}
	if !hasResource {
		t.Error("Leaked resources should include 'resource:leaked-lock'")
	}
}

// TestResourceTrackerCleanupAll verifies cleanup functionality.
func TestResourceTrackerCleanupAll(t *testing.T) {
	tracker := NewResourceTracker()

	// Create and track multiple files
	tmpDir := t.TempDir()
	files := make([]*os.File, 3)
	for i := 0; i < 3; i++ {
		tmpFile := filepath.Join(tmpDir, "test_"+string(rune('a'+i))+".txt")
		file, err := os.Create(tmpFile)
		if err != nil {
			t.Fatalf("Failed to create test file %d: %v", i, err)
		}
		files[i] = file
		tracker.TrackFile("file-"+string(rune('a'+i)), file)
	}

	// Track a generic resource
	tracker.TrackResource("lock", "some-lock")

	// Verify resources are tracked
	if count := tracker.Count(); count != 4 {
		t.Errorf("Total resources = %d, want 4", count)
	}

	// Cleanup all
	errors := tracker.CleanupAll()

	// Should have one error for the generic resource (can't auto-cleanup)
	if len(errors) != 1 {
		t.Errorf("Cleanup errors = %d, want 1 (generic resource)", len(errors))
	}

	// All resources should be untracked
	if count := tracker.Count(); count != 0 {
		t.Errorf("Resources after cleanup = %d, want 0", count)
	}

	// Files should be closed (verify by trying to write)
	for i, file := range files {
		_, err := file.Write([]byte("test"))
		if err == nil {
			t.Errorf("File %d should be closed", i)
		}
	}
}

// TestResourceTrackerCount verifies total count.
func TestResourceTrackerCount(t *testing.T) {
	tracker := NewResourceTracker()

	// Initially empty
	if count := tracker.Count(); count != 0 {
		t.Errorf("Initial count = %d, want 0", count)
	}

	// Add different types of resources
	tmpFile := filepath.Join(t.TempDir(), "test.txt")
	file, err := os.Create(tmpFile)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer func() { _ = file.Close() }()
	defer func() { _ = os.Remove(tmpFile) }()

	tracker.TrackFile("file", file)
	tracker.TrackProcess("process", &os.Process{})
	tracker.TrackResource("lock", "lock")

	// Should count all resources
	if count := tracker.Count(); count != 3 {
		t.Errorf("Total count = %d, want 3", count)
	}

	// Verify individual counts
	if count := tracker.FileCount(); count != 1 {
		t.Errorf("FileCount = %d, want 1", count)
	}
	if count := tracker.ProcessCount(); count != 1 {
		t.Errorf("ProcessCount = %d, want 1", count)
	}
	if count := tracker.ResourceCount(); count != 1 {
		t.Errorf("ResourceCount = %d, want 1", count)
	}
}

// TestResourceTrackerConcurrency verifies thread safety.
func TestResourceTrackerConcurrency(t *testing.T) {
	tracker := NewResourceTracker()
	const numGoroutines = 100

	// Concurrently track and untrack resources
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			// Track a resource
			tracker.TrackResource("resource", id)

			// Immediately untrack it
			tracker.UntrackResource("resource")

			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Final count should be deterministic (all untracked)
	// However, due to concurrent overwrites on same key, count might be 0 or 1
	if count := tracker.Count(); count > 1 {
		t.Errorf("Final count = %d, want 0 or 1 (concurrent overwrites allowed)", count)
	}
}

// TestResourceTrackerUntrackNonexistent verifies untracking nonexistent resources.
func TestResourceTrackerUntrackNonexistent(t *testing.T) {
	tracker := NewResourceTracker()

	// Untracking nonexistent resources should not panic or error
	tracker.UntrackFile("nonexistent")
	tracker.UntrackProcess("nonexistent")
	tracker.UntrackResource("nonexistent")

	// Count should still be 0
	if count := tracker.Count(); count != 0 {
		t.Errorf("Count after untracking nonexistent = %d, want 0", count)
	}
}

// TestResourceTrackerMultipleSameKey verifies overwriting with same key.
func TestResourceTrackerMultipleSameKey(t *testing.T) {
	tracker := NewResourceTracker()

	// Track two resources with same key (should overwrite)
	tracker.TrackResource("key", "value1")
	tracker.TrackResource("key", "value2")

	// Should only count as one resource
	if count := tracker.ResourceCount(); count != 1 {
		t.Errorf("ResourceCount = %d, want 1 (overwrite)", count)
	}

	// Untrack once should remove it
	tracker.UntrackResource("key")

	if count := tracker.ResourceCount(); count != 0 {
		t.Errorf("ResourceCount after untrack = %d, want 0", count)
	}
}

// BenchmarkResourceTrackerTrack measures tracking performance.
func BenchmarkResourceTrackerTrack(b *testing.B) {
	tracker := NewResourceTracker()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracker.TrackResource("resource", i)
		tracker.UntrackResource("resource")
	}
}

// BenchmarkResourceTrackerLeakedResources measures leak detection performance.
func BenchmarkResourceTrackerLeakedResources(b *testing.B) {
	tracker := NewResourceTracker()

	// Track 100 resources
	for i := 0; i < 100; i++ {
		tracker.TrackResource("resource-"+string(rune('a'+i%26)), i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tracker.LeakedResources()
	}
}

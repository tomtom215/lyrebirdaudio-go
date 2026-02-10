// SPDX-License-Identifier: MIT

package util

import (
	"fmt"
	"os"
	"sync"
)

// ResourceTracker tracks system resources for cleanup verification.
//
// This is CRITICAL for preventing resource leaks in 24/7 operation.
// The tracker maintains a registry of all open resources and can verify
// they are properly cleaned up during shutdown or testing.
//
// Tracked resources:
//   - File descriptors (os.File)
//   - Processes (os.Process)
//   - Named resources (locks, connections, etc.)
//
// Example:
//
//	tracker := NewResourceTracker()
//
//	file, _ := os.Open("test.txt")
//	tracker.TrackFile("config-file", file)
//	defer tracker.UntrackFile("config-file")
//
//	// Verify all resources cleaned up
//	if leaked := tracker.LeakedResources(); len(leaked) > 0 {
//	    log.Fatalf("Resource leak: %v", leaked)
//	}
type ResourceTracker struct {
	mu        sync.Mutex
	files     map[string]*os.File
	processes map[string]*os.Process
	resources map[string]interface{} // Named resources (locks, etc.)
}

// NewResourceTracker creates a new resource tracker.
func NewResourceTracker() *ResourceTracker {
	return &ResourceTracker{
		files:     make(map[string]*os.File),
		processes: make(map[string]*os.Process),
		resources: make(map[string]interface{}),
	}
}

// TrackFile registers a file for tracking.
func (rt *ResourceTracker) TrackFile(name string, file *os.File) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.files[name] = file
}

// UntrackFile unregisters a file.
func (rt *ResourceTracker) UntrackFile(name string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	delete(rt.files, name)
}

// TrackProcess registers a process for tracking.
func (rt *ResourceTracker) TrackProcess(name string, process *os.Process) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.processes[name] = process
}

// UntrackProcess unregisters a process.
func (rt *ResourceTracker) UntrackProcess(name string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	delete(rt.processes, name)
}

// TrackResource registers a named resource for tracking.
func (rt *ResourceTracker) TrackResource(name string, resource interface{}) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.resources[name] = resource
}

// UntrackResource unregisters a named resource.
func (rt *ResourceTracker) UntrackResource(name string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	delete(rt.resources, name)
}

// LeakedResources returns names of all resources still being tracked.
//
// In tests, this should return an empty slice if all resources were
// properly cleaned up.
func (rt *ResourceTracker) LeakedResources() []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	var leaked []string

	for name := range rt.files {
		leaked = append(leaked, fmt.Sprintf("file:%s", name))
	}

	for name := range rt.processes {
		leaked = append(leaked, fmt.Sprintf("process:%s", name))
	}

	for name := range rt.resources {
		leaked = append(leaked, fmt.Sprintf("resource:%s", name))
	}

	return leaked
}

// CleanupAll attempts to clean up all tracked resources.
//
// This is a best-effort cleanup for emergency situations.
// Returns any errors encountered during cleanup.
func (rt *ResourceTracker) CleanupAll() []error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	var errors []error

	// Close all files
	for name, file := range rt.files {
		if err := file.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close file %s: %w", name, err))
		}
		delete(rt.files, name)
	}

	// Kill all processes
	for name, process := range rt.processes {
		if err := process.Kill(); err != nil {
			errors = append(errors, fmt.Errorf("failed to kill process %s: %w", name, err))
		}
		delete(rt.processes, name)
	}

	// Clear resources (can't auto-cleanup generic resources)
	for name := range rt.resources {
		errors = append(errors, fmt.Errorf("resource %s not cleaned up", name))
		delete(rt.resources, name)
	}

	return errors
}

// Count returns the total number of tracked resources.
func (rt *ResourceTracker) Count() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return len(rt.files) + len(rt.processes) + len(rt.resources)
}

// FileCount returns the number of tracked files.
func (rt *ResourceTracker) FileCount() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return len(rt.files)
}

// ProcessCount returns the number of tracked processes.
func (rt *ResourceTracker) ProcessCount() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return len(rt.processes)
}

// ResourceCount returns the number of tracked named resources.
func (rt *ResourceTracker) ResourceCount() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return len(rt.resources)
}

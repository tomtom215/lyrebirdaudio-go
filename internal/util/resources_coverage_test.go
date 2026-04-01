// SPDX-License-Identifier: MIT

package util

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestLeakedResourcesIncludesProcess covers resources.go:106-108 — the
// process loop inside LeakedResources. By tracking a process without
// untracking it, the loop body executes and the process name appears in
// the returned slice.
func TestLeakedResourcesIncludesProcess(t *testing.T) {
	tracker := NewResourceTracker()

	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess: %v", err)
	}
	tracker.TrackProcess("self-proc", process)

	leaked := tracker.LeakedResources()
	found := false
	for _, l := range leaked {
		if l == "process:self-proc" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("LeakedResources() = %v, want to include process:self-proc", leaked)
	}
	tracker.UntrackProcess("self-proc")
}

// TestCleanupAllFileCloseError covers resources.go:129-131 — the
// file.Close() error branch in CleanupAll. A file is closed before being
// tracked; the second Close() call inside CleanupAll returns an error,
// which is collected and returned.
func TestCleanupAllFileCloseError(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "double-close.txt")
	file, err := os.Create(tmpFile)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Pre-close so the second Close() in CleanupAll fails.
	if err := file.Close(); err != nil {
		t.Fatalf("pre-close: %v", err)
	}

	tracker := NewResourceTracker()
	tracker.TrackFile("pre-closed", file)

	errors := tracker.CleanupAll()
	if len(errors) == 0 {
		t.Error("CleanupAll() expected error for pre-closed file, got none")
	}
	if tracker.FileCount() != 0 {
		t.Errorf("FileCount after CleanupAll = %d, want 0", tracker.FileCount())
	}
}

// TestCleanupAllProcessKillError covers resources.go:137-141 — the
// process.Kill() error branch in CleanupAll. An already-exited process is
// tracked; Go detects it is done and returns os.ErrProcessDone instead of
// sending any signal, producing a non-nil error.
func TestCleanupAllProcessKillError(t *testing.T) {
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("exec true: %v", err)
	}
	// cmd.Process is waited and reaped by Run(); Kill() returns ErrProcessDone.
	tracker := NewResourceTracker()
	tracker.TrackProcess("dead-proc", cmd.Process)

	errors := tracker.CleanupAll()
	if len(errors) == 0 {
		t.Error("CleanupAll() expected error for dead process, got none")
	}
	if tracker.ProcessCount() != 0 {
		t.Errorf("ProcessCount after CleanupAll = %d, want 0", tracker.ProcessCount())
	}
}

// TestCleanupAllKillsLiveProcess covers resources.go:136-141 — the
// successful process.Kill() path. A sleeping child process is tracked;
// CleanupAll sends SIGKILL and the Kill() call succeeds (no error).
func TestCleanupAllKillsLiveProcess(t *testing.T) {
	cmd := exec.Command("sleep", "100")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	tracker := NewResourceTracker()
	tracker.TrackProcess("live-proc", cmd.Process)

	errors := tracker.CleanupAll()
	// Kill on a live process should succeed — no error expected.
	if len(errors) != 0 {
		t.Errorf("CleanupAll() unexpected errors for live process: %v", errors)
	}
	if tracker.ProcessCount() != 0 {
		t.Errorf("ProcessCount after CleanupAll = %d, want 0", tracker.ProcessCount())
	}
	// Reap the process to avoid a zombie.
	_ = cmd.Wait()
}

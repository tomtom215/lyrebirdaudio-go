// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// TestCheckDiskSpaceInspectsLogDir verifies the disk check statfs's the
// filesystem backing Options.LogDir rather than always "/". /var is commonly a
// separate mount, so inspecting only "/" can report healthy while the daemon's
// log/recording filesystem is full.
func TestCheckDiskSpaceInspectsLogDir(t *testing.T) {
	tmp := t.TempDir()

	opts := DefaultOptions()
	opts.LogDir = tmp
	r := NewRunner(opts)

	result := r.checkDiskSpace(context.Background())

	if result.Status == StatusError {
		t.Fatalf("checkDiskSpace errored for LogDir=%s: %s (%s)", tmp, result.Message, result.Details)
	}
	if !strings.Contains(result.Message, "Disk usage") {
		t.Errorf("expected 'Disk usage' message, got %q", result.Message)
	}
	// The inspected filesystem path must reflect LogDir, proving "/" is no
	// longer hard-coded.
	if !strings.Contains(result.Details, tmp) {
		t.Errorf("expected disk check to inspect LogDir %q, Details=%q", tmp, result.Details)
	}
}

// TestCheckDiskSpaceLogDirNotYetCreated verifies that a LogDir which does not
// exist yet (created on first daemon run) does not error: statfsNearest walks
// up to the nearest existing ancestor.
func TestCheckDiskSpaceLogDirNotYetCreated(t *testing.T) {
	tmp := t.TempDir()
	opts := DefaultOptions()
	opts.LogDir = filepath.Join(tmp, "does", "not", "exist", "yet")
	r := NewRunner(opts)

	result := r.checkDiskSpace(context.Background())

	if result.Status == StatusError {
		t.Fatalf("expected non-error when LogDir absent (walks up), got: %s (%s)",
			result.Message, result.Details)
	}
	if !strings.Contains(result.Message, "Disk usage") {
		t.Errorf("expected 'Disk usage' message, got %q", result.Message)
	}
	// Inspected path must be an existing ancestor of the (missing) LogDir.
	inspected := strings.TrimPrefix(result.Details, "filesystem: ")
	if !strings.HasPrefix(filepath.Join(tmp, "does"), inspected) {
		t.Errorf("inspected path %q is not an ancestor of the configured LogDir", inspected)
	}
}

// TestDiskCheckPath verifies LogDir selection and the "/" fallback.
func TestDiskCheckPath(t *testing.T) {
	withLog := NewRunner(Options{LogDir: "/var/log/lyrebird"})
	if got := withLog.diskCheckPath(); got != "/var/log/lyrebird" {
		t.Errorf("expected LogDir path, got %q", got)
	}

	noLog := NewRunner(Options{LogDir: ""})
	if got := noLog.diskCheckPath(); got != "/" {
		t.Errorf("expected / fallback for empty LogDir, got %q", got)
	}
}

// TestStatfsNearestWalksUp verifies statfsNearest resolves a non-existent path
// to its nearest existing ancestor and fills a usable Statfs result.
func TestStatfsNearestWalksUp(t *testing.T) {
	tmp := t.TempDir()
	missing := filepath.Join(tmp, "a", "b", "c")

	var stat syscall.Statfs_t
	inspected, err := statfsNearest(missing, &stat)
	if err != nil {
		t.Fatalf("statfsNearest unexpected error: %v", err)
	}
	if !strings.HasPrefix(missing, inspected) {
		t.Errorf("inspected path %q is not an ancestor of %q", inspected, missing)
	}
	if stat.Blocks == 0 {
		t.Error("expected non-zero total blocks from statfs of an existing filesystem")
	}
}

// TestStatfsNearestEmptyPathUsesRoot verifies an empty path resolves to "/".
func TestStatfsNearestEmptyPathUsesRoot(t *testing.T) {
	var stat syscall.Statfs_t
	inspected, err := statfsNearest("", &stat)
	if err != nil {
		t.Fatalf("statfsNearest(\"\") error: %v", err)
	}
	if inspected != "/" {
		t.Errorf("expected empty path to resolve to /, got %q", inspected)
	}
}

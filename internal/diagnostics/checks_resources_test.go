// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"strings"
	"testing"
)

func TestCheckEntropySetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkEntropy(context.Background())

	if result.Name != "Entropy" {
		t.Errorf("expected Name 'Entropy', got %q", result.Name)
	}
	if result.Category != "System" {
		t.Errorf("expected Category 'System', got %q", result.Category)
	}
	if result.Duration <= 0 {
		t.Error("expected positive Duration")
	}
	// On Linux, /proc/sys/kernel/random/entropy_avail should exist
	if result.Status == StatusOK {
		if !strings.Contains(result.Message, "Entropy pool") {
			t.Errorf("expected message about entropy pool, got %q", result.Message)
		}
	}
}

func TestCheckInotifyLimitsSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkInotifyLimits(context.Background())

	if result.Name != "inotify Limits" {
		t.Errorf("expected Name 'inotify Limits', got %q", result.Name)
	}
	if result.Category != "Resources" {
		t.Errorf("expected Category 'Resources', got %q", result.Category)
	}
	// On Linux this should read successfully
	if result.Status == StatusOK {
		if !strings.Contains(result.Message, "inotify max_user_watches") {
			t.Errorf("expected inotify message, got %q", result.Message)
		}
	}
}

func TestCheckFileDescriptorsSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkFileDescriptors(context.Background())

	if result.Name != "File Descriptors" {
		t.Errorf("expected Name 'File Descriptors', got %q", result.Name)
	}
	if result.Category != "Resources" {
		t.Errorf("expected Category 'Resources', got %q", result.Category)
	}
	// On Linux /proc/sys/fs/file-nr should exist
	if result.Status == StatusOK {
		if !strings.Contains(result.Message, "FD usage") {
			t.Errorf("expected FD usage message, got %q", result.Message)
		}
	}
}

func TestCheckMemorySetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkMemory(context.Background())

	if result.Name != "Memory" {
		t.Errorf("expected Name 'Memory', got %q", result.Name)
	}
	if result.Category != "Resources" {
		t.Errorf("expected Category 'Resources', got %q", result.Category)
	}
	// On Linux /proc/meminfo should exist
	if result.Status == StatusError {
		t.Errorf("checkMemory should not error on Linux, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "Memory usage") {
		t.Errorf("expected Memory usage message, got %q", result.Message)
	}
}

func TestCheckDiskSpaceSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkDiskSpace(context.Background())

	if result.Name != "Disk Space" {
		t.Errorf("expected Name 'Disk Space', got %q", result.Name)
	}
	if result.Category != "Resources" {
		t.Errorf("expected Category 'Resources', got %q", result.Category)
	}
	// syscall.Statfs should work on Linux
	if result.Status == StatusError {
		t.Errorf("checkDiskSpace should not error on Linux, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "Disk usage") {
		t.Errorf("expected Disk usage message, got %q", result.Message)
	}
}

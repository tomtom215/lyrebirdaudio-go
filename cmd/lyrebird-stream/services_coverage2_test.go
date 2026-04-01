// SPDX-License-Identifier: MIT

package main

import (
	"testing"
)

// TestSystemInfoEmptyRecordDirFallback covers services.go:82-84 —
// the `dir = "/"` branch when recordDir is empty. When no local recording
// directory is configured the provider falls back to the root filesystem
// to report available disk space.
func TestSystemInfoEmptyRecordDirFallback(t *testing.T) {
	p := &daemonSystemInfoProvider{
		recordDir:        "", // triggers the `dir = "/"` fallback branch
		diskLowThreshold: 0, // disabled
	}

	si := p.SystemInfo()

	// Statfs on "/" must succeed; total bytes must be non-zero.
	if si.DiskTotalBytes == 0 {
		t.Error("DiskTotalBytes = 0 with empty recordDir; syscall.Statfs on '/' should succeed")
	}
}

// TestSystemInfoNTPBranchExclusive covers services.go:100-104 —
// asserts that exactly one of the two NTP branches (NTPSynced=true OR
// NTPMessage non-empty) executes per call. In CI timedatectl is typically
// unavailable, so the else branch fires and NTPMessage is set. On a
// synced system the if branch fires and NTPSynced is true. Either way,
// both are never true simultaneously.
func TestSystemInfoNTPBranchExclusive(t *testing.T) {
	p := &daemonSystemInfoProvider{
		recordDir:        "/",
		diskLowThreshold: 0,
	}

	si := p.SystemInfo()

	if si.NTPSynced && si.NTPMessage != "" {
		t.Error("NTPSynced and NTPMessage must not both be set simultaneously")
	}
	if !si.NTPSynced && si.NTPMessage == "" {
		t.Error("when NTPSynced is false, NTPMessage must be non-empty")
	}
}

// SPDX-License-Identifier: MIT

//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestDetectArchWithFakeUname verifies architecture detection with mocked uname.
func TestDetectArchWithFakeUname(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		wantArch string
	}{
		{
			name:     "x86_64",
			output:   "x86_64",
			wantArch: "amd64",
		},
		{
			name:     "amd64 direct",
			output:   "amd64",
			wantArch: "amd64",
		},
		{
			name:     "aarch64",
			output:   "aarch64",
			wantArch: "arm64",
		},
		{
			name:     "arm64 direct",
			output:   "arm64",
			wantArch: "arm64",
		},
		{
			name:     "armv7l",
			output:   "armv7l",
			wantArch: "armv7",
		},
		{
			name:     "armhf",
			output:   "armhf",
			wantArch: "armv7",
		},
		{
			name:     "armv6l",
			output:   "armv6l",
			wantArch: "armv6",
		},
		{
			name:     "unknown architecture",
			output:   "sparc64",
			wantArch: "",
		},
		{
			name:     "uname fails",
			output:   "",
			wantArch: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpBin := t.TempDir()
			fakeUname := filepath.Join(tmpBin, "uname")

			var script string
			if tt.output == "" && tt.name == "uname fails" {
				script = "#!/bin/sh\nexit 1\n"
			} else {
				script = fmt.Sprintf("#!/bin/sh\necho '%s'\n", tt.output)
			}

			if err := os.WriteFile(fakeUname, []byte(script), 0750); err != nil {
				t.Fatalf("failed to create fake uname: %v", err)
			}
			t.Setenv("PATH", tmpBin)

			got := detectArch()
			if got != tt.wantArch {
				t.Errorf("detectArch() = %q, want %q", got, tt.wantArch)
			}
		})
	}
}

// TestProcessExistsWithZeroPID verifies processExists with PID 0.
func TestProcessExistsWithZeroPID(t *testing.T) {
	// PID 0 refers to the kernel on Linux; signal(0) should fail for regular users.
	result := processExists(0)
	// Just verify it doesn't panic; actual result depends on permissions.
	_ = result
}

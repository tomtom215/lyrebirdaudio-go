// SPDX-License-Identifier: MIT

package main

import (
	"os/exec"
	"testing"
)

// TestRunCheckSystemSmoke verifies check-system runs without panic and returns
// an outcome consistent with the host environment.
//
// runCheckSystem returns a non-nil error if and only if a REQUIRED tool
// (ffmpeg) is absent. That outcome is environment-dependent: ffmpeg is present
// on CI but not on every host (e.g. minimal containers). Asserting err==nil
// unconditionally made this test fail on any host without ffmpeg even though
// the command behaved correctly. Instead, assert that the returned error
// correlates with ffmpeg's actual presence — this validates the contract in
// BOTH environments while remaining environment-independent.
func TestRunCheckSystemSmoke(t *testing.T) {
	err := runCheckSystem([]string{})

	_, ffmpegErr := exec.LookPath("ffmpeg")
	switch {
	case ffmpegErr == nil && err != nil:
		t.Errorf("runCheckSystem() returned error on a system with ffmpeg present: %v", err)
	case ffmpegErr != nil && err == nil:
		t.Error("runCheckSystem() returned nil but ffmpeg is missing; expected a non-nil error")
	}
}

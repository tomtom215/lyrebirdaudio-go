// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"testing"
)

// writeTestConfig writes content to path with 0640 permissions.
func writeTestConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0640); err != nil { //#nosec G304 -- test helper, path from t.TempDir()
		t.Fatalf("write config: %v", err)
	}
}

// minimalConfig returns a minimal YAML config string for testing.
func minimalConfig() string {
	return `default:
  sample_rate: 48000
  channels: 2
  bitrate: "128k"
  codec: opus
stream:
  initial_restart_delay: 1s
  max_restart_delay: 5m
`
}

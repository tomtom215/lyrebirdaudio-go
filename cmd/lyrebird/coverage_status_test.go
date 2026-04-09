// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunStatusWithLockDir verifies runStatus with various lock directory states.
func TestRunStatusWithLockDir(t *testing.T) {
	tests := []struct {
		name       string
		lockFiles  map[string]string // filename -> content
		jsonOutput bool
		wantErr    bool
	}{
		{
			name:       "empty lock dir text output",
			lockFiles:  nil,
			jsonOutput: false,
			wantErr:    false,
		},
		{
			name:       "empty lock dir json output",
			lockFiles:  nil,
			jsonOutput: true,
			wantErr:    false,
		},
		{
			name: "valid running pid text output",
			lockFiles: map[string]string{
				"test-device.lock": fmt.Sprintf("%d", os.Getpid()),
			},
			jsonOutput: false,
			wantErr:    false,
		},
		{
			name: "valid running pid json output",
			lockFiles: map[string]string{
				"test-device.lock": fmt.Sprintf("%d", os.Getpid()),
			},
			jsonOutput: true,
			wantErr:    false,
		},
		{
			name: "stale pid text output",
			lockFiles: map[string]string{
				"stale-device.lock": "99999999",
			},
			jsonOutput: false,
			wantErr:    false,
		},
		{
			name: "stale pid json output",
			lockFiles: map[string]string{
				"stale-device.lock": "99999999",
			},
			jsonOutput: true,
			wantErr:    false,
		},
		{
			name: "invalid lock file content",
			lockFiles: map[string]string{
				"bad-device.lock": "not-a-number",
			},
			jsonOutput: false,
			wantErr:    false,
		},
		{
			name: "invalid lock file content json",
			lockFiles: map[string]string{
				"bad-device.lock": "not-a-number",
			},
			jsonOutput: true,
			wantErr:    false,
		},
		{
			name: "empty lock file",
			lockFiles: map[string]string{
				"empty-device.lock": "",
			},
			jsonOutput: false,
			wantErr:    false,
		},
		{
			name: "multiple lock files mixed states",
			lockFiles: map[string]string{
				"running.lock": fmt.Sprintf("%d", os.Getpid()),
				"stale.lock":   "99999999",
				"invalid.lock": "garbage",
			},
			jsonOutput: false,
			wantErr:    false,
		},
		{
			name: "multiple lock files mixed states json",
			lockFiles: map[string]string{
				"running.lock": fmt.Sprintf("%d", os.Getpid()),
				"stale.lock":   "99999999",
				"invalid.lock": "garbage",
			},
			jsonOutput: true,
			wantErr:    false,
		},
		{
			name: "pid with whitespace",
			lockFiles: map[string]string{
				"whitespace.lock": fmt.Sprintf("  %d  \n", os.Getpid()),
			},
			jsonOutput: false,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lockDir := t.TempDir()

			// Create lock files
			for name, content := range tt.lockFiles {
				lockPath := filepath.Join(lockDir, name)
				if err := os.WriteFile(lockPath, []byte(content), 0640); err != nil {
					t.Fatalf("failed to create lock file %s: %v", name, err)
				}
			}

			args := []string{"--lock-dir=" + lockDir}
			if tt.jsonOutput {
				args = append(args, "--json")
			}

			err := runStatus(args)

			if tt.wantErr {
				if err == nil {
					t.Error("runStatus() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("runStatus() unexpected error: %v", err)
				}
			}
		})
	}
}

// TestRunStatusJSONFormat verifies the JSON output structure is valid.
func TestRunStatusJSONFormat(t *testing.T) {
	lockDir := t.TempDir()

	// Create a lock file with current PID (running)
	lockFile := filepath.Join(lockDir, "my-device.lock")
	if err := os.WriteFile(lockFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0640); err != nil {
		t.Fatalf("failed to create lock file: %v", err)
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	args := []string{"--lock-dir=" + lockDir, "--json"}
	err := runStatus(args)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runStatus() unexpected error: %v", err)
	}

	output, _ := io.ReadAll(r)

	// Parse JSON output
	var status StatusOutput
	if err := json.Unmarshal(output, &status); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, string(output))
	}

	// Verify structure
	if status.ActiveStreams == nil {
		t.Error("ActiveStreams should not be nil")
	}
	if status.AvailableURLs == nil {
		t.Error("AvailableURLs should not be nil")
	}

	// Should have at least one active stream from the lock file
	found := false
	for _, s := range status.ActiveStreams {
		if s.DeviceName == "my-device" {
			found = true
			if s.Status != "running" {
				t.Errorf("expected status 'running', got %q", s.Status)
			}
			if s.PID != os.Getpid() {
				t.Errorf("expected PID %d, got %d", os.Getpid(), s.PID)
			}
		}
	}
	if !found {
		t.Error("expected to find 'my-device' in active streams")
	}
}

// TestRunStatusConfigFlag verifies the --config flag is parsed.
func TestRunStatusConfigFlag(t *testing.T) {
	lockDir := t.TempDir()
	configPath := filepath.Join("..", "..", "testdata", "config", "valid.yaml")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	args := []string{
		"--lock-dir=" + lockDir,
		"--config=" + configPath,
	}

	err := runStatus(args)
	if err != nil {
		t.Errorf("runStatus() with config flag unexpected error: %v", err)
	}
}

// TestRunStatusJSONShortFlag verifies the -j short flag.
func TestRunStatusJSONShortFlag(t *testing.T) {
	lockDir := t.TempDir()

	args := []string{
		"--lock-dir=" + lockDir,
		"-j",
	}

	err := runStatus(args)
	if err != nil {
		t.Errorf("runStatus() with -j flag unexpected error: %v", err)
	}
}

// TestRunStatusStaleStreamTextOutput verifies text output for stale streams.
func TestRunStatusStaleStreamTextOutput(t *testing.T) {
	lockDir := t.TempDir()

	// Create a stale lock file (PID that doesn't exist)
	lockFile := filepath.Join(lockDir, "stale-stream.lock")
	if err := os.WriteFile(lockFile, []byte("99999999"), 0640); err != nil {
		t.Fatalf("failed to create lock file: %v", err)
	}

	args := []string{"--lock-dir=" + lockDir}
	err := runStatus(args)
	if err != nil {
		t.Errorf("runStatus() unexpected error: %v", err)
	}
}

// TestRunStatusZeroPID verifies handling of lock files with zero PID.
func TestRunStatusZeroPID(t *testing.T) {
	lockDir := t.TempDir()

	lockFile := filepath.Join(lockDir, "zero-pid.lock")
	if err := os.WriteFile(lockFile, []byte("0"), 0640); err != nil {
		t.Fatalf("failed to create lock file: %v", err)
	}

	args := []string{"--lock-dir=" + lockDir, "--json"}
	err := runStatus(args)
	if err != nil {
		t.Errorf("runStatus() unexpected error: %v", err)
	}
}

// TestStatusOutputJSONSerialization verifies StatusOutput serializes correctly.
func TestStatusOutputJSONSerialization(t *testing.T) {
	status := StatusOutput{
		ServiceStatus: "active (running)",
		DeviceCount:   2,
		ActiveStreams: []StreamStatus{
			{DeviceName: "mic1", Status: "running", PID: 12345},
			{DeviceName: "mic2", Status: "stale", PID: 99999},
		},
		AvailableURLs: []StreamURL{
			{DeviceName: "mic1", URL: "rtsp://localhost:8554/mic1"},
			{DeviceName: "mic2", URL: "rtsp://localhost:8554/mic2"},
		},
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("json.Marshal() unexpected error: %v", err)
	}

	var decoded StatusOutput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() unexpected error: %v", err)
	}

	if decoded.ServiceStatus != status.ServiceStatus {
		t.Errorf("ServiceStatus = %q, want %q", decoded.ServiceStatus, status.ServiceStatus)
	}
	if decoded.DeviceCount != status.DeviceCount {
		t.Errorf("DeviceCount = %d, want %d", decoded.DeviceCount, status.DeviceCount)
	}
	if len(decoded.ActiveStreams) != len(status.ActiveStreams) {
		t.Errorf("ActiveStreams length = %d, want %d", len(decoded.ActiveStreams), len(status.ActiveStreams))
	}
	if len(decoded.AvailableURLs) != len(status.AvailableURLs) {
		t.Errorf("AvailableURLs length = %d, want %d", len(decoded.AvailableURLs), len(status.AvailableURLs))
	}
}

// TestStatusOutputJSONOmitEmpty verifies error field is omitted when empty.
func TestStatusOutputJSONOmitEmpty(t *testing.T) {
	status := StatusOutput{
		ServiceStatus: "inactive",
		DeviceCount:   0,
		ActiveStreams: []StreamStatus{},
		AvailableURLs: []StreamURL{},
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("json.Marshal() unexpected error: %v", err)
	}

	if strings.Contains(string(data), "error") {
		t.Error("JSON output should omit 'error' field when empty")
	}
}

// TestStatusOutputJSONWithError verifies error field is present when set.
func TestStatusOutputJSONWithError(t *testing.T) {
	status := StatusOutput{
		ServiceStatus: "inactive",
		Error:         "some error occurred",
		ActiveStreams: []StreamStatus{},
		AvailableURLs: []StreamURL{},
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("json.Marshal() unexpected error: %v", err)
	}

	if !strings.Contains(string(data), "some error occurred") {
		t.Error("JSON output should include 'error' field when set")
	}
}

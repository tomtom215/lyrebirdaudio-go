// SPDX-License-Identifier: MIT

//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRunDetectWithQualityTier verifies detect command with quality flags.
func TestRunDetectWithQualityTier(t *testing.T) {
	asoundPath := filepath.Join("..", "..", "testdata", "proc", "asound")
	if _, err := os.Stat(asoundPath); os.IsNotExist(err) {
		t.Skip("Test data not available")
	}

	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "default quality",
			args:    []string{},
			wantErr: false,
		},
		{
			name:    "low quality with equals",
			args:    []string{"--quality=low"},
			wantErr: false,
		},
		{
			name:    "high quality with equals",
			args:    []string{"--quality=high"},
			wantErr: false,
		},
		{
			name:    "quality with space",
			args:    []string{"--quality", "normal"},
			wantErr: false,
		},
		{
			name:    "invalid quality tier",
			args:    []string{"--quality=ultra"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runDetectWithPath(asoundPath, tt.args)
			if tt.wantErr {
				if err == nil {
					t.Error("runDetectWithPath() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("runDetectWithPath() unexpected error: %v", err)
				}
			}
		})
	}
}

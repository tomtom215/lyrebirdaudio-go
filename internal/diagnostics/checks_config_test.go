// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckConfigExistsVsNotExists(t *testing.T) {
	tests := []struct {
		name           string
		setupConfig    bool
		expectedStatus CheckStatus
		expectedMsg    string
	}{
		{
			name:           "config file exists",
			setupConfig:    true,
			expectedStatus: StatusOK,
			expectedMsg:    "Configuration file exists",
		},
		{
			name:           "config file missing",
			setupConfig:    false,
			expectedStatus: StatusWarning,
			expectedMsg:    "Configuration file not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")

			if tt.setupConfig {
				if err := os.WriteFile(configPath, []byte("streams: []\n"), 0640); err != nil {
					t.Fatalf("failed to write config: %v", err)
				}
			}

			opts := DefaultOptions()
			opts.ConfigPath = configPath
			runner := NewRunner(opts)

			result := runner.checkConfig(context.Background())
			if result.Status != tt.expectedStatus {
				t.Errorf("expected status %s, got %s", tt.expectedStatus, result.Status)
			}
			if !strings.Contains(result.Message, tt.expectedMsg) {
				t.Errorf("expected message containing %q, got %q", tt.expectedMsg, result.Message)
			}
			if result.Details != configPath {
				t.Errorf("expected details to be config path %q, got %q", configPath, result.Details)
			}
		})
	}
}

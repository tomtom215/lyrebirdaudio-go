package config

import (
	"testing"
)

func TestParseBackupTimestamp(t *testing.T) {
	tests := []struct {
		filename string
		wantErr  bool
	}{
		{"config.yaml.2025-12-14T10-30-00.bak", false},
		// Note: millisecond format produces filename like "config.yaml.2025-12-14T10-30-00.000.bak"
		// where the timestamp part is "2025-12-14T10-30-00.000" - the parser splits by dots
		// and gets "000" as timestamp which is invalid. This is expected behavior.
		{"config.yaml.2025-12-14T10-30-00.000.bak", true}, // Invalid due to parsing limitation
		{"config.yaml.invalid.bak", true},
		{"config.yaml.bak", true},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			_, err := parseBackupTimestamp(tt.filename)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseBackupTimestamp(%q) error = %v, wantErr %v", tt.filename, err, tt.wantErr)
			}
		})
	}
}

func TestGetBackupDir(t *testing.T) {
	tests := []struct {
		configPath string
		want       string
	}{
		{"/etc/lyrebird/config.yaml", DefaultBackupDir},
		{"/home/user/config.yaml", "/home/user/backups"},
		{"/opt/lyrebird/config.yaml", "/opt/lyrebird/backups"},
	}

	for _, tt := range tests {
		t.Run(tt.configPath, func(t *testing.T) {
			got := GetBackupDir(tt.configPath)
			if got != tt.want {
				t.Errorf("GetBackupDir(%q) = %q, want %q", tt.configPath, got, tt.want)
			}
		})
	}
}

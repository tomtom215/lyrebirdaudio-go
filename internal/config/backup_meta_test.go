package config

import (
	"testing"
	"time"
)

func TestParseBackupTimestamp(t *testing.T) {
	tests := []struct {
		filename string
		wantErr  bool
		want     string // expected RFC3339 timestamp when wantErr is false
	}{
		{"config.yaml.2025-12-14T10-30-00.bak", false, "2025-12-14T10:30:00Z"},
		// M-cfg-bak fix: the sub-second collision form carries an internal dot in
		// the timestamp ("...T10-30-00.000"). It must round-trip, otherwise these
		// backups are invisible to ListBackups and unprunable by CleanOldBackups.
		{"config.yaml.2025-12-14T10-30-00.123.bak", false, "2025-12-14T10:30:00.123Z"},
		// A base name with no extra dots must still work in both forms.
		{"config.2025-12-14T10-30-00.bak", false, "2025-12-14T10:30:00Z"},
		{"config.2025-12-14T10-30-00.456.bak", false, "2025-12-14T10:30:00.456Z"},
		{"config.yaml.invalid.bak", true, ""},
		{"config.yaml.bak", true, ""},
		{"nodots.bak", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got, err := parseBackupTimestamp(tt.filename)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseBackupTimestamp(%q) error = %v, wantErr %v", tt.filename, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			want, perr := time.Parse(time.RFC3339Nano, tt.want)
			if perr != nil {
				t.Fatalf("bad test fixture %q: %v", tt.want, perr)
			}
			if !got.Equal(want) {
				t.Errorf("parseBackupTimestamp(%q) = %v, want %v", tt.filename, got, want)
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

// SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

const (
	// DefaultBackupDir is the default directory for config backups.
	DefaultBackupDir = "/etc/lyrebird/backups"

	// DefaultKeepBackups is the default number of backups to retain.
	DefaultKeepBackups = 10

	// BackupSuffix is the file extension for backup files.
	BackupSuffix = ".bak"

	// BackupTimestampFormat is the timestamp format used in backup filenames.
	// Format: YYYY-MM-DDTHH-MM-SS (ISO 8601 with dashes instead of colons for filesystem safety)
	BackupTimestampFormat = "2006-01-02T15-04-05"
)

// BackupInfo contains information about a backup file.
type BackupInfo struct {
	Path      string    // Full path to backup file
	Name      string    // Filename only
	Timestamp time.Time // When backup was created
	Size      int64     // File size in bytes
}

// BackupConfig creates a timestamped backup of a configuration file.
//
// The backup is stored in the backup directory with format:
//
//	{original_filename}.{timestamp}.bak
//
// Example:
//
//	config.yaml.2025-12-14T10-30-00.bak
//
// Parameters:
//   - configPath: Path to the configuration file to backup
//   - backupDir: Directory to store backups (created if doesn't exist)
//
// Returns:
//   - string: Path to the created backup file
//   - error: if file can't be read or backup can't be written
//
// Reference: lyrebird-mic-check.sh backup_config() lines 1050-1100
func BackupConfig(configPath, backupDir string) (string, error) {
	// Verify source exists
	info, err := os.Stat(configPath)
	if err != nil {
		return "", fmt.Errorf("config file not found: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("config path is a directory, not a file")
	}

	// Create backup directory if needed
	// #nosec G301 -- backup directory needs to be accessible
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Read source file
	// #nosec G304 -- configPath is user-provided config file path
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read config file: %w", err)
	}

	// Generate backup filename
	baseName := filepath.Base(configPath)
	timestamp := time.Now().Format(BackupTimestampFormat)
	backupName := fmt.Sprintf("%s.%s%s", baseName, timestamp, BackupSuffix)
	backupPath := filepath.Join(backupDir, backupName)

	// Check if backup already exists (same second)
	if _, err := os.Stat(backupPath); err == nil {
		// Add milliseconds to make unique
		timestamp = time.Now().Format("2006-01-02T15-04-05.000")
		backupName = fmt.Sprintf("%s.%s%s", baseName, timestamp, BackupSuffix)
		backupPath = filepath.Join(backupDir, backupName)
	}

	// Write backup with restrictive permissions
	if err := os.WriteFile(backupPath, data, 0600); err != nil {
		return "", fmt.Errorf("failed to write backup: %w", err)
	}

	return backupPath, nil
}

// ListBackups returns all backup files in the backup directory.
//
// Backups are returned sorted by timestamp, newest first.
//
// Parameters:
//   - backupDir: Directory containing backups
//   - configName: Original config filename to filter (e.g., "config.yaml")
//     If empty, all backups are returned.
//
// Returns:
//   - []BackupInfo: List of backup files with metadata
//   - error: if directory can't be read
func ListBackups(backupDir, configName string) ([]BackupInfo, error) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No backups yet
		}
		return nil, fmt.Errorf("failed to read backup directory: %w", err)
	}

	var backups []BackupInfo

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Must end with .bak
		if !strings.HasSuffix(name, BackupSuffix) {
			continue
		}

		// Filter by config name if specified
		if configName != "" && !strings.HasPrefix(name, configName+".") {
			continue
		}

		// Extract timestamp from filename
		// Format: config.yaml.2025-12-14T10-30-00.bak
		timestamp, err := parseBackupTimestamp(name)
		if err != nil {
			continue // Skip files with invalid timestamp format
		}

		// Get file info
		info, err := entry.Info()
		if err != nil {
			continue
		}

		backups = append(backups, BackupInfo{
			Path:      filepath.Join(backupDir, name),
			Name:      name,
			Timestamp: timestamp,
			Size:      info.Size(),
		})
	}

	// Sort by timestamp, newest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Timestamp.After(backups[j].Timestamp)
	})

	return backups, nil
}

// RestoreBackup restores a configuration file from a backup.
//
// This creates a backup of the current config before restoring,
// unless the current config doesn't exist.
//
// Parameters:
//   - backupPath: Path to the backup file to restore
//   - configPath: Path where config should be restored
//   - backupDir: Directory to store backup of current config (before restore)
//
// Returns:
//   - string: Path to backup of previous config (empty if none existed)
//   - error: if restore fails
func RestoreBackup(backupPath, configPath, backupDir string) (string, error) {
	// Verify backup exists
	if _, err := os.Stat(backupPath); err != nil {
		return "", fmt.Errorf("backup file not found: %w", err)
	}

	// Read backup content
	// #nosec G304 -- backupPath is from controlled backup directory
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return "", fmt.Errorf("failed to read backup: %w", err)
	}

	// Validate backup content is valid YAML
	if err := validateYAMLSyntax(data); err != nil {
		return "", fmt.Errorf("backup contains invalid YAML: %w", err)
	}

	// Backup current config if it exists
	var previousBackup string
	if _, err := os.Stat(configPath); err == nil {
		previousBackup, err = BackupConfig(configPath, backupDir)
		if err != nil {
			return "", fmt.Errorf("failed to backup current config before restore: %w", err)
		}
	}

	// Create parent directory if needed
	// #nosec G301 -- config directory needs to be accessible
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return previousBackup, fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write restored config
	// #nosec G306 -- config file needs to be readable by service
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return previousBackup, fmt.Errorf("failed to restore config: %w", err)
	}

	return previousBackup, nil
}

// CleanOldBackups removes old backups, keeping only the most recent ones.
//
// Parameters:
//   - backupDir: Directory containing backups
//   - configName: Original config filename to filter (e.g., "config.yaml")
//   - keepCount: Number of most recent backups to keep
//
// Returns:
//   - int: Number of backups deleted
//   - error: if cleanup fails
func CleanOldBackups(backupDir, configName string, keepCount int) (int, error) {
	if keepCount < 0 {
		return 0, fmt.Errorf("keepCount must be non-negative")
	}

	backups, err := ListBackups(backupDir, configName)
	if err != nil {
		return 0, err
	}

	// Nothing to clean if we have fewer backups than keepCount
	if len(backups) <= keepCount {
		return 0, nil
	}

	// Delete oldest backups (list is sorted newest-first)
	deleted := 0
	for _, backup := range backups[keepCount:] {
		if err := os.Remove(backup.Path); err != nil {
			// Log but continue trying to delete others
			continue
		}
		deleted++
	}

	return deleted, nil
}

// parseBackupTimestamp extracts the timestamp from a backup filename.
//
// Expected format: config.yaml.2025-12-14T10-30-00.bak
func parseBackupTimestamp(filename string) (time.Time, error) {
	// Remove .bak suffix
	name := strings.TrimSuffix(filename, BackupSuffix)

	// Find timestamp part (last component after splitting by dots)
	parts := strings.Split(name, ".")

	// Need at least 2 parts: filename and timestamp
	if len(parts) < 2 {
		return time.Time{}, fmt.Errorf("invalid backup filename format")
	}

	// Timestamp is the last part
	timestampStr := parts[len(parts)-1]

	// Handle millisecond format
	formats := []string{
		BackupTimestampFormat,
		"2006-01-02T15-04-05.000",
	}

	var t time.Time
	var err error
	for _, format := range formats {
		t, err = time.Parse(format, timestampStr)
		if err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("invalid timestamp format: %s", timestampStr)
}

// validateYAMLSyntax checks if data is valid YAML.
func validateYAMLSyntax(data []byte) error {
	var v interface{}
	return yaml.Unmarshal(data, &v)
}

// BackupBeforeSave creates a backup before saving config changes.
//
// This is a convenience function that combines backup + save operations.
//
// Parameters:
//   - cfg: Configuration to save
//   - configPath: Path to save config
//   - backupDir: Directory for backup
//
// Returns:
//   - string: Path to backup file created
//   - error: if backup or save fails
func BackupBeforeSave(cfg *Config, configPath, backupDir string) (string, error) {
	// Create backup of existing config (if exists)
	var backupPath string
	if _, err := os.Stat(configPath); err == nil {
		var err error
		backupPath, err = BackupConfig(configPath, backupDir)
		if err != nil {
			return "", fmt.Errorf("backup failed: %w", err)
		}
	}

	// Save new config
	if err := cfg.Save(configPath); err != nil {
		return backupPath, fmt.Errorf("save failed: %w", err)
	}

	return backupPath, nil
}

// GetBackupDir returns the appropriate backup directory for a config path.
//
// If configPath is in /etc/lyrebird/, uses /etc/lyrebird/backups/
// Otherwise, uses a 'backups' subdirectory next to the config.
func GetBackupDir(configPath string) string {
	dir := filepath.Dir(configPath)

	if strings.HasPrefix(dir, "/etc/lyrebird") {
		return DefaultBackupDir
	}

	return filepath.Join(dir, "backups")
}

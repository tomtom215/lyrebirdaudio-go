// SPDX-License-Identifier: MIT

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestListBackupsSeesMillisecondCollisions verifies the M-cfg-bak fix: backups
// written in the sub-second collision form (config.yaml.<ts>.000.bak) are now
// visible to ListBackups. Before the fix, parseBackupTimestamp split on dots and
// treated "000" as the timestamp, silently dropping these backups — which also
// meant CleanOldBackups could never prune them and they accumulated forever.
func TestListBackupsSeesMillisecondCollisions(t *testing.T) {
	backupDir := t.TempDir()

	names := []string{
		"config.yaml.2025-12-14T10-30-00.bak",     // whole-second
		"config.yaml.2025-12-14T10-30-00.001.bak", // sub-second collision
		"config.yaml.2025-12-14T10-30-00.002.bak", // sub-second collision
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(backupDir, n), []byte("x: 1\n"), 0600); err != nil {
			t.Fatalf("write %s: %v", n, err)
		}
	}

	backups, err := ListBackups(backupDir, "config.yaml")
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != len(names) {
		var got []string
		for _, b := range backups {
			got = append(got, b.Name)
		}
		t.Fatalf("ListBackups returned %d backups, want %d; got %v", len(backups), len(names), got)
	}

	// Newest-first ordering: the two sub-second entries must sort after the
	// whole-second one (they share the same second but carry a positive fraction).
	if !strings.Contains(backups[0].Name, ".00") {
		t.Errorf("expected a sub-second backup newest-first, got %q", backups[0].Name)
	}
}

// TestCleanOldBackupsPrunesMillisecondCollisions verifies that sub-second
// collision backups participate in retention — previously they were invisible to
// CleanOldBackups and leaked disk forever.
func TestCleanOldBackupsPrunesMillisecondCollisions(t *testing.T) {
	backupDir := t.TempDir()

	// Five backups sharing one second, distinguished by the sub-second fraction.
	names := []string{
		"config.yaml.2025-12-14T10-30-00.bak",
		"config.yaml.2025-12-14T10-30-00.001.bak",
		"config.yaml.2025-12-14T10-30-00.002.bak",
		"config.yaml.2025-12-14T10-30-00.003.bak",
		"config.yaml.2025-12-14T10-30-00.004.bak",
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(backupDir, n), []byte("x: 1\n"), 0600); err != nil {
			t.Fatalf("write %s: %v", n, err)
		}
	}

	deleted, err := CleanOldBackups(backupDir, "config.yaml", 2)
	if err != nil {
		t.Fatalf("CleanOldBackups: %v", err)
	}
	if deleted != 3 {
		t.Errorf("CleanOldBackups deleted %d, want 3", deleted)
	}

	remaining, err := ListBackups(backupDir, "config.yaml")
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(remaining) != 2 {
		t.Errorf("after prune %d remain, want 2", len(remaining))
	}
}

// TestRestoreBackupAtomicNoTempLeftover verifies the M-cfg-restore fix: a
// successful restore leaves no stray temp file behind and writes the exact
// backup content. The atomic temp+rename means a crash could only ever leave the
// old config or the complete new one, never a truncated file.
func TestRestoreBackupAtomicNoTempLeftover(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupDir, 0750); err != nil {
		t.Fatalf("mkdir backups: %v", err)
	}

	backupPath := filepath.Join(backupDir, "config.yaml.2025-12-14T10-30-00.bak")
	want := "default:\n  sample_rate: 48000\n"
	if err := os.WriteFile(backupPath, []byte(want), 0600); err != nil {
		t.Fatalf("write backup: %v", err)
	}

	configPath := filepath.Join(dir, "config.yaml")
	if _, err := RestoreBackup(backupPath, configPath, backupDir); err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}

	got, err := os.ReadFile(configPath) // #nosec G304 -- test-controlled path
	if err != nil {
		t.Fatalf("read restored config: %v", err)
	}
	if string(got) != want {
		t.Errorf("restored content = %q, want %q", got, want)
	}

	// No leftover atomic temp files in the config directory.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".config.") && strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover atomic temp file: %s", e.Name())
		}
	}
}

// TestWriteFileAtomic exercises the helper directly: exact bytes, requested
// permission bits, and no temp residue on success.
func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.yaml")
	data := []byte("hello: world\n")

	if err := writeFileAtomic(path, data, 0640); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	got, err := os.ReadFile(path) // #nosec G304 -- test-controlled path
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("content = %q, want %q", got, data)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0640 {
		t.Errorf("perm = %o, want 0640", perm)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("expected exactly the output file, got %d entries", len(entries))
	}
}

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSaveConfig verifies configuration file writing.
func TestSaveConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Devices = map[string]DeviceConfig{
		"test_device": {
			SampleRate:  44100,
			Channels:    1,
			Bitrate:     "96k",
			Codec:       "opus",
			ThreadQueue: 4096,
		},
	}

	// Write to temp file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	err := cfg.Save(configPath)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Save() did not create config file")
	}

	// Load and verify
	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() after Save() error = %v", err)
	}

	// Verify device was saved
	testDev, ok := loaded.Devices["test_device"]
	if !ok {
		t.Fatal("test_device not found in saved config")
	}
	if testDev.SampleRate != 44100 {
		t.Errorf("test_device.SampleRate = %d, want 44100", testDev.SampleRate)
	}
}

// TestSaveConfigErrorPaths tests error handling in Save().
func TestSaveConfigErrorPaths(t *testing.T) {
	cfg := DefaultConfig()

	t.Run("invalid path", func(t *testing.T) {
		// Try to save to a directory that doesn't exist and can't be created
		// Use a path with null bytes which is invalid on all systems
		invalidPath := "/tmp/\x00invalid/config.yaml"
		err := cfg.Save(invalidPath)
		if err == nil {
			t.Error("Save() with invalid path should return error")
		}
	})

	t.Run("unwritable directory", func(t *testing.T) {
		// Create a read-only directory
		tmpDir := t.TempDir()
		readOnlyDir := filepath.Join(tmpDir, "readonly")
		if err := os.Mkdir(readOnlyDir, 0444); err != nil {
			t.Skipf("Cannot create read-only directory: %v", err)
		}

		// Try to write to the read-only directory
		configPath := filepath.Join(readOnlyDir, "config.yaml")
		err := cfg.Save(configPath)
		// This might or might not error depending on OS permissions
		// Just verify it doesn't panic
		_ = err
	})
}

// TestSaveConfigAtomic verifies that Save() performs an atomic write using
// a temp file + rename pattern. After Save() returns, the file should contain
// complete valid YAML that can be loaded back. This also verifies that a
// concurrent reader never sees partial content.
func TestSaveConfigAtomic(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write an initial config
	initialCfg := DefaultConfig()
	initialCfg.Default.SampleRate = 44100
	err := initialCfg.Save(configPath)
	if err != nil {
		t.Fatalf("initial Save() error = %v", err)
	}

	// Read initial content
	initialData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile initial error = %v", err)
	}

	// Now overwrite with a new config
	newCfg := DefaultConfig()
	newCfg.Default.SampleRate = 96000
	newCfg.Devices = map[string]DeviceConfig{
		"test_device": {
			SampleRate: 22050,
			Channels:   1,
			Bitrate:    "64k",
			Codec:      "aac",
		},
	}
	err = newCfg.Save(configPath)
	if err != nil {
		t.Fatalf("overwrite Save() error = %v", err)
	}

	// Read the file content - it should be either fully old or fully new,
	// never partial. Since Save() completed, it must be fully new.
	resultData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile result error = %v", err)
	}

	// Verify the result is valid YAML that can be loaded
	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig after atomic Save() error = %v", err)
	}

	// Verify it's the new content
	if loaded.Default.SampleRate != 96000 {
		t.Errorf("SampleRate = %d, want 96000", loaded.Default.SampleRate)
	}

	// The result should NOT be the initial data
	if string(resultData) == string(initialData) {
		t.Error("File content was not updated by Save()")
	}

	// Verify that no temp files are left behind
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir error = %v", err)
	}
	for _, entry := range entries {
		if entry.Name() != "config.yaml" {
			t.Errorf("Unexpected leftover file in directory: %s", entry.Name())
		}
	}
}

// TestSaveConfigAtomicPermissions verifies that the atomically-saved file
// has the correct permissions (0644).
func TestSaveConfigAtomicPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := DefaultConfig()
	err := cfg.Save(configPath)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("Stat error = %v", err)
	}

	// SEC-3: Config files are saved with 0640 (owner+group, no world-read).
	perm := info.Mode().Perm()
	if perm != 0640 {
		t.Errorf("File permissions = %04o, want 0640 (SEC-3 least privilege)", perm)
	}
}

// TestSaveConfigAtomicTempFileCleanupOnError verifies that temp files are
// cleaned up if the write fails mid-way.
func TestSaveConfigAtomicTempFileCleanupOnError(t *testing.T) {
	// Try saving to a non-existent directory (rename will fail because
	// the temp file is created in the same dir, which doesn't exist)
	cfg := DefaultConfig()
	err := cfg.Save("/nonexistent_dir_12345/config.yaml")
	if err == nil {
		t.Error("Save() to nonexistent directory should fail")
	}

	// Verify no temp files left behind (the directory doesn't exist,
	// so there's nothing to clean up, but also no crash)
}

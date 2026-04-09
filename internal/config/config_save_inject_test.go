package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockAtomicFile implements atomicFile for testing error injection.
type mockAtomicFile struct {
	name       string
	realFile   *os.File // used to back Name() and cleanup
	writeErr   error
	syncErr    error
	chmodErr   error
	closeErr   error
	writeCalls int
}

func (m *mockAtomicFile) Write(p []byte) (int, error) {
	m.writeCalls++
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	return len(p), nil
}

func (m *mockAtomicFile) Sync() error { return m.syncErr }

func (m *mockAtomicFile) Chmod(_ os.FileMode) error { return m.chmodErr }

func (m *mockAtomicFile) Close() error {
	if m.realFile != nil {
		_ = m.realFile.Close()
	}
	return m.closeErr
}

func (m *mockAtomicFile) Name() string { return m.name }

// newMockCreateTemp returns a createTemp func that produces a mockAtomicFile.
// A real temp file is created so cleanup (os.Remove) has a real path to remove.
func newMockCreateTemp(dir string, mock *mockAtomicFile) atomicCreateTemp {
	return func(d, pattern string) (atomicFile, error) {
		f, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return nil, err
		}
		mock.realFile = f
		mock.name = f.Name()
		return mock, nil
	}
}

// TestSaveWithInjectableErrors tests the error paths of saveWith.
func TestSaveWithInjectableErrors(t *testing.T) {
	cfg := DefaultConfig()

	t.Run("write error", func(t *testing.T) {
		tmpDir := t.TempDir()
		mock := &mockAtomicFile{writeErr: errors.New("disk full")}
		err := cfg.saveWith(filepath.Join(tmpDir, "config.yaml"), newMockCreateTemp(tmpDir, mock))
		if err == nil {
			t.Fatal("saveWith() expected error on write failure")
		}
		if !strings.Contains(err.Error(), "failed to write temp config file") {
			t.Errorf("error = %q, want 'failed to write temp config file'", err.Error())
		}
	})

	t.Run("sync error", func(t *testing.T) {
		tmpDir := t.TempDir()
		mock := &mockAtomicFile{syncErr: errors.New("sync failed")}
		err := cfg.saveWith(filepath.Join(tmpDir, "config.yaml"), newMockCreateTemp(tmpDir, mock))
		if err == nil {
			t.Fatal("saveWith() expected error on sync failure")
		}
		if !strings.Contains(err.Error(), "failed to sync temp config file") {
			t.Errorf("error = %q, want 'failed to sync temp config file'", err.Error())
		}
	})

	t.Run("chmod error", func(t *testing.T) {
		tmpDir := t.TempDir()
		mock := &mockAtomicFile{chmodErr: errors.New("chmod failed")}
		err := cfg.saveWith(filepath.Join(tmpDir, "config.yaml"), newMockCreateTemp(tmpDir, mock))
		if err == nil {
			t.Fatal("saveWith() expected error on chmod failure")
		}
		if !strings.Contains(err.Error(), "failed to set config file permissions") {
			t.Errorf("error = %q, want 'failed to set config file permissions'", err.Error())
		}
	})

	t.Run("close error", func(t *testing.T) {
		tmpDir := t.TempDir()
		mock := &mockAtomicFile{closeErr: errors.New("close failed")}
		err := cfg.saveWith(filepath.Join(tmpDir, "config.yaml"), newMockCreateTemp(tmpDir, mock))
		if err == nil {
			t.Fatal("saveWith() expected error on close failure")
		}
		if !strings.Contains(err.Error(), "failed to close temp config file") {
			t.Errorf("error = %q, want 'failed to close temp config file'", err.Error())
		}
	})

	t.Run("createTemp error", func(t *testing.T) {
		failCreate := func(dir, pattern string) (atomicFile, error) {
			return nil, errors.New("createTemp failed")
		}
		err := cfg.saveWith("/tmp/config.yaml", failCreate)
		if err == nil {
			t.Fatal("saveWith() expected error when createTemp fails")
		}
		if !strings.Contains(err.Error(), "failed to create temp config file") {
			t.Errorf("error = %q, want 'failed to create temp config file'", err.Error())
		}
	})
}

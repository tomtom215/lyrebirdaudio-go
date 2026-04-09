package stream

import (
	"bytes"
	"fmt"
	"testing"
	"time"
)

// TestManagerClose tests the Manager.Close() method.
func TestManagerClose(t *testing.T) {
	t.Run("no log writer - returns nil", func(t *testing.T) {
		cfg := &ManagerConfig{
			DeviceName: "test_device",
			ALSADevice: "hw:0,0",
			StreamName: "test",
			SampleRate: 48000,
			Channels:   2,
			Bitrate:    "128k",
			Codec:      "opus",
			RTSPURL:    "rtsp://localhost:8554/test",
			LockDir:    t.TempDir(),
			FFmpegPath: "/usr/bin/ffmpeg",
			Backoff:    NewBackoff(1*time.Second, 10*time.Second, 3),
			// LogDir intentionally omitted so logWriter stays nil
		}
		mgr, err := NewManager(cfg)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}
		if err := mgr.Close(); err != nil {
			t.Errorf("Close() with nil logWriter returned error: %v", err)
		}
	})

	t.Run("idempotent - second close is safe", func(t *testing.T) {
		cfg := &ManagerConfig{
			DeviceName: "test_device",
			ALSADevice: "hw:0,0",
			StreamName: "test",
			SampleRate: 48000,
			Channels:   2,
			Bitrate:    "128k",
			Codec:      "opus",
			RTSPURL:    "rtsp://localhost:8554/test",
			LockDir:    t.TempDir(),
			FFmpegPath: "/usr/bin/ffmpeg",
			Backoff:    NewBackoff(1*time.Second, 10*time.Second, 3),
		}
		mgr, err := NewManager(cfg)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}
		// Calling Close twice must not panic or error
		if err := mgr.Close(); err != nil {
			t.Errorf("first Close() error = %v", err)
		}
		if err := mgr.Close(); err != nil {
			t.Errorf("second Close() error = %v (should be idempotent)", err)
		}
	})

	t.Run("with log writer - closes and nils writer", func(t *testing.T) {
		logDir := t.TempDir()
		cfg := &ManagerConfig{
			DeviceName: "test_device",
			ALSADevice: "hw:0,0",
			StreamName: "test",
			SampleRate: 48000,
			Channels:   2,
			Bitrate:    "128k",
			Codec:      "opus",
			RTSPURL:    "rtsp://localhost:8554/test",
			LockDir:    t.TempDir(),
			FFmpegPath: "/usr/bin/ffmpeg",
			LogDir:     logDir,
			Backoff:    NewBackoff(1*time.Second, 10*time.Second, 3),
		}
		mgr, err := NewManager(cfg)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}
		// Verify logWriter was created
		mgr.mu.RLock()
		hasWriter := mgr.logWriter != nil
		mgr.mu.RUnlock()
		if !hasWriter {
			t.Fatal("expected logWriter to be set when LogDir is provided")
		}

		if err := mgr.Close(); err != nil {
			t.Errorf("Close() with logWriter error = %v", err)
		}

		// Verify logWriter is nil after close
		mgr.mu.RLock()
		writerAfterClose := mgr.logWriter
		mgr.mu.RUnlock()
		if writerAfterClose != nil {
			t.Error("logWriter should be nil after Close()")
		}
	})

	t.Run("error from logWriter.Close propagates", func(t *testing.T) {
		cfg := &ManagerConfig{
			DeviceName: "test_device",
			ALSADevice: "hw:0,0",
			StreamName: "test",
			SampleRate: 48000,
			Channels:   2,
			Bitrate:    "128k",
			Codec:      "opus",
			RTSPURL:    "rtsp://localhost:8554/test",
			LockDir:    t.TempDir(),
			FFmpegPath: "/usr/bin/ffmpeg",
			Backoff:    NewBackoff(1*time.Second, 10*time.Second, 3),
		}
		mgr, err := NewManager(cfg)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		// Inject a writer whose Close fails
		mgr.mu.Lock()
		mgr.logWriter = &failingCloseWriter{}
		mgr.mu.Unlock()

		err = mgr.Close()
		if err == nil {
			t.Fatal("Close() should propagate error from logWriter.Close()")
		}
		if !bytes.Contains([]byte(err.Error()), []byte("close failed")) {
			t.Errorf("Close() error = %q, want 'close failed'", err.Error())
		}
	})
}

// failingCloseWriter is an io.WriteCloser whose Close always fails.
type failingCloseWriter struct{}

func (f *failingCloseWriter) Write(p []byte) (int, error) { return len(p), nil }

func (f *failingCloseWriter) Close() error { return fmt.Errorf("close failed") }

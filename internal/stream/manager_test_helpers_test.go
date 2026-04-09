package stream

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// testLogWriter wraps *testing.T to implement io.Writer for slog output.
type testLogWriter struct {
	t *testing.T
}

func (tl *testLogWriter) Write(p []byte) (n int, err error) {
	tl.t.Log(string(p))
	return len(p), nil
}

// newTestLogger creates an *slog.Logger that writes to testing.T.
func newTestLogger(t *testing.T) *slog.Logger {
	return slog.New(slog.NewTextHandler(&testLogWriter{t: t}, nil))
}

// getTestAudioDevice returns an appropriate audio device for testing.
// In CI environments without ALSA, it returns a lavfi virtual audio source.
// On systems with ALSA, it returns hw:0,0.
func getTestAudioDevice(t *testing.T) (device, inputFormat string) {
	t.Helper()

	// Check if ALSA device exists
	if _, err := os.Stat("/proc/asound/card0"); err == nil {
		return "hw:0,0", "alsa"
	}

	// Fall back to lavfi virtual audio (null audio source)
	// This generates silence for testing with 600 second duration
	return "anullsrc=r=48000:cl=stereo:d=600", "lavfi"
}

// getTestOutputURL returns an appropriate output URL for testing.
// Uses temporary file output to avoid dependency on MediaMTX server.
func getTestOutputURL(t *testing.T, name string) string {
	t.Helper()
	// Use temporary file for output with .m4a extension
	// M4A is the standard container for AAC codec
	tmpFile := filepath.Join(t.TempDir(), name+".m4a")
	return tmpFile
}

func waitForState(t *testing.T, mgr *Manager, want State, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if mgr.State() == want {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Check one final time after timeout (state might have changed during sleep)
	if mgr.State() == want {
		return true
	}

	t.Logf("Timeout waiting for state %v, current state: %v", want, mgr.State())
	return false
}

func findFFmpegOrSkip(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not found in PATH, skipping test")
	}
	return path
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

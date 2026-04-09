package stream

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestFFmpegDiagnostic is a diagnostic test to see FFmpeg's actual error output.
// This helps debug why the integration tests are failing.
func TestFFmpegDiagnostic(t *testing.T) {
	ffmpegPath := findFFmpegOrSkip(t)
	device, inputFormat := getTestAudioDevice(t)

	// Try AAC codec first (built into FFmpeg, no external libs needed)
	outputFile := filepath.Join(t.TempDir(), "diagnostic.m4a")

	// Build command with AAC codec
	args := []string{
		"-f", inputFormat,
		"-i", device,
		"-ar", "48000",
		"-ac", "2",
		"-c:a", "aac",
		"-b:a", "128k",
		outputFile,
	}

	cmd := exec.Command(ffmpegPath, args...)

	// Capture stderr
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	t.Logf("Running FFmpeg with command: %s %v", ffmpegPath, args)

	// Start FFmpeg
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start FFmpeg: %v", err)
	}

	// Wait for it to either succeed or fail
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		// FFmpeg exited
		stderrOutput := stderr.String()
		t.Logf("FFmpeg stderr:\n%s", stderrOutput)

		if err != nil {
			t.Logf("FFmpeg exited with error: %v", err)
			t.Logf("This explains why integration tests are failing!")
		} else {
			t.Logf("FFmpeg completed successfully")
		}

	case <-time.After(3 * time.Second):
		// FFmpeg is running - kill it and wait for exit
		_ = cmd.Process.Kill()
		<-done // Wait for process to fully exit
		t.Logf("FFmpeg is running successfully after 3 seconds")
		t.Logf("FFmpeg stderr so far:\n%s", stderr.String())
	}
}

// TestStreamManagerFFmpegCommandGeneration verifies correct FFmpeg command construction.
func TestStreamManagerFFmpegCommandGeneration(t *testing.T) {
	tests := []struct {
		name      string
		cfg       *ManagerConfig
		wantArgs  []string
		wantNotIn []string
	}{
		{
			name: "aac codec stereo",
			cfg: &ManagerConfig{
				ALSADevice: "hw:0,0",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "192k",
				Codec:      "aac",
				RTSPURL:    getTestOutputURL(t, "test"),
			},
			wantArgs: []string{
				"-f", "alsa",
				"-i", "hw:0,0",
				"-ar", "48000",
				"-ac", "2",
				"-c:a", "aac",
				"-b:a", "192k",
			},
		},
		{
			name: "aac codec mono",
			cfg: &ManagerConfig{
				ALSADevice: "hw:1,0",
				SampleRate: 44100,
				Channels:   1,
				Bitrate:    "128k",
				Codec:      "aac",
				RTSPURL:    getTestOutputURL(t, "mono"),
			},
			wantArgs: []string{
				"-f", "alsa",
				"-i", "hw:1,0",
				"-ar", "44100",
				"-ac", "1",
				"-c:a", "aac",
				"-b:a", "128k",
			},
			wantNotIn: []string{"libopus"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := buildFFmpegCommand(context.Background(), tt.cfg)

			// Verify all expected args present
			for _, want := range tt.wantArgs {
				if !contains(cmd.Args, want) {
					t.Errorf("FFmpeg command missing arg: %s\nGot: %v", want, cmd.Args)
				}
			}

			// Verify unwanted args not present
			for _, notWant := range tt.wantNotIn {
				if contains(cmd.Args, notWant) {
					t.Errorf("FFmpeg command contains unwanted arg: %s\nGot: %v", notWant, cmd.Args)
				}
			}

			// Verify RTSP URL is last
			if len(cmd.Args) < 2 {
				t.Fatal("FFmpeg command too short")
			}
			lastArg := cmd.Args[len(cmd.Args)-1]
			if lastArg != tt.cfg.RTSPURL {
				t.Errorf("Last arg = %q, want RTSP URL %q", lastArg, tt.cfg.RTSPURL)
			}
		})
	}
}

// TestStartFFmpegCmdNilOnFailure is the C-5 regression test.
//
// If cmd.Start() fails, m.cmd must remain nil.  The original code assigned
// m.cmd before calling cmd.Start(), which left a stale pointer to an unstarted
// command.  A concurrent stop() call would then dereference cmd.Process (nil),
// causing a nil-pointer panic.
func TestStartFFmpegCmdNilOnFailure(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &ManagerConfig{
		DeviceName: "test_device",
		ALSADevice: "hw:0,0",
		StreamName: "test",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "opus",
		RTSPURL:    "rtsp://localhost:8554/test",
		LockDir:    tmpDir,
		// Non-existent binary: cmd.Start() will fail with "exec: not found".
		FFmpegPath: filepath.Join(tmpDir, "nonexistent-ffmpeg-binary"),
		Backoff:    NewBackoff(10*time.Millisecond, 50*time.Millisecond, 1),
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Call startFFmpeg directly (unexported, accessible from same package).
	ctx := context.Background()
	startErr := mgr.startFFmpeg(ctx)
	if startErr == nil {
		t.Fatal("startFFmpeg() should fail with non-existent binary")
	}

	// m.cmd must be nil: no process was started.
	mgr.mu.RLock()
	cmd := mgr.cmd
	mgr.mu.RUnlock()

	if cmd != nil {
		t.Error("C-5 regression: m.cmd must be nil when cmd.Start() fails")
	}
}

// TestStartFFmpegCreatesLocalRecordDir verifies that startFFmpeg auto-creates
// LocalRecordDir before launching FFmpeg (GAP-1a / A-1).
func TestStartFFmpegCreatesLocalRecordDir(t *testing.T) {
	tmpDir := t.TempDir()
	lockDir := t.TempDir()

	// Point to a directory that does NOT exist yet.
	recordDir := filepath.Join(tmpDir, "recordings", "nested")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := &ManagerConfig{
		DeviceName:     "test_device",
		ALSADevice:     "hw:0,0",
		StreamName:     "test",
		SampleRate:     48000,
		Channels:       2,
		Bitrate:        "128k",
		Codec:          "aac",
		RTSPURL:        "rtsp://localhost:8554/test",
		OutputFormat:   "rtsp",
		LockDir:        lockDir,
		FFmpegPath:     "/nonexistent/ffmpeg", // will fail to start, but dir should be created first
		Backoff:        NewBackoff(1*time.Second, 10*time.Second, 3),
		LocalRecordDir: recordDir,
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// startFFmpeg creates the dir, then fails to start because FFmpeg doesn't exist.
	_ = mgr.startFFmpeg(ctx)

	// The recording directory must have been created.
	if _, err := os.Stat(recordDir); os.IsNotExist(err) {
		t.Errorf("LocalRecordDir %q was not created by startFFmpeg", recordDir)
	}
}

// TestStartFFmpegNoLocalRecordDir verifies that startFFmpeg succeeds (or fails
// for unrelated reasons) when LocalRecordDir is empty.
func TestStartFFmpegNoLocalRecordDir(t *testing.T) {
	lockDir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := &ManagerConfig{
		DeviceName:     "test_device",
		ALSADevice:     "hw:0,0",
		StreamName:     "test",
		SampleRate:     48000,
		Channels:       2,
		Bitrate:        "128k",
		Codec:          "aac",
		RTSPURL:        "rtsp://localhost:8554/test",
		LockDir:        lockDir,
		FFmpegPath:     "/nonexistent/ffmpeg",
		Backoff:        NewBackoff(1*time.Second, 10*time.Second, 3),
		LocalRecordDir: "", // disabled
	}

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Should fail for ffmpeg not found, not for directory creation.
	err = mgr.startFFmpeg(ctx)
	if err == nil {
		t.Fatal("expected error from non-existent ffmpeg binary")
	}
	if !strings.Contains(err.Error(), "ffmpeg") {
		t.Errorf("expected ffmpeg error, got: %v", err)
	}
}

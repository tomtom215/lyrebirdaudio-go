// SPDX-License-Identifier: MIT

package stream

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// startFFmpeg starts the FFmpeg process and blocks until it exits.
func (m *Manager) startFFmpeg(ctx context.Context) error {
	if m.cfg.LocalRecordDir != "" {
		if err := os.MkdirAll(m.cfg.LocalRecordDir, 0750); err != nil {
			return fmt.Errorf("failed to create recording directory %q: %w", m.cfg.LocalRecordDir, err)
		}
	}

	cmd := buildFFmpegCommand(ctx, m.cfg)

	m.mu.Lock()
	if m.logWriter != nil {
		cmd.Stderr = m.logWriter
	}
	m.startTime = time.Now()
	m.mu.Unlock()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	m.mu.Lock()
	m.cmd = cmd
	m.mu.Unlock()

	m.setState(StateRunning)

	if m.resourceMonitor != nil && cmd.Process != nil && m.cfg.MonitorInterval > 0 {
		monitorCtx, cancel := context.WithCancel(ctx)
		m.mu.Lock()
		m.monitorCancel = cancel
		m.mu.Unlock()

		go m.resourceMonitor.MonitorProcess(
			monitorCtx,
			cmd.Process.Pid,
			m.cfg.MonitorInterval,
			m.cfg.AlertCallback,
		)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		m.stopMonitoring()
		m.stop()
		<-done
		return context.Canceled

	case err := <-done:
		m.stopMonitoring()
		m.mu.Lock()
		m.cmd = nil
		m.mu.Unlock()

		if err != nil {
			return fmt.Errorf("ffmpeg exited with error: %w", err)
		}
		return nil
	}
}

// stop stops the FFmpeg process gracefully.
func (m *Manager) stop() {
	m.setState(StateStopping)

	m.mu.Lock()
	cmd := m.cmd
	m.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		proc := cmd.Process
		_ = proc.Signal(os.Interrupt)

		stopTimeout := m.cfg.StopTimeout
		if stopTimeout <= 0 {
			stopTimeout = 5 * time.Second
		}

		killCtx, killCancel := context.WithTimeout(context.Background(), stopTimeout)
		go func() {
			defer killCancel()
			<-killCtx.Done()
			if killCtx.Err() == context.DeadlineExceeded {
				_ = proc.Kill()
			}
		}()
	}
}

// stopMonitoring stops the resource monitor goroutine.
func (m *Manager) stopMonitoring() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.monitorCancel != nil {
		m.monitorCancel()
		m.monitorCancel = nil
	}
}

// forceStop immediately kills the FFmpeg process (for testing).
func (m *Manager) forceStop() error {
	m.mu.Lock()
	cmd := m.cmd
	m.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		return cmd.Process.Kill()
	}
	return fmt.Errorf("no process to kill")
}

// buildFFmpegCommand constructs the FFmpeg command line.
func buildFFmpegCommand(ctx context.Context, cfg *ManagerConfig) *exec.Cmd {
	inputFormat := cfg.InputFormat
	if inputFormat == "" {
		inputFormat = "alsa"
	}

	args := []string{"-f", inputFormat}

	// A synthetic "lavfi" source (e.g. a test tone) generates frames as fast as
	// the CPU allows, so for LIVE streaming it must be paced to real time with
	// -re; otherwise it blasts many minutes of audio per wall-clock second and
	// the RTSP publish never settles into a healthy live state. A real ALSA
	// capture is already paced by the hardware clock, so it must NOT get -re
	// (that would double-pace it and drift). -re is an input option and must
	// precede -i.
	if inputFormat == "lavfi" {
		args = append(args, "-re")
	}

	args = append(args,
		"-i", cfg.ALSADevice,
		"-ar", fmt.Sprintf("%d", cfg.SampleRate),
		"-ac", fmt.Sprintf("%d", cfg.Channels),
	)

	if cfg.ThreadQueue > 0 {
		args = append(args, "-thread_queue_size", fmt.Sprintf("%d", cfg.ThreadQueue))
	}

	switch cfg.Codec {
	case "opus":
		args = append(args, "-c:a", "libopus")
	case "aac":
		args = append(args, "-c:a", "aac")
	}

	args = append(args, "-b:a", cfg.Bitrate)

	outputFormat := cfg.OutputFormat
	if outputFormat == "" {
		if strings.HasPrefix(cfg.RTSPURL, "rtsp://") {
			outputFormat = "rtsp"
		} else if cfg.RTSPURL == "-" || cfg.RTSPURL == "/dev/null" || strings.HasPrefix(cfg.RTSPURL, "pipe:") {
			outputFormat = "null"
		} else if strings.Contains(cfg.RTSPURL, "/") {
			outputFormat = ""
		} else {
			outputFormat = "rtsp"
		}
	}

	if cfg.LocalRecordDir != "" && outputFormat == "rtsp" {
		segDuration := cfg.SegmentDuration
		if segDuration <= 0 {
			segDuration = 3600
		}
		segFormat := cfg.SegmentFormat
		if segFormat == "" {
			segFormat = "wav"
		}
		segPattern := filepath.Join(cfg.LocalRecordDir, cfg.StreamName+"_%Y%m%d_%H%M%S."+segFormat)

		// onfail=ignore on the SEGMENT slave decouples local recording from the
		// live stream: ffmpeg's tee muxer defaults to onfail=abort, so without
		// this a failing segment write (a full or read-only recording disk, an
		// I/O error) would abort the ENTIRE ffmpeg process and drop the live RTSP
		// stream too. The live stream is the monitored primary function and must
		// survive local-disk problems. The RTSP slave keeps the default
		// (onfail=abort) so a genuine publish failure still exits ffmpeg promptly
		// and the manager's backoff restart re-establishes BOTH outputs (which
		// also recovers segment recording once the disk issue clears).
		teeOutput := fmt.Sprintf(
			"[f=rtsp:reconnect=1:reconnect_streamed=1:reconnect_delay_max=30]%s|[onfail=ignore:f=segment:segment_time=%d:strftime=1]%s",
			cfg.RTSPURL, segDuration, segPattern,
		)
		// The tee muxer does NOT perform ffmpeg's automatic stream selection the
		// way a normal single output does, so without an explicit -map it maps no
		// streams and aborts with "Output file does not contain any stream" — the
		// whole ffmpeg process exits before either slave opens, so local recording
		// never starts AND the live stream never comes up. Map the (single) audio
		// stream explicitly. This is required for the tee path only; the plain
		// -f rtsp path below relies on automatic selection, which works there.
		args = append(args, "-map", "0:a", "-f", "tee", teeOutput)
	} else if outputFormat == "rtsp" {
		args = append(args,
			"-reconnect", "1",
			"-reconnect_streamed", "1",
			"-reconnect_delay_max", "30",
			"-f", outputFormat, cfg.RTSPURL,
		)
	} else if outputFormat != "" {
		args = append(args, "-f", outputFormat, cfg.RTSPURL)
	} else {
		args = append(args, cfg.RTSPURL)
	}

	// Intentionally exec.Command, NOT exec.CommandContext(ctx): tying the
	// process to the shutdown context makes os/exec send SIGKILL the instant the
	// context is cancelled, which truncates the in-progress recording segment
	// (the tee/segment muxer never writes its container trailer) on every
	// graceful shutdown, hot-reload, or stall-triggered restart. Instead,
	// Manager.stop() owns shutdown: it sends a single SIGINT so ffmpeg can flush
	// and finalize, then escalates to SIGKILL only after StopTimeout. Sending a
	// second SIGINT (which os/exec's default cancel would race with) would make
	// ffmpeg force-quit without finalizing, so there must be exactly one.
	// ctx is retained in the signature for API stability.
	_ = ctx
	// #nosec G204 - FFmpegPath is from validated configuration, not user input
	cmd := exec.Command(cfg.FFmpegPath, args...)

	return cmd
}

// validateConfig validates manager configuration.
func validateConfig(cfg *ManagerConfig) error {
	if cfg.DeviceName == "" {
		return fmt.Errorf("device name cannot be empty")
	}
	if cfg.ALSADevice == "" {
		return fmt.Errorf("ALSA device cannot be empty")
	}
	if cfg.StreamName == "" {
		return fmt.Errorf("stream name cannot be empty")
	}
	if cfg.SampleRate <= 0 {
		return fmt.Errorf("sample rate must be positive")
	}
	if cfg.Channels <= 0 || cfg.Channels > 32 {
		return fmt.Errorf("channels must be between 1 and 32")
	}
	if cfg.Bitrate == "" {
		return fmt.Errorf("bitrate cannot be empty")
	}
	if cfg.Codec != "opus" && cfg.Codec != "aac" {
		return fmt.Errorf("codec must be opus or aac")
	}
	if cfg.RTSPURL == "" {
		return fmt.Errorf("RTSP URL cannot be empty")
	}
	if cfg.LockDir == "" {
		return fmt.Errorf("lock directory cannot be empty")
	}
	if cfg.FFmpegPath == "" {
		return fmt.Errorf("FFmpeg path cannot be empty")
	}
	if cfg.Backoff == nil {
		return fmt.Errorf("backoff policy cannot be nil")
	}
	return nil
}

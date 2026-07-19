package stream

import (
	"context"
	"strings"
	"testing"
)

// TestBuildFFmpegCommandThreadQueue verifies thread queue handling.
func TestBuildFFmpegCommandThreadQueue(t *testing.T) {
	tests := []struct {
		name        string
		threadQueue int
		wantArg     bool
	}{
		{"with thread queue", 8192, true},
		{"without thread queue", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ManagerConfig{
				ALSADevice:  "hw:0,0",
				SampleRate:  48000,
				Channels:    2,
				Bitrate:     "128k",
				Codec:       "opus",
				ThreadQueue: tt.threadQueue,
				RTSPURL:     "rtsp://localhost:8554/test",
			}

			cmd := buildFFmpegCommand(context.Background(), cfg)

			hasThreadQueue := false
			for _, arg := range cmd.Args {
				if arg == "-thread_queue_size" {
					hasThreadQueue = true
					break
				}
			}

			if hasThreadQueue != tt.wantArg {
				t.Errorf("thread_queue_size in args = %v, want %v", hasThreadQueue, tt.wantArg)
			}
		})
	}
}

// TestBuildFFmpegCommandInputFormat verifies input format handling.
func TestBuildFFmpegCommandInputFormat(t *testing.T) {
	tests := []struct {
		name        string
		inputFormat string
		wantFormat  string
	}{
		{"alsa format", "alsa", "alsa"},
		{"lavfi format", "lavfi", "lavfi"},
		{"empty defaults to alsa", "", "alsa"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ManagerConfig{
				ALSADevice:  "hw:0,0",
				InputFormat: tt.inputFormat,
				SampleRate:  48000,
				Channels:    2,
				Bitrate:     "128k",
				Codec:       "opus",
				RTSPURL:     "rtsp://localhost:8554/test",
			}

			cmd := buildFFmpegCommand(context.Background(), cfg)

			// Find -f flag
			foundFormat := false
			for i, arg := range cmd.Args {
				if arg == "-f" && i+1 < len(cmd.Args) {
					if cmd.Args[i+1] == tt.wantFormat {
						foundFormat = true
					} else {
						t.Errorf("input format = %q, want %q", cmd.Args[i+1], tt.wantFormat)
					}
					break
				}
			}

			if !foundFormat {
				t.Errorf("input format %q not found in command", tt.wantFormat)
			}
		})
	}
}

// TestBuildFFmpegCommandRealtimePacing verifies that RealtimeInput adds -re
// (required for a healthy live publish from a synthetic source), while the
// default (a hardware ALSA capture, already paced by the device clock) does not.
func TestBuildFFmpegCommandRealtimePacing(t *testing.T) {
	tests := []struct {
		name     string
		realtime bool
		wantRe   bool
	}{
		{"realtime input is paced with -re", true, true},
		{"default (hardware) is not paced", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ManagerConfig{
				ALSADevice:    "hw:0,0",
				RealtimeInput: tt.realtime,
				StreamName:    "test",
				SampleRate:    48000,
				Channels:      2,
				Bitrate:       "128k",
				Codec:         "opus",
				RTSPURL:       "rtsp://localhost:8554/test",
			}

			cmd := buildFFmpegCommand(context.Background(), cfg)

			reIdx, iIdx := -1, -1
			for i, arg := range cmd.Args {
				if arg == "-re" {
					reIdx = i
				}
				if arg == "-i" && iIdx == -1 {
					iIdx = i
				}
			}

			hasRe := reIdx != -1
			if hasRe != tt.wantRe {
				t.Errorf("-re present = %v, want %v; args=%v", hasRe, tt.wantRe, cmd.Args)
			}
			if tt.wantRe && (iIdx == -1 || reIdx > iIdx) {
				t.Errorf("-re must appear before -i; args=%v", cmd.Args)
			}
		})
	}
}

// TestBuildFFmpegCommandRTSPTransportTCP verifies both RTSP publish paths (plain
// and tee) publish over TCP, so RTP is delivered losslessly and in order to
// MediaMTX instead of over drop-prone UDP.
func TestBuildFFmpegCommandRTSPTransportTCP(t *testing.T) {
	base := ManagerConfig{
		ALSADevice: "hw:0,0",
		StreamName: "test",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "opus",
		RTSPURL:    "rtsp://localhost:8554/test",
	}

	t.Run("plain rtsp publishes over tcp", func(t *testing.T) {
		cfg := base
		cmd := buildFFmpegCommand(context.Background(), &cfg)
		found := false
		for i, arg := range cmd.Args {
			if arg == "-rtsp_transport" && i+1 < len(cmd.Args) && cmd.Args[i+1] == "tcp" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("plain rtsp output must include -rtsp_transport tcp, got: %v", cmd.Args)
		}
	})

	t.Run("tee rtsp slave publishes over tcp", func(t *testing.T) {
		cfg := base
		cfg.LocalRecordDir = "/tmp/rec"
		cmd := buildFFmpegCommand(context.Background(), &cfg)
		last := cmd.Args[len(cmd.Args)-1]
		if !strings.Contains(last, "rtsp_transport=tcp") {
			t.Errorf("tee rtsp slave must include rtsp_transport=tcp, got: %s", last)
		}
	})
}

// TestBuildFFmpegCommandOutputFormat verifies output format handling.
func TestBuildFFmpegCommandOutputFormat(t *testing.T) {
	tests := []struct {
		name         string
		rtspURL      string
		outputFormat string
		wantFormat   string
	}{
		{"rtsp URL auto-detect", "rtsp://localhost:8554/test", "", "rtsp"},
		{"pipe URL auto-detect", "pipe:1", "", "null"},
		{"stdout auto-detect", "-", "", "null"},
		{"devnull auto-detect", "/dev/null", "", "null"},
		{"explicit rtsp format", "rtsp://localhost:8554/test", "rtsp", "rtsp"},
		{"explicit null format", "/dev/null", "null", "null"},
		{"file path auto-detect", "/tmp/test.ogg", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ManagerConfig{
				ALSADevice:   "hw:0,0",
				SampleRate:   48000,
				Channels:     2,
				Bitrate:      "128k",
				Codec:        "opus",
				RTSPURL:      tt.rtspURL,
				OutputFormat: tt.outputFormat,
			}

			cmd := buildFFmpegCommand(context.Background(), cfg)

			if tt.wantFormat == "" {
				// Verify no -f flag before the URL (auto-detect)
				// The URL should be the last argument
				if cmd.Args[len(cmd.Args)-1] != tt.rtspURL {
					t.Errorf("expected URL %q as last arg, got %q", tt.rtspURL, cmd.Args[len(cmd.Args)-1])
				}
				// Verify no -f immediately before URL
				if len(cmd.Args) >= 2 && cmd.Args[len(cmd.Args)-2] == "-f" {
					t.Error("expected no -f flag for file path (auto-detect)")
				}
			} else {
				// Find the output format in args
				foundFormat := false
				for i := len(cmd.Args) - 3; i >= 0; i-- {
					if cmd.Args[i] == "-f" && i+1 < len(cmd.Args) {
						// Check if this is the output format (not input format)
						if i+2 < len(cmd.Args) && cmd.Args[i+2] == tt.rtspURL {
							if cmd.Args[i+1] == tt.wantFormat {
								foundFormat = true
							} else {
								t.Errorf("output format = %q, want %q", cmd.Args[i+1], tt.wantFormat)
							}
							break
						}
					}
				}

				if !foundFormat {
					t.Errorf("output format %q not found in command", tt.wantFormat)
				}
			}
		})
	}
}

// TestBuildFFmpegCommandReconnectFlags verifies C-2 fix: RTSP reconnect flags.
func TestBuildFFmpegCommandReconnectFlags(t *testing.T) {
	tests := []struct {
		name           string
		rtspURL        string
		outputFormat   string
		localRecordDir string
		wantReconnect  bool
	}{
		{
			name:          "rtsp auto-detect adds reconnect flags",
			rtspURL:       "rtsp://localhost:8554/test",
			wantReconnect: true,
		},
		{
			name:          "null format does not add reconnect flags",
			rtspURL:       "/dev/null",
			outputFormat:  "null",
			wantReconnect: false,
		},
		{
			name:          "file output does not add reconnect flags",
			rtspURL:       "/tmp/test.ogg",
			wantReconnect: false,
		},
		{
			name:           "tee muxer with local recording includes reconnect in tee options",
			rtspURL:        "rtsp://localhost:8554/test",
			localRecordDir: "/tmp/recordings",
			wantReconnect:  false, // reconnect is inside tee option string, not as top-level arg
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ManagerConfig{
				ALSADevice:     "hw:0,0",
				StreamName:     "test",
				SampleRate:     48000,
				Channels:       2,
				Bitrate:        "128k",
				Codec:          "opus",
				RTSPURL:        tt.rtspURL,
				OutputFormat:   tt.outputFormat,
				LocalRecordDir: tt.localRecordDir,
			}

			cmd := buildFFmpegCommand(context.Background(), cfg)

			hasReconnect := false
			for _, arg := range cmd.Args {
				if arg == "-reconnect" {
					hasReconnect = true
					break
				}
			}

			if hasReconnect != tt.wantReconnect {
				t.Errorf("-reconnect flag present = %v, want %v\nArgs: %v",
					hasReconnect, tt.wantReconnect, cmd.Args)
			}
		})
	}
}

// TestBuildFFmpegCommandTeeMuxer verifies C-1 fix: tee muxer with local recording.
func TestBuildFFmpegCommandTeeMuxer(t *testing.T) {
	t.Run("tee muxer enabled with local record dir", func(t *testing.T) {
		cfg := &ManagerConfig{
			ALSADevice:      "hw:0,0",
			StreamName:      "blue_yeti",
			SampleRate:      48000,
			Channels:        2,
			Bitrate:         "128k",
			Codec:           "opus",
			RTSPURL:         "rtsp://localhost:8554/blue_yeti",
			LocalRecordDir:  "/var/audio/recordings",
			SegmentDuration: 1800,
			SegmentFormat:   "flac",
		}

		cmd := buildFFmpegCommand(context.Background(), cfg)

		// Should use -f tee
		hasTee := false
		for i, arg := range cmd.Args {
			if arg == "-f" && i+1 < len(cmd.Args) && cmd.Args[i+1] == "tee" {
				hasTee = true
				break
			}
		}
		if !hasTee {
			t.Errorf("C-1: expected -f tee in args, got: %v", cmd.Args)
		}

		// The tee muxer requires an explicit stream map; without "-map 0:a"
		// ffmpeg aborts with "Output file does not contain any stream" and local
		// recording never starts. Verify -map 0:a precedes -f tee.
		mapIdx, teeIdx := -1, -1
		for i, arg := range cmd.Args {
			if arg == "-map" && i+1 < len(cmd.Args) && cmd.Args[i+1] == "0:a" {
				mapIdx = i
			}
			if arg == "-f" && i+1 < len(cmd.Args) && cmd.Args[i+1] == "tee" {
				teeIdx = i
			}
		}
		if mapIdx == -1 {
			t.Errorf("tee command must include -map 0:a (else ffmpeg: no stream), got: %v", cmd.Args)
		}
		if mapIdx != -1 && teeIdx != -1 && mapIdx > teeIdx {
			t.Errorf("-map 0:a must precede -f tee, got: %v", cmd.Args)
		}

		// Last arg should be the tee output string containing both RTSP and segment
		lastArg := cmd.Args[len(cmd.Args)-1]
		if !strings.Contains(lastArg, "[f=rtsp") {
			t.Errorf("C-1: tee output should contain [f=rtsp], got: %s", lastArg)
		}
		if !strings.Contains(lastArg, "f=segment") {
			t.Errorf("C-1: tee output should contain f=segment, got: %s", lastArg)
		}
		if !strings.Contains(lastArg, "segment_time=1800") {
			t.Errorf("C-1: tee output should contain segment_time=1800, got: %s", lastArg)
		}
		if !strings.Contains(lastArg, ".flac") {
			t.Errorf("C-1: tee output should contain .flac extension, got: %s", lastArg)
		}
		if !strings.Contains(lastArg, "blue_yeti_") {
			t.Errorf("C-1: tee output should contain stream name, got: %s", lastArg)
		}
		if !strings.Contains(lastArg, "reconnect=1") {
			t.Errorf("C-2: tee RTSP output should contain reconnect options, got: %s", lastArg)
		}
		// The segment slave must carry onfail=ignore so a full/broken recording
		// disk cannot abort the whole tee and drop the live RTSP stream.
		segIdx := strings.Index(lastArg, "[f=segment")
		altSegIdx := strings.Index(lastArg, ":f=segment")
		if segIdx == -1 && altSegIdx == -1 {
			t.Fatalf("tee output should contain an f=segment slave, got: %s", lastArg)
		}
		if !strings.Contains(lastArg, "onfail=ignore") {
			t.Errorf("tee segment output must contain onfail=ignore so disk failures don't kill the live stream, got: %s", lastArg)
		}
		// The RTSP slave must NOT be onfail=ignore: a genuine publish failure
		// should still exit ffmpeg so the manager restarts promptly.
		rtspSlave := lastArg
		if pipe := strings.Index(lastArg, "|"); pipe != -1 {
			rtspSlave = lastArg[:pipe]
		}
		if strings.Contains(rtspSlave, "onfail=ignore") {
			t.Errorf("tee RTSP slave must NOT be onfail=ignore (fast restart on publish failure), got: %s", rtspSlave)
		}
	})

	t.Run("tee muxer defaults", func(t *testing.T) {
		cfg := &ManagerConfig{
			ALSADevice:     "hw:0,0",
			StreamName:     "test",
			SampleRate:     48000,
			Channels:       2,
			Bitrate:        "128k",
			Codec:          "opus",
			RTSPURL:        "rtsp://localhost:8554/test",
			LocalRecordDir: "/tmp/recordings",
			// SegmentDuration and SegmentFormat unset - should use defaults
		}

		cmd := buildFFmpegCommand(context.Background(), cfg)
		lastArg := cmd.Args[len(cmd.Args)-1]

		if !strings.Contains(lastArg, "segment_time=3600") {
			t.Errorf("C-1: default segment_time should be 3600, got: %s", lastArg)
		}
		if !strings.Contains(lastArg, ".wav") {
			t.Errorf("C-1: default segment format should be wav, got: %s", lastArg)
		}
	})

	t.Run("no tee when local record dir empty", func(t *testing.T) {
		cfg := &ManagerConfig{
			ALSADevice: "hw:0,0",
			StreamName: "test",
			SampleRate: 48000,
			Channels:   2,
			Bitrate:    "128k",
			Codec:      "opus",
			RTSPURL:    "rtsp://localhost:8554/test",
			// LocalRecordDir unset
		}

		cmd := buildFFmpegCommand(context.Background(), cfg)

		for _, arg := range cmd.Args {
			if arg == "tee" {
				t.Error("C-1: should not use tee when LocalRecordDir is empty")
			}
		}
	})

	t.Run("no tee for non-rtsp output", func(t *testing.T) {
		cfg := &ManagerConfig{
			ALSADevice:     "hw:0,0",
			StreamName:     "test",
			SampleRate:     48000,
			Channels:       2,
			Bitrate:        "128k",
			Codec:          "opus",
			RTSPURL:        "/dev/null",
			OutputFormat:   "null",
			LocalRecordDir: "/tmp/recordings",
		}

		cmd := buildFFmpegCommand(context.Background(), cfg)

		for _, arg := range cmd.Args {
			if arg == "tee" {
				t.Error("C-1: should not use tee for null output format")
			}
		}
	})
}

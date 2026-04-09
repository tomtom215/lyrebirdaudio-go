package stream

import (
	"testing"
	"time"
)

// TestValidateConfig verifies configuration validation.
func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *ManagerConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			cfg: &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "opus",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: false,
		},
		{
			name: "empty device name",
			cfg: &ManagerConfig{
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "opus",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: true,
			errMsg:  "device name cannot be empty",
		},
		{
			name: "empty ALSA device",
			cfg: &ManagerConfig{
				DeviceName: "test",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "opus",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: true,
			errMsg:  "ALSA device cannot be empty",
		},
		{
			name: "invalid sample rate",
			cfg: &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: -1,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "opus",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: true,
			errMsg:  "sample rate must be positive",
		},
		{
			name: "invalid channels - zero",
			cfg: &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   0,
				Bitrate:    "128k",
				Codec:      "opus",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: true,
			errMsg:  "channels must be between 1 and 32",
		},
		{
			name: "invalid channels - too many",
			cfg: &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   33,
				Bitrate:    "128k",
				Codec:      "opus",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: true,
			errMsg:  "channels must be between 1 and 32",
		},
		{
			name: "invalid codec",
			cfg: &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "mp3",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: true,
			errMsg:  "codec must be opus or aac",
		},
		{
			name: "missing backoff",
			cfg: &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "opus",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    nil,
			},
			wantErr: true,
			errMsg:  "backoff policy cannot be nil",
		},
		{
			name: "empty stream name",
			cfg: &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "opus",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: true,
			errMsg:  "stream name cannot be empty",
		},
		{
			name: "empty bitrate",
			cfg: &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "",
				Codec:      "opus",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: true,
			errMsg:  "bitrate cannot be empty",
		},
		{
			name: "empty RTSP URL",
			cfg: &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "opus",
				RTSPURL:    "",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: true,
			errMsg:  "RTSP URL cannot be empty",
		},
		{
			name: "empty lock directory",
			cfg: &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "opus",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: true,
			errMsg:  "lock directory cannot be empty",
		},
		{
			name: "empty FFmpeg path",
			cfg: &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "opus",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: true,
			errMsg:  "FFmpeg path cannot be empty",
		},
		{
			name: "aac codec valid",
			cfg: &ManagerConfig{
				DeviceName: "test",
				ALSADevice: "hw:0,0",
				StreamName: "stream",
				SampleRate: 48000,
				Channels:   2,
				Bitrate:    "128k",
				Codec:      "aac",
				RTSPURL:    "rtsp://localhost:8554/test",
				LockDir:    "/tmp",
				FFmpegPath: "/usr/bin/ffmpeg",
				Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.cfg)

			if tt.wantErr {
				if err == nil {
					t.Error("validateConfig() expected error, got nil")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("validateConfig() error = %q, want %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateConfig() unexpected error: %v", err)
				}
			}
		})
	}
}

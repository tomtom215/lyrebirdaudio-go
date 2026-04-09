package stream

import (
	"context"
	"testing"
	"time"
)

// BenchmarkNewManager measures manager creation performance.
func BenchmarkNewManager(b *testing.B) {
	cfg := &ManagerConfig{
		DeviceName: "bench",
		ALSADevice: "hw:0,0",
		StreamName: "bench",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "opus",
		RTSPURL:    "rtsp://localhost:8554/bench",
		LockDir:    "/tmp",
		FFmpegPath: "/usr/bin/ffmpeg",
		Backoff:    NewBackoff(1*time.Second, 10*time.Second, 5),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = NewManager(cfg)
	}
}

// BenchmarkBuildFFmpegCommand measures command building performance.
func BenchmarkBuildFFmpegCommand(b *testing.B) {
	cfg := &ManagerConfig{
		ALSADevice:  "hw:0,0",
		SampleRate:  48000,
		Channels:    2,
		Bitrate:     "128k",
		Codec:       "opus",
		ThreadQueue: 8192,
		RTSPURL:     "rtsp://localhost:8554/bench",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildFFmpegCommand(context.Background(), cfg)
	}
}

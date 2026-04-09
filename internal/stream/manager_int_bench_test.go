package stream

import (
	"testing"
	"time"
)

// BenchmarkStreamManagerStart measures manager startup performance.
func BenchmarkStreamManagerStart(b *testing.B) {
	cfg := &ManagerConfig{
		DeviceName: "bench_device",
		ALSADevice: "hw:0,0",
		StreamName: "bench",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    "128k",
		Codec:      "aac",
		RTSPURL:    "/dev/null",
		LockDir:    b.TempDir(),
		FFmpegPath: "/usr/bin/ffmpeg",
		Backoff:    NewBackoff(1*time.Second, 10*time.Second, 3),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = NewManager(cfg)
	}
}

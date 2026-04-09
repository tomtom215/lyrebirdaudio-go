// SPDX-License-Identifier: MIT

package main

import (
	"testing"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
)

// TestDeviceConfigHashDifferentConfigs verifies that changing any field
// produces a different hash.
func TestDeviceConfigHashDifferentConfigs(t *testing.T) {
	base := config.DeviceConfig{
		SampleRate:  48000,
		Channels:    2,
		Bitrate:     "128k",
		Codec:       "opus",
		ThreadQueue: 512,
	}
	baseURL := "rtsp://localhost:8554/device"
	baseSC := config.StreamConfig{
		LocalRecordDir:  "/recordings",
		SegmentDuration: 3600,
		SegmentFormat:   "wav",
		StopTimeout:     5 * time.Second,
	}

	baseHash := deviceConfigHash(base, baseURL, baseSC)

	tests := []struct {
		name   string
		devCfg config.DeviceConfig
		url    string
		sc     config.StreamConfig
	}{
		{
			name:   "different thread queue",
			devCfg: func() config.DeviceConfig { d := base; d.ThreadQueue = 1024; return d }(),
			url:    baseURL,
			sc:     baseSC,
		},
		{
			name:   "different segment format",
			devCfg: base,
			url:    baseURL,
			sc: func() config.StreamConfig {
				s := baseSC
				s.SegmentFormat = "flac"
				return s
			}(),
		},
		{
			name:   "different local record dir",
			devCfg: base,
			url:    baseURL,
			sc: func() config.StreamConfig {
				s := baseSC
				s.LocalRecordDir = "/other"
				return s
			}(),
		},
		{
			name:   "different segment duration",
			devCfg: base,
			url:    baseURL,
			sc: func() config.StreamConfig {
				s := baseSC
				s.SegmentDuration = 1800
				return s
			}(),
		},
		{
			name:   "different stop timeout",
			devCfg: base,
			url:    baseURL,
			sc: func() config.StreamConfig {
				s := baseSC
				s.StopTimeout = 15 * time.Second
				return s
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := deviceConfigHash(tt.devCfg, tt.url, tt.sc)
			if h == baseHash {
				t.Errorf("hash should differ from base when %s changes", tt.name)
			}
		})
	}
}

// TestDeviceConfigHashStability verifies the same inputs produce identical hashes.
func TestDeviceConfigHashStability(t *testing.T) {
	devCfg := config.DeviceConfig{
		SampleRate:  48000,
		Channels:    2,
		Bitrate:     "128k",
		Codec:       "opus",
		ThreadQueue: 512,
	}
	url := "rtsp://localhost:8554/test"
	sc := config.StreamConfig{}

	h1 := deviceConfigHash(devCfg, url, sc)
	h2 := deviceConfigHash(devCfg, url, sc)
	if h1 != h2 {
		t.Errorf("identical inputs produced different hashes: %q vs %q", h1, h2)
	}
}

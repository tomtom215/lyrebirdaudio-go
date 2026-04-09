// SPDX-License-Identifier: MIT

package mediamtx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPath_IsAvailable_PrefersNewField verifies that the v1.17+ "available"
// field takes precedence over the deprecated "ready" field, and that the
// deprecated field is still honored when a legacy server is in use.
func TestPath_IsAvailable_PrefersNewField(t *testing.T) {
	tests := []struct {
		name      string
		path      Path
		wantAvail bool
	}{
		{"only available (v1.17+)", Path{Available: true}, true},
		{"only ready (legacy)", Path{Ready: true}, true},
		{"neither set", Path{}, false},
		{"available false with ready true (legacy fallback)", Path{Available: false, Ready: true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.path.IsAvailable(); got != tt.wantAvail {
				t.Errorf("IsAvailable() = %v, want %v", got, tt.wantAvail)
			}
		})
	}
}

// TestPath_ByteHelpers verifies the v1.17+ byte counters take precedence
// over the deprecated ones and that fallback works for legacy servers.
func TestPath_ByteHelpers(t *testing.T) {
	tests := []struct {
		name        string
		path        Path
		wantIn      int64
		wantOut     int64
		wantReadyAt string
	}{
		{
			name:        "v1.17+ fields populated",
			path:        Path{InboundBytes: 42, OutboundBytes: 17, AvailableTime: "2026-04-09T00:00:00Z"},
			wantIn:      42,
			wantOut:     17,
			wantReadyAt: "2026-04-09T00:00:00Z",
		},
		{
			name:        "legacy fields populated",
			path:        Path{BytesReceived: 99, BytesSent: 8, ReadyTime: "2025-01-01T00:00:00Z"},
			wantIn:      99,
			wantOut:     8,
			wantReadyAt: "2025-01-01T00:00:00Z",
		},
		{
			name:        "v1.17+ available preferred over legacy ready time",
			path:        Path{AvailableTime: "2026-04-09T00:00:00Z", ReadyTime: "2025-01-01T00:00:00Z"},
			wantReadyAt: "2026-04-09T00:00:00Z",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.path.TotalInboundBytes(); got != tt.wantIn {
				t.Errorf("TotalInboundBytes() = %d, want %d", got, tt.wantIn)
			}
			if got := tt.path.TotalOutboundBytes(); got != tt.wantOut {
				t.Errorf("TotalOutboundBytes() = %d, want %d", got, tt.wantOut)
			}
			if got := tt.path.AvailableAtTime(); got != tt.wantReadyAt {
				t.Errorf("AvailableAtTime() = %q, want %q", got, tt.wantReadyAt)
			}
		})
	}
}

// TestIsStreamHealthy_V117Payload verifies that IsStreamHealthy accepts a
// payload containing only the new v1.17+ field names.
func TestIsStreamHealthy_V117Payload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Emit only the new field names - no "ready"/"bytesReceived".
		_, _ = w.Write([]byte(`{
			"name": "stream1",
			"confName": "all_others",
			"available": true,
			"availableTime": "2026-04-09T00:00:00Z",
			"online": true,
			"onlineTime": "2026-04-09T00:00:00Z",
			"inboundBytes": 12345,
			"outboundBytes": 678,
			"inboundFramesInError": 0,
			"tracks2": [
				{"codec": "Opus", "codecProps": {"channelCount": 2}}
			]
		}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	healthy, err := client.IsStreamHealthy(context.Background(), "stream1")
	if err != nil {
		t.Fatalf("IsStreamHealthy() error: %v", err)
	}
	if !healthy {
		t.Error("IsStreamHealthy() = false, want true for v1.17+ payload")
	}
}

// TestGetStreamStats_V117Tracks2 verifies audio track extraction works with
// the new "tracks2" field when the server omits the deprecated "tracks" field.
func TestGetStreamStats_V117Tracks2(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := Path{
			Name:          "test_stream",
			ConfName:      "all_others",
			Available:     true,
			AvailableTime: "2026-04-09T00:00:00Z",
			Online:        true,
			InboundBytes:  10000,
			OutboundBytes: 5000,
			Tracks2: []PathTrack{
				{
					Codec: "Opus",
					CodecProps: &PathCodecProps{
						SampleRate:   48000,
						ChannelCount: 2,
					},
				},
			},
			Readers: []Reader{{Type: "rtspSession", ID: "reader1"}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	stats, err := client.GetStreamStats(context.Background(), "test_stream")
	if err != nil {
		t.Fatalf("GetStreamStats() error: %v", err)
	}

	if stats.AudioCodec != "Opus" {
		t.Errorf("AudioCodec = %q, want %q", stats.AudioCodec, "Opus")
	}
	if stats.SampleRate != 48000 {
		t.Errorf("SampleRate = %d, want 48000", stats.SampleRate)
	}
	if stats.Channels != 2 {
		t.Errorf("Channels = %d, want 2", stats.Channels)
	}
	if stats.BytesReceived != 10000 {
		t.Errorf("BytesReceived = %d, want 10000", stats.BytesReceived)
	}
	if stats.BytesSent != 5000 {
		t.Errorf("BytesSent = %d, want 5000", stats.BytesSent)
	}
	if !stats.Ready {
		t.Error("Ready = false, want true")
	}
	if stats.ReadyTime.IsZero() {
		t.Error("ReadyTime should be parsed from AvailableTime")
	}
}

// TestGetStreamStats_NonAudioTracks2Ignored verifies that video-only tracks2
// entries do not populate audio stats fields.
func TestGetStreamStats_NonAudioTracks2Ignored(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := Path{
			Name:         "video_stream",
			Available:    true,
			InboundBytes: 1,
			Tracks2: []PathTrack{
				{Codec: "H264", CodecProps: &PathCodecProps{Width: 1920, Height: 1080}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	stats, err := client.GetStreamStats(context.Background(), "video_stream")
	if err != nil {
		t.Fatalf("GetStreamStats() error: %v", err)
	}
	if stats.AudioCodec != "" {
		t.Errorf("AudioCodec = %q, want empty for video-only stream", stats.AudioCodec)
	}
	if stats.SampleRate != 0 {
		t.Errorf("SampleRate = %d, want 0", stats.SampleRate)
	}
}

// TestHealthCheck_V117Payload verifies HealthCheck aggregates correctly when
// servers emit only the v1.17+ fields.
func TestHealthCheck_V117Payload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"pageCount": 1,
			"itemCount": 2,
			"items": [
				{"name": "a", "available": true, "inboundBytes": 500},
				{"name": "b", "available": false, "inboundBytes": 0}
			]
		}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	status, err := client.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error: %v", err)
	}
	if status.TotalStreams != 2 {
		t.Errorf("TotalStreams = %d, want 2", status.TotalStreams)
	}
	if status.HealthyStreams != 1 {
		t.Errorf("HealthyStreams = %d, want 1", status.HealthyStreams)
	}
	if status.Healthy {
		t.Error("Healthy should be false when only 1 of 2 streams is available")
	}
}

// TestIsAudioCodec covers the codec classifier used when decoding tracks2.
func TestIsAudioCodec(t *testing.T) {
	audio := []string{"Opus", "Vorbis", "MPEG-4 Audio", "MPEG-4 Audio LATM",
		"MPEG-1/2 Audio", "AC3", "Speex", "G726", "G722", "G711", "LPCM"}
	for _, c := range audio {
		if !isAudioCodec(c) {
			t.Errorf("isAudioCodec(%q) = false, want true", c)
		}
	}
	video := []string{"AV1", "VP9", "H265", "H264", "M-JPEG", "Generic", ""}
	for _, c := range video {
		if isAudioCodec(c) {
			t.Errorf("isAudioCodec(%q) = true, want false", c)
		}
	}
}

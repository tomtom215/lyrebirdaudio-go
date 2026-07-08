// SPDX-License-Identifier: MIT

package mediamtx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// These fixtures are byte-for-byte responses captured from a real
// MediaMTX v1.19.2 server (started with api: yes) while an ffmpeg
// synthetic-audio publisher was connected. They are the authoritative
// wire format and guard against regressions like the one where "tracks"
// was modeled as []Track (objects) instead of []string, which made
// json.Decode fail on every path that had a track — silently disabling
// stall detection and auto-restart.
const (
	realPathGetOpus = `{
  "name": "testmic",
  "confName": "all_others",
  "ready": true,
  "readyTime": "2026-07-07T23:28:18.996018783Z",
  "available": true,
  "availableTime": "2026-07-07T23:28:18.996018783Z",
  "online": true,
  "onlineTime": "2026-07-07T23:28:18.996018988Z",
  "source": {"type": "rtspSession", "id": "562fbd8a-212c-4cb3-99ff-07b4ec47f337"},
  "tracks": ["Opus"],
  "tracks2": [{"codec": "Opus", "codecProps": {"channelCount": 2}}],
  "readers": [],
  "inboundBytes": 60152,
  "outboundBytes": 0,
  "inboundFramesInError": 0,
  "bytesReceived": 60152,
  "bytesSent": 0
}`

	realPathsListOpus = `{
  "itemCount": 1,
  "pageCount": 1,
  "items": [` + realPathGetOpus + `]
}`
)

// TestDecodeRealMediaMTXPath is the regression guard for the "tracks" decode
// bug. With the old []Track model, json.Decode returns an UnmarshalTypeError
// for "tracks":["Opus"] and GetPath fails; this test asserts a clean decode
// and correct extraction from the real wire format.
func TestDecodeRealMediaMTXPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/paths/get/testmic" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(realPathGetOpus))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	path, err := client.GetPath(context.Background(), "testmic")
	if err != nil {
		t.Fatalf("GetPath() on real MediaMTX JSON failed to decode: %v", err)
	}

	if len(path.Tracks) != 1 || path.Tracks[0] != "Opus" {
		t.Errorf("path.Tracks = %v, want [Opus]", path.Tracks)
	}
	if !path.IsAvailable() {
		t.Error("path.IsAvailable() = false, want true (available:true in fixture)")
	}
	if got := path.TotalInboundBytes(); got != 60152 {
		t.Errorf("TotalInboundBytes() = %d, want 60152", got)
	}
	if len(path.Tracks2) != 1 || path.Tracks2[0].Codec != "Opus" {
		t.Errorf("path.Tracks2 = %+v, want one Opus track", path.Tracks2)
	}
	if path.Tracks2[0].CodecProps == nil || path.Tracks2[0].CodecProps.ChannelCount != 2 {
		t.Errorf("Opus codecProps channelCount = %+v, want 2", path.Tracks2[0].CodecProps)
	}
}

func TestIsStreamHealthyRealJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(realPathGetOpus))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	healthy, err := client.IsStreamHealthy(context.Background(), "testmic")
	if err != nil {
		t.Fatalf("IsStreamHealthy() error: %v", err)
	}
	if !healthy {
		t.Error("IsStreamHealthy() = false, want true for an available stream with inbound bytes")
	}
}

func TestGetStreamStatsRealOpusJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(realPathGetOpus))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	stats, err := client.GetStreamStats(context.Background(), "testmic")
	if err != nil {
		t.Fatalf("GetStreamStats() error: %v", err)
	}
	if stats.AudioCodec != "Opus" {
		t.Errorf("AudioCodec = %q, want Opus", stats.AudioCodec)
	}
	if stats.Channels != 2 {
		t.Errorf("Channels = %d, want 2", stats.Channels)
	}
	// Opus does not report a sample rate through the API (always 48kHz), so 0
	// is expected here — this documents the real behavior.
	if stats.SampleRate != 0 {
		t.Errorf("SampleRate = %d, want 0 (Opus reports no sampleRate)", stats.SampleRate)
	}
}

func TestListPathsRealJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/paths/list" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(realPathsListOpus))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	paths, err := client.ListPaths(context.Background())
	if err != nil {
		t.Fatalf("ListPaths() error: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("ListPaths() returned %d paths, want 1", len(paths))
	}
	if len(paths[0].Tracks) != 1 || paths[0].Tracks[0] != "Opus" {
		t.Errorf("paths[0].Tracks = %v, want [Opus]", paths[0].Tracks)
	}
}

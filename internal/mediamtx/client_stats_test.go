package mediamtx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetStreamStats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := Path{
			Name:          "test_stream",
			Ready:         true,
			ReadyTime:     "2025-12-14T10:00:00Z",
			BytesReceived: 10000,
			BytesSent:     5000,
			Tracks: []Track{
				{Type: "audio", Codec: "opus", SampleRate: 48000, Channels: 2},
			},
			Readers: []Reader{
				{Type: "rtsp", ID: "reader1"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	stats, err := client.GetStreamStats(context.Background(), "test_stream")
	if err != nil {
		t.Fatalf("GetStreamStats() error: %v", err)
	}

	if stats.Name != "test_stream" {
		t.Errorf("stats.Name = %q, want %q", stats.Name, "test_stream")
	}
	if stats.ReaderCount != 1 {
		t.Errorf("stats.ReaderCount = %d, want 1", stats.ReaderCount)
	}
	if stats.AudioCodec != "opus" {
		t.Errorf("stats.AudioCodec = %q, want %q", stats.AudioCodec, "opus")
	}
	if stats.SampleRate != 48000 {
		t.Errorf("stats.SampleRate = %d, want 48000", stats.SampleRate)
	}
}

func TestGetStreamStatsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.GetStreamStats(context.Background(), "nonexistent")
	if err == nil {
		t.Error("GetStreamStats() expected error for 404 response")
	}
}

func TestGetStreamStatsNoTracks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := Path{
			Name:          "test_stream",
			Ready:         true,
			BytesReceived: 1000,
			Tracks:        []Track{}, // No tracks
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	stats, err := client.GetStreamStats(context.Background(), "test_stream")
	if err != nil {
		t.Fatalf("GetStreamStats() error: %v", err)
	}

	if stats.AudioCodec != "" {
		t.Errorf("stats.AudioCodec = %q, want empty", stats.AudioCodec)
	}
	if stats.SampleRate != 0 {
		t.Errorf("stats.SampleRate = %d, want 0", stats.SampleRate)
	}
}

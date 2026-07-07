package mediamtx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetStreamStats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Real MediaMTX v1.19.2 wire format for an AAC stream (captured live):
		// codec label + structured tracks2 carrying sampleRate/channelCount.
		_, _ = w.Write([]byte(`{
			"name": "test_stream",
			"ready": true,
			"available": true,
			"availableTime": "2025-12-14T10:00:00Z",
			"inboundBytes": 10000,
			"outboundBytes": 5000,
			"bytesReceived": 10000,
			"bytesSent": 5000,
			"tracks": ["MPEG-4 Audio"],
			"tracks2": [{"codec": "MPEG-4 Audio", "codecProps": {"sampleRate": 44100, "channelCount": 1}}],
			"readers": [{"type": "rtspSession", "id": "reader1"}]
		}`))
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
	if stats.AudioCodec != "MPEG-4 Audio" {
		t.Errorf("stats.AudioCodec = %q, want %q", stats.AudioCodec, "MPEG-4 Audio")
	}
	if stats.SampleRate != 44100 {
		t.Errorf("stats.SampleRate = %d, want 44100", stats.SampleRate)
	}
	if stats.Channels != 1 {
		t.Errorf("stats.Channels = %d, want 1", stats.Channels)
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
		_, _ = w.Write([]byte(`{
			"name": "test_stream",
			"ready": true,
			"available": true,
			"inboundBytes": 1000,
			"bytesReceived": 1000,
			"tracks": [],
			"tracks2": []
		}`))
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

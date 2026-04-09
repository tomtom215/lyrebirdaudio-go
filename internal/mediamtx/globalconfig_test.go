// SPDX-License-Identifier: MIT

package mediamtx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGetGlobalConfig_HappyPath(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		// Response contains a few known fields plus some we don't model,
		// to verify unknown-field tolerance.
		_, _ = w.Write([]byte(`{
			"logLevel": "info",
			"logDestinations": ["stdout"],
			"logFile": "mediamtx.log",
			"readTimeout": "10s",
			"writeTimeout": "10s",
			"writeQueueSize": 512,
			"udpMaxPayloadSize": 1452,
			"authMethod": "internal",
			"api": true,
			"apiAddress": ":9997",
			"apiEncryption": false,
			"metrics": false,
			"metricsAddress": ":9998",
			"pprof": false,
			"pprofAddress": ":9999",
			"playback": false,
			"playbackAddress": ":9996",
			"rtsp": true,
			"rtspAddress": ":8554",
			"rtspsAddress": ":8322",
			"rtspTransports": ["udp", "multicast", "tcp"],
			"rtspEncryption": "no",
			"rtspAuthMethods": ["basic"],
			"rtmp": true,
			"rtmpAddress": ":1935",
			"hls": true,
			"hlsAddress": ":8888",
			"webrtc": true,
			"webrtcAddress": ":8889",
			"srt": true,
			"srtAddress": ":8890",
			"unknownFutureField": "should be ignored silently",
			"anotherUnknown": {"nested": true}
		}`))
	}))
	defer server.Close()

	cfg, err := NewClient(server.URL).GetGlobalConfig(context.Background())
	if err != nil {
		t.Fatalf("GetGlobalConfig() error: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/v3/config/global/get" {
		t.Errorf("path = %q, want /v3/config/global/get", gotPath)
	}

	// Spot-check the operational fields most likely to be used by diagnose.
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
	if len(cfg.LogDestinations) != 1 || cfg.LogDestinations[0] != "stdout" {
		t.Errorf("LogDestinations = %v, want [stdout]", cfg.LogDestinations)
	}
	if cfg.ReadTimeout != "10s" {
		t.Errorf("ReadTimeout = %q, want 10s", cfg.ReadTimeout)
	}
	if cfg.WriteQueueSize != 512 {
		t.Errorf("WriteQueueSize = %d, want 512", cfg.WriteQueueSize)
	}
	if cfg.UDPMaxPayloadSize != 1452 {
		t.Errorf("UDPMaxPayloadSize = %d, want 1452", cfg.UDPMaxPayloadSize)
	}
	if cfg.AuthMethod != "internal" {
		t.Errorf("AuthMethod = %q, want internal", cfg.AuthMethod)
	}
	if !cfg.API || cfg.APIAddress != ":9997" {
		t.Errorf("API control: got api=%v addr=%q, want true :9997", cfg.API, cfg.APIAddress)
	}
	if cfg.APIEncryption {
		t.Error("APIEncryption should be false")
	}
	if !cfg.RTSP || cfg.RTSPAddress != ":8554" {
		t.Errorf("RTSP: got rtsp=%v addr=%q, want true :8554", cfg.RTSP, cfg.RTSPAddress)
	}
	if cfg.RTSPEncryption != "no" {
		t.Errorf("RTSPEncryption = %q, want no", cfg.RTSPEncryption)
	}
	wantTransports := []string{"udp", "multicast", "tcp"}
	if !equalStrSlice(cfg.RTSPTransports, wantTransports) {
		t.Errorf("RTSPTransports = %v, want %v", cfg.RTSPTransports, wantTransports)
	}
	if !equalStrSlice(cfg.RTSPAuthMethods, []string{"basic"}) {
		t.Errorf("RTSPAuthMethods = %v, want [basic]", cfg.RTSPAuthMethods)
	}
	if !cfg.RTMP || !cfg.HLS || !cfg.WebRTC || !cfg.SRT {
		t.Errorf("protocol toggles: rtmp=%v hls=%v webrtc=%v srt=%v", cfg.RTMP, cfg.HLS, cfg.WebRTC, cfg.SRT)
	}
}

func TestGetGlobalConfig_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"status":"error","error":"boom"}`))
	}))
	defer server.Close()

	_, err := NewClient(server.URL).GetGlobalConfig(context.Background())
	if err == nil {
		t.Fatal("GetGlobalConfig() expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status 500: %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error should contain server body: %v", err)
	}
}

func TestGetGlobalConfig_ServerErrorUnreadableBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusBadGateway)
		if hj, ok := w.(http.Hijacker); ok {
			conn, _, err := hj.Hijack()
			if err == nil {
				_ = conn.Close()
			}
		}
	}))
	defer server.Close()

	_, err := NewClient(server.URL).GetGlobalConfig(context.Background())
	if err == nil {
		t.Fatal("expected error for truncated body")
	}
}

func TestGetGlobalConfig_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	_, err := NewClient(server.URL).GetGlobalConfig(context.Background())
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestGetGlobalConfig_NetworkError(t *testing.T) {
	_, err := NewClient("http://127.0.0.1:1").GetGlobalConfig(context.Background())
	if err == nil {
		t.Error("expected error for network failure")
	}
}

func TestGetGlobalConfig_ContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := NewClient(server.URL).GetGlobalConfig(ctx)
	if err == nil {
		t.Error("expected error for ctx timeout")
	}
}

// equalStrSlice reports whether two string slices are element-wise equal.
// Used only by these tests; kept package-private and local.
func equalStrSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

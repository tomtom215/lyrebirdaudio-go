// SPDX-License-Identifier: MIT

package mediamtx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// GlobalConfig represents the live global configuration of a MediaMTX server
// as reported by GET /v3/config/global/get. MediaMTX's GlobalConf schema has
// well over 100 fields spanning every server subsystem; only the subset that
// lyrebird's diagnose/status code needs to sanity-check is decoded here.
// Unknown fields are ignored by encoding/json, so decoding will not fail if
// MediaMTX adds new fields.
//
// Deprecated fields (ready/protocols/encryption/…) are intentionally omitted;
// rely on their non-deprecated replacements listed below.
//
// If you need a field not present here, add it — the server emits all GlobalConf
// fields verbatim, so adding a struct tag is sufficient.
type GlobalConfig struct {
	// General.
	LogLevel          string   `json:"logLevel"`
	LogDestinations   []string `json:"logDestinations"`
	LogFile           string   `json:"logFile"`
	ReadTimeout       string   `json:"readTimeout"`
	WriteTimeout      string   `json:"writeTimeout"`
	WriteQueueSize    int64    `json:"writeQueueSize"`
	UDPMaxPayloadSize int64    `json:"udpMaxPayloadSize"`

	// Authentication.
	AuthMethod string `json:"authMethod"`

	// Control API.
	API           bool   `json:"api"`
	APIAddress    string `json:"apiAddress"`
	APIEncryption bool   `json:"apiEncryption"`

	// Metrics / pprof.
	Metrics        bool   `json:"metrics"`
	MetricsAddress string `json:"metricsAddress"`
	Pprof          bool   `json:"pprof"`
	PprofAddress   string `json:"pprofAddress"`

	// Playback.
	Playback        bool   `json:"playback"`
	PlaybackAddress string `json:"playbackAddress"`

	// RTSP server (the one lyrebird actually uses).
	RTSP            bool     `json:"rtsp"`
	RTSPAddress     string   `json:"rtspAddress"`
	RTSPSAddress    string   `json:"rtspsAddress"`
	RTSPTransports  []string `json:"rtspTransports"`
	RTSPEncryption  string   `json:"rtspEncryption"`
	RTSPAuthMethods []string `json:"rtspAuthMethods"`

	// Other protocol servers — exposed so operators can confirm unused
	// servers are disabled in production deployments.
	RTMP          bool   `json:"rtmp"`
	RTMPAddress   string `json:"rtmpAddress"`
	HLS           bool   `json:"hls"`
	HLSAddress    string `json:"hlsAddress"`
	WebRTC        bool   `json:"webrtc"`
	WebRTCAddress string `json:"webrtcAddress"`
	SRT           bool   `json:"srt"`
	SRTAddress    string `json:"srtAddress"`
}

// GetGlobalConfig fetches the live global configuration from MediaMTX.
//
// API endpoint: GET /v3/config/global/get
//
// This is a read-only operation: no request body is sent and the server's
// configuration is not modified. To change the configuration, edit
// mediamtx.yml and rely on the server's built-in hot-reload — lyrebird does
// not expose the /v3/config/global/patch endpoint in order to avoid a
// second source of truth racing with the on-disk config file.
//
// Unknown fields in the response are silently ignored.
func (c *Client) GetGlobalConfig(ctx context.Context) (*GlobalConfig, error) {
	reqURL := c.baseURL + "/v3/config/global/get"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req) // #nosec G107 -- URL is derived from client baseURL, not user HTTP input
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		if readErr != nil {
			return nil, fmt.Errorf("get global config: API returned status %d (failed to read body: %w)", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("get global config: API returned status %d: %s", resp.StatusCode, string(body))
	}

	var cfg GlobalConfig
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &cfg, nil
}

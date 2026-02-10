// SPDX-License-Identifier: MIT

// Package mediamtx provides a client for the MediaMTX REST API.
//
// This enables health checking, stream monitoring, and runtime management
// of the RTSP server without modifying configuration files.
//
// Reference: https://github.com/bluenviron/mediamtx
package mediamtx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// DefaultAPIURL is the default MediaMTX API endpoint.
	DefaultAPIURL = "http://localhost:9997"

	// DefaultTimeout is the default HTTP request timeout.
	DefaultTimeout = 5 * time.Second
)

// Client provides methods for interacting with the MediaMTX REST API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// Path represents a stream path in MediaMTX.
type Path struct {
	Name          string   `json:"name"`
	Source        *Source  `json:"source,omitempty"`
	Ready         bool     `json:"ready"`
	ReadyTime     string   `json:"readyTime,omitempty"`
	Tracks        []Track  `json:"tracks,omitempty"`
	BytesReceived int64    `json:"bytesReceived"`
	BytesSent     int64    `json:"bytesSent"`
	Readers       []Reader `json:"readers,omitempty"`
}

// Source describes the source of a stream.
type Source struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
}

// TrackType represents the type of media track.
type TrackType string

const (
	// TrackTypeAudio represents an audio track.
	TrackTypeAudio TrackType = "audio"
	// TrackTypeVideo represents a video track.
	TrackTypeVideo TrackType = "video"
)

// Track represents a media track in a stream.
type Track struct {
	Type       string `json:"type"`       // "audio" or "video" (use TrackTypeAudio/TrackTypeVideo constants)
	Codec      string `json:"codec"`      // e.g., "opus", "aac"
	ClockRate  int    `json:"clockRate"`  // e.g., 48000
	Channels   int    `json:"channels"`   // Audio channels
	BitDepth   int    `json:"bitDepth"`   // Audio bit depth
	SampleRate int    `json:"sampleRate"` // Audio sample rate
}

// Reader represents a client reading from a stream.
type Reader struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	BytesSent int64  `json:"bytesSent"`
}

// PathList is the response from the list paths endpoint.
type PathList struct {
	PageCount int    `json:"pageCount"`
	ItemCount int    `json:"itemCount"`
	Items     []Path `json:"items"`
}

// ServerInfo contains MediaMTX server information.
type ServerInfo struct {
	Version string `json:"version"`
}

// ClientOption is a functional option for configuring the client.
type ClientOption func(*Client)

// WithTimeout sets the HTTP client timeout.
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = timeout
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// NewClient creates a new MediaMTX API client.
//
// Parameters:
//   - baseURL: Base URL of the MediaMTX API (e.g., "http://localhost:9997")
//   - opts: Optional configuration options
//
// Returns:
//   - *Client: Configured API client
//
// Example:
//
//	client := NewClient("http://localhost:9997")
//	paths, err := client.ListPaths(ctx)
func NewClient(baseURL string, opts ...ClientOption) *Client {
	c := &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// ListPaths returns all configured paths.
//
// API endpoint: GET /v3/paths/list
//
// Parameters:
//   - ctx: Context for cancellation
//
// Returns:
//   - []Path: List of all paths
//   - error: if request fails
func (c *Client) ListPaths(ctx context.Context) ([]Path, error) {
	url := fmt.Sprintf("%s/v3/paths/list", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("API returned status %d (failed to read body: %v)", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var pathList PathList
	if err := json.NewDecoder(resp.Body).Decode(&pathList); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return pathList.Items, nil
}

// GetPath returns information about a specific path.
//
// API endpoint: GET /v3/paths/get/{name}
//
// Parameters:
//   - ctx: Context for cancellation
//   - name: Path name (e.g., "blue_yeti")
//
// Returns:
//   - *Path: Path information
//   - error: if path not found or request fails
func (c *Client) GetPath(ctx context.Context, name string) (*Path, error) {
	url := fmt.Sprintf("%s/v3/paths/get/%s", c.baseURL, name)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("path %q not found", name)
	}

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("API returned status %d (failed to read body: %v)", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var path Path
	if err := json.NewDecoder(resp.Body).Decode(&path); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &path, nil
}

// IsStreamHealthy checks if a stream is active and receiving data.
//
// A stream is considered healthy if:
//  1. The path exists
//  2. ready == true
//  3. BytesReceived > 0 (data is flowing)
//
// Parameters:
//   - ctx: Context for cancellation
//   - name: Stream/path name
//
// Returns:
//   - bool: true if stream is healthy
//   - error: if API request fails
//
// Reference: mediamtx-stream-manager.sh validate_stream_api()
func (c *Client) IsStreamHealthy(ctx context.Context, name string) (bool, error) {
	path, err := c.GetPath(ctx, name)
	if err != nil {
		return false, err
	}

	// Check readiness and data flow
	return path.Ready && path.BytesReceived > 0, nil
}

// WaitForStream waits for a stream to become ready.
//
// Polls the API until the stream is ready or timeout is reached.
//
// Parameters:
//   - ctx: Context for cancellation
//   - name: Stream/path name
//   - timeout: Maximum time to wait
//   - pollInterval: Time between polls (default 1s)
//
// Returns:
//   - error: if timeout reached or API error
func (c *Client) WaitForStream(ctx context.Context, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	pollInterval := time.Second

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for stream %q to become ready", name)
		}

		healthy, err := c.IsStreamHealthy(ctx, name)
		if err == nil && healthy {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
			// Continue polling
		}
	}
}

// Ping checks if the MediaMTX API is reachable.
//
// Parameters:
//   - ctx: Context for cancellation
//
// Returns:
//   - error: if API is not reachable
func (c *Client) Ping(ctx context.Context) error {
	url := fmt.Sprintf("%s/v3/paths/list", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("MediaMTX API not reachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("MediaMTX API returned status %d (failed to read body: %v)", resp.StatusCode, readErr)
		}
		return fmt.Errorf("MediaMTX API returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetStreamStats returns statistics for a stream.
//
// Parameters:
//   - ctx: Context for cancellation
//   - name: Stream/path name
//
// Returns:
//   - StreamStats: Statistics about the stream
//   - error: if request fails
func (c *Client) GetStreamStats(ctx context.Context, name string) (*StreamStats, error) {
	path, err := c.GetPath(ctx, name)
	if err != nil {
		return nil, err
	}

	stats := &StreamStats{
		Name:          path.Name,
		Ready:         path.Ready,
		BytesReceived: path.BytesReceived,
		BytesSent:     path.BytesSent,
		ReaderCount:   len(path.Readers),
	}

	// Parse ready time if available
	if path.ReadyTime != "" {
		if t, err := time.Parse(time.RFC3339, path.ReadyTime); err == nil {
			stats.ReadyTime = t
			stats.Uptime = time.Since(t)
		}
	}

	// Extract track info
	for _, track := range path.Tracks {
		if track.Type == string(TrackTypeAudio) {
			stats.AudioCodec = track.Codec
			stats.SampleRate = track.SampleRate
			stats.Channels = track.Channels
		}
	}

	return stats, nil
}

// StreamStats contains statistics about a stream.
type StreamStats struct {
	Name          string        // Stream name
	Ready         bool          // Is stream ready
	ReadyTime     time.Time     // When stream became ready
	Uptime        time.Duration // How long stream has been ready
	BytesReceived int64         // Total bytes received
	BytesSent     int64         // Total bytes sent to readers
	ReaderCount   int           // Number of active readers
	AudioCodec    string        // Audio codec (e.g., "opus")
	SampleRate    int           // Audio sample rate
	Channels      int           // Audio channels
}

// HealthCheck performs a comprehensive health check of MediaMTX.
//
// Returns:
//   - *HealthStatus: Overall health status
//   - error: if health check fails
func (c *Client) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	status := &HealthStatus{
		Timestamp: time.Now(),
	}

	// Check API reachability
	if err := c.Ping(ctx); err != nil {
		status.APIReachable = false
		status.Error = err.Error()
		return status, nil
	}
	status.APIReachable = true

	// Get all paths
	paths, err := c.ListPaths(ctx)
	if err != nil {
		status.Error = err.Error()
		return status, nil
	}

	status.TotalStreams = len(paths)

	// Count healthy streams
	for _, path := range paths {
		if path.Ready && path.BytesReceived > 0 {
			status.HealthyStreams++
		}
	}

	status.Healthy = status.APIReachable && status.HealthyStreams == status.TotalStreams

	return status, nil
}

// HealthStatus contains the health status of MediaMTX.
type HealthStatus struct {
	Timestamp      time.Time // When check was performed
	Healthy        bool      // Overall health
	APIReachable   bool      // Is API reachable
	TotalStreams   int       // Total configured streams
	HealthyStreams int       // Number of healthy streams
	Error          string    // Error message if unhealthy
}

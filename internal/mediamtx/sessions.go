// SPDX-License-Identifier: MIT

package mediamtx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
)

// RTSPSession represents one active RTSP session as reported by MediaMTX
// v1.17+ at GET /v3/rtspsessions/list and /v3/rtspsessions/get/{id}.
//
// Only the operationally useful fields are decoded here. Deprecated byte
// and packet counter duplicates (bytesReceived, rtpPacketsReceived, …) are
// intentionally omitted — use the modern Inbound*/Outbound* counters.
//
// The server also returns per-session RTCP/RTP packet counts that are not
// currently used by lyrebird; add them only when a concrete consumer needs
// them, to keep this surface minimal.
type RTSPSession struct {
	// ID is the session's UUID. Pass this value to KickRTSPSession.
	ID string `json:"id"`

	// Created is the RFC3339 timestamp at which the session was opened.
	Created string `json:"created"`

	// RemoteAddr is the client address in host:port form.
	RemoteAddr string `json:"remoteAddr"`

	// State is one of "idle", "read", "publish".
	State string `json:"state"`

	// Path is the MediaMTX path name the session is attached to.
	Path string `json:"path"`

	// Query is the URL query string supplied by the client, if any.
	Query string `json:"query"`

	// User is the authenticated user, or empty if none.
	User string `json:"user"`

	// Transport describes the RTP transport (e.g. "UDP", "TCP", "MULTICAST").
	// The field is nullable in the API; when the session is idle the server
	// omits it, and it is decoded here as an empty string.
	Transport string `json:"transport,omitempty"`

	// Profile is the RTSP profile name; nullable, empty when absent.
	Profile string `json:"profile,omitempty"`

	// InboundBytes is the total number of bytes received from the client.
	InboundBytes uint64 `json:"inboundBytes"`

	// InboundRTPPackets is the total number of RTP packets received.
	InboundRTPPackets uint64 `json:"inboundRTPPackets"`

	// InboundRTPPacketsLost is the total number of RTP packets the server
	// detected as lost on the inbound side.
	InboundRTPPacketsLost uint64 `json:"inboundRTPPacketsLost"`

	// OutboundBytes is the total number of bytes sent to the client.
	OutboundBytes uint64 `json:"outboundBytes"`

	// OutboundRTPPackets is the total number of RTP packets sent.
	OutboundRTPPackets uint64 `json:"outboundRTPPackets"`

	// OutboundRTPPacketsReportedLost is the number of outbound RTP packets
	// the client reported as lost via RTCP feedback.
	OutboundRTPPacketsReportedLost uint64 `json:"outboundRTPPacketsReportedLost"`
}

// RTSPSessionList is the server response envelope for session-listing
// endpoints. It is exported so that callers that need to surface total counts
// (for pagination or UI) can do so.
type RTSPSessionList struct {
	PageCount int64         `json:"pageCount"`
	ItemCount int64         `json:"itemCount"`
	Items     []RTSPSession `json:"items"`
}

// ListRTSPSessions returns all active RTSP sessions, transparently paginating
// across every page the server reports. It fetches the first page using the
// server's default itemsPerPage, reads the PageCount from that response, then
// walks pages 1..PageCount-1, accumulating every session into a single slice.
//
// This matters for correctness on busy servers: MediaMTX caps a page at its
// default itemsPerPage (100 as of v1.17.1), so a deployment with more active
// sessions than the page size would otherwise silently drop the later pages —
// hiding reader sessions from stall-recovery and under-reporting status.
//
// Callers that need per-page control, or the raw ItemCount/PageCount totals,
// should use ListRTSPSessionsPage instead.
//
// The number of requests is bounded by the PageCount reported in the first
// response, so a misbehaving server cannot induce an unbounded fetch loop.
// Pagination stops early if ctx is canceled between page fetches, returning
// the context error. A degenerate PageCount of 0 or 1 yields no extra
// requests and returns the first page as-is.
//
// API endpoint: GET /v3/rtspsessions/list
//
// The returned slice is always non-nil when err is nil; it may be empty.
func (c *Client) ListRTSPSessions(ctx context.Context) ([]RTSPSession, error) {
	// Fetch the first page using the server's default itemsPerPage (negative
	// arguments omit the query params). This keeps the common single-page case
	// byte-for-byte identical to a plain unpaginated request.
	first, err := c.ListRTSPSessionsPage(ctx, -1, -1)
	if err != nil {
		return nil, err
	}

	// ListRTSPSessionsPage guarantees a non-nil Items slice, so all starts
	// non-nil and the non-nil-on-success contract holds through accumulation.
	all := first.Items

	// PageCount is captured once from the first response and used as a hard
	// upper bound, so the number of iterations is fixed regardless of what
	// later responses claim.
	for page := 1; int64(page) < first.PageCount; page++ {
		// Honor cancellation between pages rather than issuing every remaining
		// request after the caller has given up.
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		next, err := c.ListRTSPSessionsPage(ctx, page, -1)
		if err != nil {
			return nil, fmt.Errorf("fetching rtsp sessions page %d: %w", page, err)
		}
		all = append(all, next.Items...)
	}

	return all, nil
}

// ListRTSPSessionsPage fetches one page of active RTSP sessions. Pass a
// negative value for page or itemsPerPage to omit that query parameter and
// let the server use its default (page 0, 100 items per page as of v1.17.1).
//
// API endpoint: GET /v3/rtspsessions/list
func (c *Client) ListRTSPSessionsPage(ctx context.Context, page, itemsPerPage int) (*RTSPSessionList, error) {
	reqURL := c.baseURL + "/v3/rtspsessions/list"

	// Build query string safely. Negative values mean "use server default".
	q := url.Values{}
	if page >= 0 {
		q.Set("page", fmt.Sprintf("%d", page))
	}
	if itemsPerPage >= 0 {
		q.Set("itemsPerPage", fmt.Sprintf("%d", itemsPerPage))
	}
	if encoded := q.Encode(); encoded != "" {
		reqURL += "?" + encoded
	}

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
			return nil, fmt.Errorf("API returned status %d (failed to read body: %w)", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var list RTSPSessionList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	if list.Items == nil {
		list.Items = []RTSPSession{}
	}
	return &list, nil
}

// uuidRe matches the canonical UUID format (8-4-4-4-12 hex digits).
// MediaMTX identifies RTSP sessions by UUID; validating before building the
// request URL is defense-in-depth against path injection in case a caller
// ever threads attacker-controlled input into a session ID. The kick handler
// URL-escapes the ID as well, so this regex is the second of two barriers.
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// isValidSessionID reports whether s is a syntactically valid MediaMTX
// session identifier (a UUID).
func isValidSessionID(s string) bool {
	return uuidRe.MatchString(s)
}

// KickRTSPSession forcibly disconnects an active RTSP session by ID.
//
// API endpoint: POST /v3/rtspsessions/kick/{id}
//
// Behavior:
//   - Returns nil on 200 OK.
//   - Returns a wrapped ErrSessionNotFound when the server replies 404,
//     which callers can match via errors.Is. This is typically non-fatal:
//     the session has already disconnected on its own.
//   - Returns a wrapped ErrInvalidSessionID when id is not a valid UUID.
//     The ID is also URL-escaped before being placed in the path as
//     defense-in-depth against injection.
//   - Returns a descriptive error for any other status code, including the
//     response body when possible for operator debugging.
//
// No request body is sent; the endpoint does not accept one.
func (c *Client) KickRTSPSession(ctx context.Context, id string) error {
	if !isValidSessionID(id) {
		return fmt.Errorf("%w: %q", ErrInvalidSessionID, id)
	}

	// url.PathEscape is defense-in-depth: isValidSessionID has already
	// constrained the input to UUID chars, but escaping ensures that any
	// future relaxation of the validator cannot by itself become an
	// injection vector.
	reqURL := fmt.Sprintf("%s/v3/rtspsessions/kick/%s", c.baseURL, url.PathEscape(id))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req) // #nosec G107 -- URL is derived from client baseURL with a validated, escaped id
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		// Drain body so the connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	case http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrSessionNotFound, id)
	default:
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		if readErr != nil {
			return fmt.Errorf("kick session: API returned status %d (failed to read body: %w)", resp.StatusCode, readErr)
		}
		return fmt.Errorf("kick session: API returned status %d: %s", resp.StatusCode, string(body))
	}
}

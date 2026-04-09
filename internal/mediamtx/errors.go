// SPDX-License-Identifier: MIT

package mediamtx

import "errors"

// Sentinel errors returned by the MediaMTX client.
//
// Callers should use errors.Is to match these values, since wrapping with
// fmt.Errorf("%w", …) is used to attach context.
var (
	// ErrSessionNotFound is returned when an RTSP session identified by ID
	// does not exist on the server. This typically means the session has
	// already disconnected, and is often not a fatal condition — for example,
	// a stall-recovery caller can treat it as "already gone" and skip
	// further work.
	ErrSessionNotFound = errors.New("mediamtx: rtsp session not found")

	// ErrInvalidSessionID is returned when a caller passes a session ID
	// that is not a valid UUID. The MediaMTX API uses UUIDs to identify
	// sessions, and validating before sending protects against URL path
	// injection via attacker-controlled input.
	ErrInvalidSessionID = errors.New("mediamtx: invalid rtsp session id")
)

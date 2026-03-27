// SPDX-License-Identifier: MIT

package stream

import (
	"fmt"
	"time"
)

// State represents the stream manager's current state.
type State int

const (
	StateIdle     State = iota // Not started
	StateStarting              // Acquiring lock and starting FFmpeg
	StateRunning               // FFmpeg process running
	StateStopping              // Gracefully stopping FFmpeg
	StateFailed                // FFmpeg failed, waiting for backoff
	StateStopped               // Stopped (terminal state)
)

// String returns the string representation of State.
func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateStopping:
		return "stopping"
	case StateFailed:
		return "failed"
	case StateStopped:
		return "stopped"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

// Metrics contains stream manager metrics.
type Metrics struct {
	DeviceName string
	StreamName string
	State      State
	StartTime  time.Time
	Uptime     time.Duration
	Attempts   int
	Failures   int
}

// State returns the current manager state.
//
// Returns StateIdle if the manager was not properly initialized
// (e.g., created via &Manager{} instead of NewManager()).
func (m *Manager) State() State {
	if m == nil {
		return StateIdle
	}
	v := m.state.Load()
	if v == nil {
		return StateIdle
	}
	return v.(State)
}

// Attempts returns the total number of start attempts.
func (m *Manager) Attempts() int {
	return int(m.attempts.Load())
}

// Failures returns the total number of failures.
func (m *Manager) Failures() int {
	return int(m.failures.Load())
}

// Metrics returns current manager metrics.
//
// Returns zero-value Metrics if manager is nil.
func (m *Manager) Metrics() Metrics {
	if m == nil {
		return Metrics{State: StateIdle}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var uptime time.Duration
	if !m.startTime.IsZero() {
		uptime = time.Since(m.startTime)
	}

	var deviceName, streamName string
	if m.cfg != nil {
		deviceName = m.cfg.DeviceName
		streamName = m.cfg.StreamName
	}

	return Metrics{
		DeviceName: deviceName,
		StreamName: streamName,
		State:      m.State(),
		StartTime:  m.startTime,
		Uptime:     uptime,
		Attempts:   m.Attempts(),
		Failures:   m.Failures(),
	}
}

// setState atomically updates the manager state.
func (m *Manager) setState(s State) {
	m.state.Store(s)
}

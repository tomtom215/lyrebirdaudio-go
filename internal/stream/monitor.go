package stream

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ResourceMetrics contains resource usage information for a process.
type ResourceMetrics struct {
	PID             int           // Process ID
	FileDescriptors int           // Number of open file descriptors
	CPUPercent      float64       // CPU usage percentage
	MemoryBytes     int64         // Resident memory in bytes
	MemoryPercent   float64       // Memory usage percentage
	ThreadCount     int           // Number of threads
	Uptime          time.Duration // Process uptime
	Timestamp       time.Time     // When metrics were collected
}

// ResourceThresholds defines warning and critical thresholds for resources.
//
// Reference: mediamtx-stream-manager.sh resource thresholds
type ResourceThresholds struct {
	FDWarning      int     // File descriptor warning threshold (default: 500)
	FDCritical     int     // File descriptor critical threshold (default: 1000)
	CPUWarning     float64 // CPU warning threshold % (default: 20.0)
	CPUCritical    float64 // CPU critical threshold % (default: 40.0)
	MemoryWarning  int64   // Memory warning threshold bytes (default: 512MB)
	MemoryCritical int64   // Memory critical threshold bytes (default: 1GB)
}

// DefaultThresholds returns sensible default resource thresholds.
//
// These values match the bash implementation.
func DefaultThresholds() ResourceThresholds {
	return ResourceThresholds{
		FDWarning:      500,
		FDCritical:     1000,
		CPUWarning:     20.0,
		CPUCritical:    40.0,
		MemoryWarning:  512 * 1024 * 1024,  // 512 MB
		MemoryCritical: 1024 * 1024 * 1024, // 1 GB
	}
}

// AlertLevel indicates the severity of a resource alert.
type AlertLevel int

const (
	AlertNone AlertLevel = iota
	AlertWarning
	AlertCritical
)

func (a AlertLevel) String() string {
	switch a {
	case AlertWarning:
		return "WARNING"
	case AlertCritical:
		return "CRITICAL"
	default:
		return "OK"
	}
}

// ResourceAlert represents an alert for resource usage.
type ResourceAlert struct {
	Level    AlertLevel
	Resource string // "fd", "cpu", "memory"
	Message  string
	Value    interface{}
}

// ResourceMonitor monitors resource usage for processes.
type ResourceMonitor struct {
	thresholds ResourceThresholds
	logger     io.Writer
	mu         sync.RWMutex
	metrics    map[int]*ResourceMetrics // PID -> metrics
	procPath   string                   // Path to /proc (for testing)
}

// MonitorOption is a functional option for configuring the monitor.
type MonitorOption func(*ResourceMonitor)

// WithThresholds sets custom resource thresholds.
func WithThresholds(t ResourceThresholds) MonitorOption {
	return func(m *ResourceMonitor) {
		m.thresholds = t
	}
}

// WithLogger sets a logger for the monitor.
func WithLogger(w io.Writer) MonitorOption {
	return func(m *ResourceMonitor) {
		m.logger = w
	}
}

// WithProcPath sets a custom /proc path (for testing).
func WithProcPath(path string) MonitorOption {
	return func(m *ResourceMonitor) {
		m.procPath = path
	}
}

// NewResourceMonitor creates a new resource monitor.
func NewResourceMonitor(opts ...MonitorOption) *ResourceMonitor {
	m := &ResourceMonitor{
		thresholds: DefaultThresholds(),
		metrics:    make(map[int]*ResourceMetrics),
		procPath:   "/proc",
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// GetMetrics collects current resource metrics for a process.
//
// Reads from /proc/{pid}/ to get resource information.
//
// Parameters:
//   - pid: Process ID to monitor
//
// Returns:
//   - *ResourceMetrics: Current resource usage
//   - error: if process doesn't exist or can't be read
func (m *ResourceMonitor) GetMetrics(pid int) (*ResourceMetrics, error) {
	procDir := filepath.Join(m.procPath, strconv.Itoa(pid))

	// Verify process exists
	if _, err := os.Stat(procDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("process %d not found", pid)
	}

	metrics := &ResourceMetrics{
		PID:       pid,
		Timestamp: time.Now(),
	}

	// Count file descriptors
	fdDir := filepath.Join(procDir, "fd")
	if entries, err := os.ReadDir(fdDir); err == nil {
		metrics.FileDescriptors = len(entries)
	}

	// Read stat for CPU and thread info
	statPath := filepath.Join(procDir, "stat")
	// #nosec G304 -- reading from /proc, controlled path
	if data, err := os.ReadFile(statPath); err == nil {
		metrics.ThreadCount = parseThreadCount(string(data))
		// Note: CPU percentage requires delta calculation over time
		// For now, we just parse the raw values
	}

	// Read statm for memory info
	statmPath := filepath.Join(procDir, "statm")
	// #nosec G304 -- reading from /proc, controlled path
	if data, err := os.ReadFile(statmPath); err == nil {
		metrics.MemoryBytes = parseMemoryBytes(string(data))
	}

	// Calculate uptime from process start time
	if startTime, err := m.getProcessStartTime(pid); err == nil {
		metrics.Uptime = time.Since(startTime)
	}

	// Store metrics
	m.mu.Lock()
	m.metrics[pid] = metrics
	m.mu.Unlock()

	return metrics, nil
}

// CheckThresholds checks metrics against thresholds and returns alerts.
//
// Parameters:
//   - metrics: Resource metrics to check
//
// Returns:
//   - []ResourceAlert: List of alerts (may be empty if all OK)
func (m *ResourceMonitor) CheckThresholds(metrics *ResourceMetrics) []ResourceAlert {
	var alerts []ResourceAlert

	// Check file descriptors
	if metrics.FileDescriptors >= m.thresholds.FDCritical {
		alerts = append(alerts, ResourceAlert{
			Level:    AlertCritical,
			Resource: "fd",
			Message:  fmt.Sprintf("File descriptors at critical level: %d >= %d", metrics.FileDescriptors, m.thresholds.FDCritical),
			Value:    metrics.FileDescriptors,
		})
	} else if metrics.FileDescriptors >= m.thresholds.FDWarning {
		alerts = append(alerts, ResourceAlert{
			Level:    AlertWarning,
			Resource: "fd",
			Message:  fmt.Sprintf("File descriptors at warning level: %d >= %d", metrics.FileDescriptors, m.thresholds.FDWarning),
			Value:    metrics.FileDescriptors,
		})
	}

	// Check CPU
	if metrics.CPUPercent >= m.thresholds.CPUCritical {
		alerts = append(alerts, ResourceAlert{
			Level:    AlertCritical,
			Resource: "cpu",
			Message:  fmt.Sprintf("CPU usage at critical level: %.1f%% >= %.1f%%", metrics.CPUPercent, m.thresholds.CPUCritical),
			Value:    metrics.CPUPercent,
		})
	} else if metrics.CPUPercent >= m.thresholds.CPUWarning {
		alerts = append(alerts, ResourceAlert{
			Level:    AlertWarning,
			Resource: "cpu",
			Message:  fmt.Sprintf("CPU usage at warning level: %.1f%% >= %.1f%%", metrics.CPUPercent, m.thresholds.CPUWarning),
			Value:    metrics.CPUPercent,
		})
	}

	// Check memory
	if metrics.MemoryBytes >= m.thresholds.MemoryCritical {
		alerts = append(alerts, ResourceAlert{
			Level:    AlertCritical,
			Resource: "memory",
			Message:  fmt.Sprintf("Memory usage at critical level: %d bytes >= %d bytes", metrics.MemoryBytes, m.thresholds.MemoryCritical),
			Value:    metrics.MemoryBytes,
		})
	} else if metrics.MemoryBytes >= m.thresholds.MemoryWarning {
		alerts = append(alerts, ResourceAlert{
			Level:    AlertWarning,
			Resource: "memory",
			Message:  fmt.Sprintf("Memory usage at warning level: %d bytes >= %d bytes", metrics.MemoryBytes, m.thresholds.MemoryWarning),
			Value:    metrics.MemoryBytes,
		})
	}

	return alerts
}

// MonitorProcess starts continuous monitoring of a process.
//
// Periodically collects metrics and logs alerts. Stops when context is cancelled.
//
// Parameters:
//   - ctx: Context for cancellation
//   - pid: Process ID to monitor
//   - interval: Time between metric collections
//   - alertCallback: Called when alerts are generated (optional)
func (m *ResourceMonitor) MonitorProcess(ctx context.Context, pid int, interval time.Duration, alertCallback func([]ResourceAlert)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			metrics, err := m.GetMetrics(pid)
			if err != nil {
				// Process may have exited
				if m.logger != nil {
					fmt.Fprintf(m.logger, "Failed to get metrics for PID %d: %v\n", pid, err)
				}
				return
			}

			alerts := m.CheckThresholds(metrics)
			if len(alerts) > 0 {
				if m.logger != nil {
					for _, alert := range alerts {
						fmt.Fprintf(m.logger, "[%s] PID %d: %s\n", alert.Level, pid, alert.Message)
					}
				}
				if alertCallback != nil {
					alertCallback(alerts)
				}
			}
		}
	}
}

// GetCachedMetrics returns the last collected metrics for a process.
func (m *ResourceMonitor) GetCachedMetrics(pid int) *ResourceMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.metrics[pid]
}

// ClearMetrics removes cached metrics for a process.
func (m *ResourceMonitor) ClearMetrics(pid int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.metrics, pid)
}

// getProcessStartTime reads the process start time from /proc/{pid}/stat.
func (m *ResourceMonitor) getProcessStartTime(pid int) (time.Time, error) {
	statPath := filepath.Join(m.procPath, strconv.Itoa(pid), "stat")
	// #nosec G304 -- reading from /proc, controlled path
	data, err := os.ReadFile(statPath)
	if err != nil {
		return time.Time{}, err
	}

	// Parse start time (field 22 in stat, after the comm field in parentheses)
	content := string(data)
	// Find closing paren of comm field
	idx := strings.LastIndex(content, ")")
	if idx == -1 {
		return time.Time{}, fmt.Errorf("invalid stat format")
	}

	fields := strings.Fields(content[idx+1:])
	if len(fields) < 20 {
		return time.Time{}, fmt.Errorf("insufficient fields in stat")
	}

	// Field 20 (0-indexed from after comm) is start time in clock ticks
	startTicks, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return time.Time{}, err
	}

	// Convert to time (requires system boot time and clock ticks per second)
	// This is an approximation - actual conversion requires reading /proc/stat
	bootTime := getSystemBootTime(m.procPath)
	ticksPerSecond := int64(100) // Typical value, should use sysconf(_SC_CLK_TCK)
	startSeconds := startTicks / ticksPerSecond

	return bootTime.Add(time.Duration(startSeconds) * time.Second), nil
}

// parseThreadCount extracts thread count from /proc/{pid}/stat content.
func parseThreadCount(stat string) int {
	// Thread count is field 20 (1-indexed) after the comm field
	idx := strings.LastIndex(stat, ")")
	if idx == -1 {
		return 0
	}

	fields := strings.Fields(stat[idx+1:])
	if len(fields) < 18 {
		return 0
	}

	// Field 17 (0-indexed from after comm) is num_threads
	threads, err := strconv.Atoi(fields[17])
	if err != nil {
		return 0
	}
	return threads
}

// parseMemoryBytes extracts resident memory from /proc/{pid}/statm content.
func parseMemoryBytes(statm string) int64 {
	fields := strings.Fields(statm)
	if len(fields) < 2 {
		return 0
	}

	// Second field is resident set size in pages
	pages, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0
	}

	// Convert pages to bytes (typically 4096 bytes per page)
	pageSize := int64(os.Getpagesize())
	return pages * pageSize
}

// getSystemBootTime reads the system boot time from /proc/stat.
func getSystemBootTime(procPath string) time.Time {
	statPath := filepath.Join(procPath, "stat")
	// #nosec G304 -- reading from /proc, controlled path
	data, err := os.ReadFile(statPath)
	if err != nil {
		return time.Now() // Fallback to current time
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "btime ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				bootSecs, err := strconv.ParseInt(fields[1], 10, 64)
				if err == nil {
					return time.Unix(bootSecs, 0)
				}
			}
		}
	}

	return time.Now() // Fallback
}

// GetSystemFDLimits returns the system-wide file descriptor limits.
func GetSystemFDLimits(procPath string) (current, max int, err error) {
	path := filepath.Join(procPath, "sys", "fs", "file-nr")
	// #nosec G304 -- reading from /proc, controlled path
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}

	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return 0, 0, fmt.Errorf("invalid file-nr format")
	}

	current64, _ := strconv.ParseInt(fields[0], 10, 64)
	max64, _ := strconv.ParseInt(fields[2], 10, 64)

	return int(current64), int(max64), nil
}

// FormatBytes formats bytes as human-readable string.
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"log/slog"
	"os/exec"
	"strings"
	"syscall"

	"github.com/tomtom215/lyrebirdaudio-go/internal/health"
	"github.com/tomtom215/lyrebirdaudio-go/internal/stream"
	"github.com/tomtom215/lyrebirdaudio-go/internal/supervisor"
)

// streamService wraps a stream.Manager to implement supervisor.Service.
type streamService struct {
	name    string
	manager *stream.Manager
	logger  *slog.Logger
}

func (s *streamService) Name() string {
	return s.name
}

func (s *streamService) Run(ctx context.Context) error {
	s.logger.Info("starting stream", "service", s.name)
	err := s.manager.Run(ctx)
	// M-5 fix: Release the lock file and other resources held by the manager
	// regardless of whether Run returned an error.  Without this call the lock
	// file fd is not closed until the process exits, which leaks the fd and
	// prevents a clean re-acquire after a hot-reload.
	if closeErr := s.manager.Close(); closeErr != nil {
		s.logger.Warn("manager close error", "service", s.name, "error", closeErr)
	}
	// M-1 fix: use errors.Is so that wrapped context errors are handled correctly.
	if err != nil && !errors.Is(err, context.Canceled) {
		s.logger.Error("stream stopped with error", "service", s.name, "error", err)
	} else {
		s.logger.Info("stream stopped", "service", s.name)
	}
	return err
}

// supervisorStatusProvider implements health.StatusProvider by querying the
// supervisor for live service state. This replaces the nil provider that was
// previously passed to health.NewHandler (P-4 fix).
type supervisorStatusProvider struct {
	sup *supervisor.Supervisor
}

func (p *supervisorStatusProvider) Services() []health.ServiceInfo {
	statuses := p.sup.Status()
	services := make([]health.ServiceInfo, len(statuses))
	for i, s := range statuses {
		services[i] = health.ServiceInfo{
			Name:     s.Name,
			State:    s.State.String(),
			Uptime:   s.Uptime,
			Healthy:  s.State == supervisor.ServiceStateRunning,
			Restarts: s.Restarts,
		}
		if s.LastError != nil {
			services[i].Error = s.LastError.Error()
		}
	}
	return services
}

// daemonSystemInfoProvider implements health.SystemInfoProvider for the daemon.
// It reports disk space for the recording directory and NTP sync status (GAP-7, GAP-1d).
type daemonSystemInfoProvider struct {
	recordDir        string // LocalRecordDir, or "/" if empty
	diskLowThreshold uint64 // bytes; 0 = disabled (always initialized from a positive int64)
}

func (p *daemonSystemInfoProvider) SystemInfo() health.SystemInfo {
	dir := p.recordDir
	if dir == "" {
		dir = "/"
	}

	var si health.SystemInfo

	// Disk space check via syscall.Statfs.
	var stat syscall.Statfs_t
	if err := syscall.Statfs(dir, &stat); err == nil {
		si.DiskFreeBytes = stat.Bavail * uint64(stat.Bsize)  //#nosec G115 -- Bavail and Bsize are always ≥ 0
		si.DiskTotalBytes = stat.Blocks * uint64(stat.Bsize) //#nosec G115 -- same
		if p.diskLowThreshold > 0 && si.DiskFreeBytes < p.diskLowThreshold {
			si.DiskLowWarning = true
		}
	}

	// NTP sync check via timedatectl.
	out, err := exec.Command("timedatectl", "show", "--property=NTPSynchronized", "--value").Output() //#nosec G204 -- fixed args
	if err == nil && strings.TrimSpace(string(out)) == "yes" {
		si.NTPSynced = true
	} else {
		si.NTPMessage = "NTP not synchronized or timedatectl unavailable"
	}

	return si
}

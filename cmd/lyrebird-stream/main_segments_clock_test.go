// SPDX-License-Identifier: MIT

package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
)

// TestCleanupSegmentsKeepsPreClockSyncRecordings covers the no-RTC boot
// sequence every Raspberry Pi field station goes through: the clock starts at
// (or near) the Unix epoch, FFmpeg records segments stamped ~1970, then NTP
// steps the clock forward to the real date. The very next hourly retention
// pass would see those segments as "55 years old" and delete every recording
// made before time sync — irreplaceable bioacoustic data captured while the
// station was offline. Files whose mtime predates the sanity floor carry a
// bogus timestamp and must be exempt from AGE-based deletion.
func TestCleanupSegmentsKeepsPreClockSyncRecordings(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	write := func(name string, mtime time.Time) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("audio data"), 0600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatalf("chtimes %s: %v", path, err)
		}
		return path
	}

	now := time.Now()
	// Recorded before NTP sync: stamped a few hours after the epoch.
	preSync := write("presync_19700101_030000.ogg", time.Unix(3*3600, 0))
	// Genuinely expired segment with a sane timestamp.
	expired := write("expired.ogg", now.Add(-30*24*time.Hour))
	// Fresh segment.
	fresh := write("fresh.ogg", now.Add(-time.Hour))

	cfg := config.StreamConfig{
		LocalRecordDir: dir,
		SegmentMaxAge:  7 * 24 * time.Hour,
	}

	cleanupSegments(logger, cfg)

	if _, err := os.Stat(preSync); os.IsNotExist(err) {
		t.Error("pre-clock-sync segment (epoch mtime) was age-deleted; bogus timestamps must be exempt from age-based retention")
	}
	if _, err := os.Stat(expired); !os.IsNotExist(err) {
		t.Error("genuinely expired segment with a sane timestamp should still be deleted")
	}
	if _, err := os.Stat(fresh); os.IsNotExist(err) {
		t.Error("fresh segment should be kept")
	}
}

// TestCleanupSegmentsSizeBudgetStillCoversBogusTimestamps pins that the
// age-exemption above does NOT leak disk: under the SIZE budget, segments
// with bogus (epoch) timestamps sort oldest and are deleted first, so a full
// disk still recovers even if the clock never syncs.
func TestCleanupSegmentsSizeBudgetStillCoversBogusTimestamps(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	data := make([]byte, 1024)
	write := func(name string, mtime time.Time) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatalf("chtimes %s: %v", path, err)
		}
		return path
	}

	now := time.Now()
	bogus := write("presync.ogg", time.Unix(3600, 0))
	newer := write("newer.ogg", now.Add(-time.Hour))

	cfg := config.StreamConfig{
		LocalRecordDir:       dir,
		SegmentMaxTotalBytes: 1024, // room for exactly one file
	}

	cleanupSegments(logger, cfg)

	if _, err := os.Stat(bogus); !os.IsNotExist(err) {
		t.Error("bogus-timestamp segment should be deleted FIRST under the size budget")
	}
	if _, err := os.Stat(newer); os.IsNotExist(err) {
		t.Error("newer segment should survive the size budget")
	}
}

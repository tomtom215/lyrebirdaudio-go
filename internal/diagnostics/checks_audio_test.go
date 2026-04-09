// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestCheckAudioConflictsSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkAudioConflicts(ctx)

	if result.Name != "Audio Conflicts" {
		t.Errorf("expected Name 'Audio Conflicts', got %q", result.Name)
	}
	if result.Category != "Audio" {
		t.Errorf("expected Category 'Audio', got %q", result.Category)
	}
	if result.Status == "" {
		t.Error("expected non-empty status")
	}
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestCheckUSBAudioSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	result := runner.checkUSBAudio(context.Background())

	if result.Name != "USB Audio" {
		t.Errorf("expected Name 'USB Audio', got %q", result.Name)
	}
	if result.Category != "Audio" {
		t.Errorf("expected Category 'Audio', got %q", result.Category)
	}
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("unexpected status %s", result.Status)
	}
	if result.Status == StatusWarning {
		if !strings.Contains(result.Message, "No USB audio devices") {
			t.Errorf("unexpected warning message: %q", result.Message)
		}
	}
	if result.Status == StatusOK {
		if !strings.Contains(result.Message, "USB audio device") {
			t.Errorf("unexpected OK message: %q", result.Message)
		}
	}
}

func TestCheckALSASetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkALSA(ctx)

	if result.Name != "ALSA" {
		t.Errorf("expected Name 'ALSA', got %q", result.Name)
	}
	if result.Category != "Audio" {
		t.Errorf("expected Category 'Audio', got %q", result.Category)
	}
	if result.Status == StatusCritical {
		if !strings.Contains(result.Message, "/proc/asound missing") {
			t.Errorf("unexpected critical message: %q", result.Message)
		}
	}
}

func TestCheckFFmpegSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := runner.checkFFmpeg(ctx)

	if result.Name != "FFmpeg" {
		t.Errorf("expected Name 'FFmpeg', got %q", result.Name)
	}
	if result.Category != "Tools" {
		t.Errorf("expected Category 'Tools', got %q", result.Category)
	}

	switch result.Status {
	case StatusCritical:
		if !strings.Contains(result.Message, "not found") {
			t.Errorf("unexpected critical message: %q", result.Message)
		}
		if len(result.Suggestions) == 0 {
			t.Error("expected suggestions when FFmpeg is missing")
		}
	case StatusOK:
		if result.Details == "" {
			t.Error("expected non-empty Details when FFmpeg is found")
		}
	case StatusWarning:
		t.Logf("FFmpeg warning: %s", result.Message)
	default:
		t.Errorf("unexpected status %s for FFmpeg check", result.Status)
	}
}

func TestCheckAudioCapabilitiesSetsFields(t *testing.T) {
	opts := DefaultOptions()
	runner := NewRunner(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := runner.checkAudioCapabilities(ctx)

	if result.Name != "Audio Capabilities" {
		t.Errorf("expected Name 'Audio Capabilities', got %q", result.Name)
	}
	if result.Category != "Audio" {
		t.Errorf("expected Category 'Audio', got %q", result.Category)
	}
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("unexpected status %s: %s", result.Status, result.Message)
	}
}

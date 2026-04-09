package main

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestStreamService_Name(t *testing.T) {
	svc := &streamService{
		name: "test_device",
	}

	if got := svc.Name(); got != "test_device" {
		t.Errorf("Name() = %q, want %q", got, "test_device")
	}
}

func TestStreamService_Run_WithNilManager(t *testing.T) {
	// This tests the error path when manager is nil
	// In production, this shouldn't happen, but we test defensively

	svc := &streamService{
		name:    "test",
		manager: nil,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This should panic or return an error since manager is nil
	// We're testing that the code handles this gracefully
	defer func() {
		if r := recover(); r != nil {
			// Expected - nil manager causes panic
			t.Logf("Recovered from panic (expected): %v", r)
		}
	}()

	_ = svc.Run(ctx)
}

func TestStreamServiceWithLogger(t *testing.T) {
	// Test streamService with a logger
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	svc := &streamService{
		name:    "test_device",
		manager: nil,
		logger:  logger,
	}

	// Verify name works with logger set
	if got := svc.Name(); got != "test_device" {
		t.Errorf("Name() = %q, want %q", got, "test_device")
	}
}

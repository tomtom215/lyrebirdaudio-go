package main

import (
	"os"
	"testing"
)

func TestSdNotifyNoSocket(t *testing.T) {
	// Without NOTIFY_SOCKET set, sdNotify should be a no-op.
	if err := os.Unsetenv("NOTIFY_SOCKET"); err != nil {
		t.Fatal(err)
	}
	if err := sdNotify("WATCHDOG=1"); err != nil {
		t.Errorf("sdNotify should be no-op without NOTIFY_SOCKET, got: %v", err)
	}
}

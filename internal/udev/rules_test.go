package udev

import (
	"fmt"
	"strings"
	"testing"
)

// TestWriteRulesFileToPathWithReload tests the reload=true path using injectable runner.
func TestWriteRulesFileToPathWithReload(t *testing.T) {
	t.Run("reload succeeds", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := tmpDir + "/rules.test"
		var reloaded bool
		runner := func(name string, args ...string) ([]byte, error) {
			reloaded = true
			return nil, nil
		}

		devices := []*DeviceInfo{{PortPath: "1-1.4", BusNum: 1, DevNum: 5}}
		err := writeRulesFileToPathWithRunner(devices, path, true, runner)
		if err != nil {
			t.Fatalf("writeRulesFileToPathWithRunner() unexpected error: %v", err)
		}
		if !reloaded {
			t.Error("expected udev runner to be called for reload")
		}
	})

	t.Run("reload fails returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := tmpDir + "/rules.test"
		runner := func(name string, args ...string) ([]byte, error) {
			return []byte("no udevadm"), fmt.Errorf("not found")
		}

		devices := []*DeviceInfo{{PortPath: "1-1.4", BusNum: 1, DevNum: 5}}
		err := writeRulesFileToPathWithRunner(devices, path, true, runner)
		if err == nil {
			t.Fatal("expected error when reload fails")
		}
		if !strings.Contains(err.Error(), "failed to reload udev rules") {
			t.Errorf("error = %q, want 'failed to reload udev rules'", err.Error())
		}
	})

	t.Run("reload=false skips runner", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := tmpDir + "/rules.test"
		var called bool
		runner := func(name string, args ...string) ([]byte, error) {
			called = true
			return nil, nil
		}

		devices := []*DeviceInfo{{PortPath: "1-1.4", BusNum: 1, DevNum: 5}}
		err := writeRulesFileToPathWithRunner(devices, path, false, runner)
		if err != nil {
			t.Fatalf("writeRulesFileToPathWithRunner() unexpected error: %v", err)
		}
		if called {
			t.Error("runner should not be called when reload=false")
		}
	})
}

// TestWriteRulesFile tests the top-level WriteRulesFile function.
// Since it writes to /etc/udev/rules.d/ (requires root), we verify
// the error path when running unprivileged.
func TestWriteRulesFile(t *testing.T) {
	devices := []*DeviceInfo{{PortPath: "1-1.4", BusNum: 1, DevNum: 5}}
	err := WriteRulesFile(devices, false)
	if err == nil {
		// Running as root — skip the assertion but the call must not panic
		t.Log("WriteRulesFile() succeeded (running as root)")
		return
	}
	if !strings.Contains(err.Error(), "failed to write rules file") {
		t.Errorf("WriteRulesFile() error = %q, want 'failed to write rules file'", err.Error())
	}
}

// BenchmarkGenerateRule measures performance of rule generation.
func BenchmarkGenerateRule(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = GenerateRule("1-1.4", 1, 5)
	}
}

// BenchmarkGenerateRulesFile measures performance of full file generation.
func BenchmarkGenerateRulesFile(b *testing.B) {
	devices := make([]*DeviceInfo, 10)

	for i := range devices {
		devices[i] = &DeviceInfo{
			PortPath: "1-1.4",
			BusNum:   1,
			DevNum:   i + 1,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GenerateRulesFile(devices)
	}
}

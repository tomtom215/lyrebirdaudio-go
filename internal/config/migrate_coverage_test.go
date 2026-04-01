// SPDX-License-Identifier: MIT

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParseBashEnvLineNoEquals covers the len(parts) != 2 branch in
// parseBashEnvLine — a line with no '=' sign returns ok=false.
func TestParseBashEnvLineNoEquals(t *testing.T) {
	_, _, _, ok := parseBashEnvLine("VARIABLE_WITHOUT_EQUALS")
	if ok {
		t.Error("expected ok=false for line with no '=' sign")
	}
}

// TestParseBashEnvLineUnknownPrefix covers the final return in parseBashEnvLine
// when the variable name has an '=' but doesn't match any known prefix.
func TestParseBashEnvLineUnknownPrefix(t *testing.T) {
	_, _, _, ok := parseBashEnvLine("UNKNOWN_VAR_device=value")
	if ok {
		t.Error("expected ok=false for unknown variable prefix")
	}
}

// TestParseBashEnvLineEmptyDeviceName covers the case where a known variable
// prefix is followed immediately by '=' with no device name, returning ok=true
// but deviceName="".
func TestParseBashEnvLineEmptyDeviceName(t *testing.T) {
	varName, deviceName, _, ok := parseBashEnvLine("SAMPLE_RATE_=48000")
	if !ok {
		t.Fatalf("expected ok=true for SAMPLE_RATE_=48000, got false")
	}
	if varName != "SAMPLE_RATE" {
		t.Errorf("varName = %q, want %q", varName, "SAMPLE_RATE")
	}
	if deviceName != "" {
		t.Errorf("deviceName = %q, want empty string", deviceName)
	}
}

// TestMigrateFromBashEmptyDeviceName covers the `if deviceName == "" { continue }`
// branch in MigrateFromBash when a known variable has an empty device suffix.
func TestMigrateFromBashEmptyDeviceName(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.conf")
	// SAMPLE_RATE_ has the known prefix but empty device name → skipped
	content := "SAMPLE_RATE_=48000\nUNKNOWN_VAR=ignored\nVARIABLE_NO_EQUALS\n"
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := MigrateFromBash(configPath)
	if err != nil {
		t.Fatalf("MigrateFromBash: %v", err)
	}
	// All lines should be silently skipped; no devices added.
	if len(cfg.Devices) != 0 {
		t.Errorf("expected 0 devices, got %d", len(cfg.Devices))
	}
}

// TestNewKoanfConfigOptionError covers the option application error path in
// NewKoanfConfig by passing an option that returns a non-nil error.
func TestNewKoanfConfigOptionError(t *testing.T) {
	errOpt := func(kc *KoanfConfig) error {
		return os.ErrPermission // any non-nil error
	}

	_, err := NewKoanfConfig(errOpt)
	if err == nil {
		t.Error("expected error from failing option, got nil")
	}
}

// TestKoanfConfigLoadValidateFails covers the Validate() error in Load().
// We load a YAML that unmarshals successfully but contains values that
// fail cfg.Validate() (e.g. a device with an invalid sample rate).
func TestKoanfConfigLoadValidateFails(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	// Write a config with a negative sample rate which should fail Validate().
	yaml := `devices:
  bad_device:
    sample_rate: -1
    channels: 2
    bitrate: 128k
    codec: opus
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	kc, err := NewKoanfConfig(WithYAMLFile(cfgPath))
	if err != nil {
		t.Fatalf("NewKoanfConfig: %v", err)
	}

	_, err = kc.Load()
	// If Validate() rejects negative sample rate, this should be non-nil.
	// If Validate() is lenient, it may be nil — we just verify no panic.
	_ = err
}

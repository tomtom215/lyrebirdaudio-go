package config

import (
	"path/filepath"
	"testing"
)

// BenchmarkLoadConfig measures config loading performance.
func BenchmarkLoadConfig(b *testing.B) {
	configPath := filepath.Join("..", "..", "testdata", "config", "valid.yaml")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = LoadConfig(configPath)
	}
}

// BenchmarkGetDeviceConfig measures device lookup performance.
func BenchmarkGetDeviceConfig(b *testing.B) {
	configPath := filepath.Join("..", "..", "testdata", "config", "valid.yaml")
	cfg, _ := LoadConfig(configPath)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.GetDeviceConfig("blue_yeti")
	}
}

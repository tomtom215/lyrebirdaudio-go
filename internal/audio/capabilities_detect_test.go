package audio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectCapabilities(t *testing.T) {
	tests := []struct {
		name       string
		setupFunc  func(t *testing.T, tmpDir string)
		cardNumber int
		wantErr    bool
		checkFunc  func(t *testing.T, caps *Capabilities)
	}{
		{
			name: "valid card with full stream0",
			setupFunc: func(t *testing.T, tmpDir string) {
				cardDir := filepath.Join(tmpDir, "card0")
				if err := os.MkdirAll(cardDir, 0755); err != nil {
					t.Fatal(err)
				}
				// Create id file
				if err := os.WriteFile(filepath.Join(cardDir, "id"), []byte("Blue_Yeti\n"), 0644); err != nil {
					t.Fatal(err)
				}
				// Create stream0 with capabilities
				stream0Content := `USB Audio
  Status: Stop
  Interface 1
    Altset 1
    Format: S16_LE
    Channels: 2
    Endpoint: 1 IN (ASYNC)
    Rates: 44100, 48000
`
				if err := os.WriteFile(filepath.Join(cardDir, "stream0"), []byte(stream0Content), 0644); err != nil {
					t.Fatal(err)
				}
			},
			cardNumber: 0,
			wantErr:    false,
			checkFunc: func(t *testing.T, caps *Capabilities) {
				if caps.DeviceName != "Blue_Yeti" {
					t.Errorf("DeviceName = %q, want %q", caps.DeviceName, "Blue_Yeti")
				}
				if !contains(caps.Formats, "S16_LE") {
					t.Errorf("Formats should contain S16_LE, got %v", caps.Formats)
				}
				if !containsInt(caps.SampleRates, 44100) || !containsInt(caps.SampleRates, 48000) {
					t.Errorf("SampleRates should contain 44100 and 48000, got %v", caps.SampleRates)
				}
				if !containsInt(caps.Channels, 2) {
					t.Errorf("Channels should contain 2, got %v", caps.Channels)
				}
			},
		},
		{
			name: "card with rate range",
			setupFunc: func(t *testing.T, tmpDir string) {
				cardDir := filepath.Join(tmpDir, "card1")
				if err := os.MkdirAll(cardDir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(cardDir, "id"), []byte("USB_Mic\n"), 0644); err != nil {
					t.Fatal(err)
				}
				stream0Content := `USB Audio
  Interface 1
    Altset 1
    Format: S24_LE
    Channels: 1
    Endpoint: 1 IN (ASYNC)
    Rates: 8000 - 96000
`
				if err := os.WriteFile(filepath.Join(cardDir, "stream0"), []byte(stream0Content), 0644); err != nil {
					t.Fatal(err)
				}
			},
			cardNumber: 1,
			wantErr:    false,
			checkFunc: func(t *testing.T, caps *Capabilities) {
				if caps.MinRate != 8000 {
					t.Errorf("MinRate = %d, want 8000", caps.MinRate)
				}
				if caps.MaxRate != 96000 {
					t.Errorf("MaxRate = %d, want 96000", caps.MaxRate)
				}
				// Should generate common rates in range
				if !containsInt(caps.SampleRates, 48000) {
					t.Errorf("SampleRates should contain 48000, got %v", caps.SampleRates)
				}
			},
		},
		{
			name: "card not found",
			setupFunc: func(t *testing.T, tmpDir string) {
				// Don't create any card directories
			},
			cardNumber: 99,
			wantErr:    true,
		},
		{
			name: "card without stream0 uses fallback",
			setupFunc: func(t *testing.T, tmpDir string) {
				cardDir := filepath.Join(tmpDir, "card2")
				if err := os.MkdirAll(cardDir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(cardDir, "id"), []byte("Fallback_Device\n"), 0644); err != nil {
					t.Fatal(err)
				}
				// No stream0 file - should use fallback defaults
			},
			cardNumber: 2,
			wantErr:    false,
			checkFunc: func(t *testing.T, caps *Capabilities) {
				// Should have fallback defaults
				if len(caps.Formats) == 0 {
					t.Error("Should have fallback formats")
				}
				if len(caps.SampleRates) == 0 {
					t.Error("Should have fallback sample rates")
				}
			},
		},
		{
			name: "busy device detection",
			setupFunc: func(t *testing.T, tmpDir string) {
				cardDir := filepath.Join(tmpDir, "card3")
				pcmDir := filepath.Join(cardDir, "pcm0c", "sub0")
				if err := os.MkdirAll(pcmDir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(cardDir, "id"), []byte("Busy_Device\n"), 0644); err != nil {
					t.Fatal(err)
				}
				// Create hw_params with active state
				if err := os.WriteFile(filepath.Join(pcmDir, "hw_params"), []byte("format: S16_LE\nrate: 48000\n"), 0644); err != nil {
					t.Fatal(err)
				}
				// Create status with RUNNING state
				if err := os.WriteFile(filepath.Join(pcmDir, "status"), []byte("state: RUNNING\nowner_pid: 12345\n"), 0644); err != nil {
					t.Fatal(err)
				}
			},
			cardNumber: 3,
			wantErr:    false,
			checkFunc: func(t *testing.T, caps *Capabilities) {
				if !caps.IsBusy {
					t.Error("Device should be marked as busy")
				}
			},
		},
		{
			name: "available device (not busy)",
			setupFunc: func(t *testing.T, tmpDir string) {
				cardDir := filepath.Join(tmpDir, "card4")
				pcmDir := filepath.Join(cardDir, "pcm0c", "sub0")
				if err := os.MkdirAll(pcmDir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(cardDir, "id"), []byte("Available_Device\n"), 0644); err != nil {
					t.Fatal(err)
				}
				// Create hw_params with closed state
				if err := os.WriteFile(filepath.Join(pcmDir, "hw_params"), []byte("closed"), 0644); err != nil {
					t.Fatal(err)
				}
			},
			cardNumber: 4,
			wantErr:    false,
			checkFunc: func(t *testing.T, caps *Capabilities) {
				if caps.IsBusy {
					t.Error("Device should not be marked as busy")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tt.setupFunc(t, tmpDir)

			caps, err := DetectCapabilities(tmpDir, tt.cardNumber)

			if (err != nil) != tt.wantErr {
				t.Errorf("DetectCapabilities() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.checkFunc != nil {
				tt.checkFunc(t, caps)
			}
		})
	}
}

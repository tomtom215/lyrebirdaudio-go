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

func TestParseStreamFile(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantFormats []string
		wantRates   []int
		wantChans   []int
		wantErr     bool
	}{
		{
			name: "basic USB audio",
			content: `USB Audio
  Status: Stop
  Interface 1
    Altset 1
    Format: S16_LE
    Channels: 2
    Endpoint: 1 IN (ASYNC)
    Rates: 44100, 48000
`,
			wantFormats: []string{"S16_LE"},
			wantRates:   []int{44100, 48000},
			wantChans:   []int{2},
			wantErr:     false,
		},
		{
			name: "multiple formats",
			content: `USB Audio Device
  Interface 1
    Altset 1
    Format: S16_LE
    Format: S24_LE
    Channels: 2
    Endpoint: 1 IN (SYNC)
    Rates: 48000, 96000
`,
			wantFormats: []string{"S16_LE", "S24_LE"},
			wantRates:   []int{48000, 96000},
			wantChans:   []int{2},
			wantErr:     false,
		},
		{
			name: "rate range format",
			content: `USB Microphone
  Interface 1
    Altset 1
    Format: S32_LE
    Channels: 1
    Endpoint: 1 IN (ASYNC)
    Rates: 8000 - 192000
`,
			wantFormats: []string{"S32_LE"},
			wantRates:   []int{8000, 11025, 16000, 22050, 32000, 44100, 48000, 88200, 96000, 176400, 192000},
			wantChans:   []int{1},
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			path := filepath.Join(tmpDir, "stream0")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}

			caps := &Capabilities{}
			err := parseStreamFile(path, caps)

			if (err != nil) != tt.wantErr {
				t.Errorf("parseStreamFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				for _, f := range tt.wantFormats {
					if !contains(caps.Formats, f) {
						t.Errorf("Formats should contain %s, got %v", f, caps.Formats)
					}
				}
				for _, r := range tt.wantRates {
					if !containsInt(caps.SampleRates, r) {
						t.Errorf("SampleRates should contain %d, got %v", r, caps.SampleRates)
					}
				}
				for _, c := range tt.wantChans {
					if !containsInt(caps.Channels, c) {
						t.Errorf("Channels should contain %d, got %v", c, caps.Channels)
					}
				}
			}
		})
	}
}

func TestRecommendSettings(t *testing.T) {
	tests := []struct {
		name     string
		caps     *Capabilities
		tier     QualityTier
		wantRate int
		wantChan int
	}{
		{
			name: "normal tier with full support",
			caps: &Capabilities{
				Formats:     []string{"S16_LE", "S24_LE"},
				SampleRates: []int{44100, 48000, 96000},
				Channels:    []int{1, 2},
			},
			tier:     QualityNormal,
			wantRate: 48000,
			wantChan: 2,
		},
		{
			name: "high tier with full support",
			caps: &Capabilities{
				Formats:     []string{"S16_LE", "S24_LE"},
				SampleRates: []int{44100, 48000, 96000},
				Channels:    []int{1, 2},
			},
			tier:     QualityHigh,
			wantRate: 48000,
			wantChan: 2,
		},
		{
			name: "low tier for voice",
			caps: &Capabilities{
				Formats:     []string{"S16_LE"},
				SampleRates: []int{8000, 16000, 48000},
				Channels:    []int{1, 2},
			},
			tier:     QualityLow,
			wantRate: 16000,
			wantChan: 1,
		},
		{
			name: "adjust to closest rate",
			caps: &Capabilities{
				Formats:     []string{"S16_LE"},
				SampleRates: []int{44100, 96000},
				Channels:    []int{2},
			},
			tier:     QualityNormal,
			wantRate: 44100, // Closest to 48000
			wantChan: 2,
		},
		{
			name: "mono only device",
			caps: &Capabilities{
				Formats:     []string{"S16_LE"},
				SampleRates: []int{48000},
				Channels:    []int{1},
			},
			tier:     QualityNormal,
			wantRate: 48000,
			wantChan: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := RecommendSettings(tt.caps, tt.tier)

			if settings.SampleRate != tt.wantRate {
				t.Errorf("SampleRate = %d, want %d", settings.SampleRate, tt.wantRate)
			}
			if settings.Channels != tt.wantChan {
				t.Errorf("Channels = %d, want %d", settings.Channels, tt.wantChan)
			}
			if settings.Codec == "" {
				t.Error("Codec should not be empty")
			}
			if settings.Bitrate == "" {
				t.Error("Bitrate should not be empty")
			}
		})
	}
}

func TestParseQualityTier(t *testing.T) {
	tests := []struct {
		input   string
		want    QualityTier
		wantErr bool
	}{
		{"low", QualityLow, false},
		{"l", QualityLow, false},
		{"LOW", QualityLow, false},
		{"normal", QualityNormal, false},
		{"n", QualityNormal, false},
		{"medium", QualityNormal, false},
		{"m", QualityNormal, false},
		{"", QualityNormal, false},
		{"high", QualityHigh, false},
		{"h", QualityHigh, false},
		{"HIGH", QualityHigh, false},
		{"invalid", "", true},
		{"ultra", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseQualityTier(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseQualityTier(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseQualityTier(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCapabilitiesSummary(t *testing.T) {
	caps := &Capabilities{
		CardNumber:  0,
		DeviceName:  "Test_Device",
		Formats:     []string{"S16_LE", "S24_LE"},
		SampleRates: []int{44100, 48000},
		Channels:    []int{1, 2},
		BitDepths:   []int{16, 24},
		MinRate:     44100,
		MaxRate:     48000,
		IsBusy:      false,
	}

	summary := caps.CapabilitiesSummary()

	if summary == "" {
		t.Error("Summary should not be empty")
	}

	// Check for expected content
	expectedParts := []string{
		"Card 0",
		"Test_Device",
		"S16_LE",
		"48000",
		"Available",
	}

	for _, part := range expectedParts {
		if !containsSubstring(summary, part) {
			t.Errorf("Summary should contain %q, got:\n%s", part, summary)
		}
	}
}

func TestCapabilitiesSummaryBusy(t *testing.T) {
	caps := &Capabilities{
		CardNumber:  1,
		DeviceName:  "Busy_Device",
		Formats:     []string{"S16_LE"},
		SampleRates: []int{48000},
		Channels:    []int{2},
		BitDepths:   []int{16},
		IsBusy:      true,
		BusyBy:      "12345",
	}

	summary := caps.CapabilitiesSummary()

	if !containsSubstring(summary, "In Use") {
		t.Errorf("Summary should indicate device is in use, got:\n%s", summary)
	}
	if !containsSubstring(summary, "12345") {
		t.Errorf("Summary should show PID, got:\n%s", summary)
	}
}

func TestSupportsRate(t *testing.T) {
	caps := &Capabilities{
		SampleRates: []int{44100, 48000},
		MinRate:     8000,
		MaxRate:     96000,
	}

	tests := []struct {
		rate int
		want bool
	}{
		{44100, true},
		{48000, true},
		{96000, true},   // In range
		{8000, true},    // In range
		{192000, false}, // Out of range
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.rate)), func(t *testing.T) {
			if got := caps.SupportsRate(tt.rate); got != tt.want {
				t.Errorf("SupportsRate(%d) = %v, want %v", tt.rate, got, tt.want)
			}
		})
	}
}

func TestSupportsChannels(t *testing.T) {
	caps := &Capabilities{
		Channels:    []int{1, 2},
		MinChannels: 1,
		MaxChannels: 8,
	}

	tests := []struct {
		channels int
		want     bool
	}{
		{1, true},
		{2, true},
		{4, true},   // In range
		{8, true},   // In range
		{16, false}, // Out of range
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.channels)), func(t *testing.T) {
			if got := caps.SupportsChannels(tt.channels); got != tt.want {
				t.Errorf("SupportsChannels(%d) = %v, want %v", tt.channels, got, tt.want)
			}
		})
	}
}

func TestSupportsFormat(t *testing.T) {
	caps := &Capabilities{
		Formats: []string{"S16_LE", "S24_LE", "S32_LE"},
	}

	tests := []struct {
		format string
		want   bool
	}{
		{"S16_LE", true},
		{"S24_LE", true},
		{"S32_LE", true},
		{"FLOAT_LE", false},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			if got := caps.SupportsFormat(tt.format); got != tt.want {
				t.Errorf("SupportsFormat(%q) = %v, want %v", tt.format, got, tt.want)
			}
		})
	}
}

func TestDeriveBitDepths(t *testing.T) {
	tests := []struct {
		formats []string
		want    []int
	}{
		{[]string{"S16_LE"}, []int{16}},
		{[]string{"S16_LE", "S24_LE"}, []int{16, 24}},
		{[]string{"S16_LE", "S24_LE", "S32_LE"}, []int{16, 24, 32}},
		{[]string{"FLOAT_LE"}, []int{32}},
		{[]string{}, []int{}},
		{[]string{"UNKNOWN"}, []int{}},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := deriveBitDepths(tt.formats)
			if len(got) != len(tt.want) {
				t.Errorf("deriveBitDepths(%v) = %v, want %v", tt.formats, got, tt.want)
				return
			}
			for i, v := range tt.want {
				if got[i] != v {
					t.Errorf("deriveBitDepths(%v)[%d] = %d, want %d", tt.formats, i, got[i], v)
				}
			}
		})
	}
}

func TestGenerateRatesInRange(t *testing.T) {
	tests := []struct {
		minRate     int
		maxRate     int
		wantInclude []int
		wantExclude []int
	}{
		{
			minRate:     8000,
			maxRate:     48000,
			wantInclude: []int{8000, 16000, 44100, 48000},
			wantExclude: []int{96000, 192000},
		},
		{
			minRate:     44100,
			maxRate:     96000,
			wantInclude: []int{44100, 48000, 96000},
			wantExclude: []int{8000, 192000},
		},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := generateRatesInRange(tt.minRate, tt.maxRate)

			for _, r := range tt.wantInclude {
				if !containsInt(got, r) {
					t.Errorf("generateRatesInRange(%d, %d) should include %d", tt.minRate, tt.maxRate, r)
				}
			}
			for _, r := range tt.wantExclude {
				if containsInt(got, r) {
					t.Errorf("generateRatesInRange(%d, %d) should not include %d", tt.minRate, tt.maxRate, r)
				}
			}
		})
	}
}

func TestHalveBitrate(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"128k", "64k"},
		{"256k", "128k"},
		{"64k", "32k"},
		{"24k", "12k"},
		{"1M", "0M"},
		{"invalid", "invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := halveBitrate(tt.input)
			if got != tt.want {
				t.Errorf("halveBitrate(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetQualityPresets(t *testing.T) {
	presets := GetQualityPresets()

	if len(presets) != 3 {
		t.Errorf("Expected 3 presets, got %d", len(presets))
	}

	// Verify all tiers exist
	tiers := []QualityTier{QualityLow, QualityNormal, QualityHigh}
	for _, tier := range tiers {
		if _, ok := presets[tier]; !ok {
			t.Errorf("Missing preset for tier %s", tier)
		}
	}

	// Verify presets have valid values
	for tier, preset := range presets {
		if preset.SampleRate <= 0 {
			t.Errorf("Tier %s has invalid SampleRate: %d", tier, preset.SampleRate)
		}
		if preset.Channels <= 0 {
			t.Errorf("Tier %s has invalid Channels: %d", tier, preset.Channels)
		}
		if preset.Codec == "" {
			t.Errorf("Tier %s has empty Codec", tier)
		}
		if preset.Bitrate == "" {
			t.Errorf("Tier %s has empty Bitrate", tier)
		}
	}
}

func TestFindClosestRate(t *testing.T) {
	tests := []struct {
		rates  []int
		target int
		want   int
	}{
		{[]int{44100, 48000}, 48000, 48000},
		{[]int{44100, 48000}, 46000, 44100}, // 44100 is closer (1900 vs 2000)
		{[]int{44100, 48000}, 44000, 44100},
		{[]int{44100, 96000}, 48000, 44100}, // 44100 is closer (3900 vs 48000)
		{[]int{8000, 96000}, 48000, 8000},   // 8000 is closer (40000 vs 48000)
		{[]int{}, 48000, 48000},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := findClosestRate(tt.rates, tt.target)
			if got != tt.want {
				t.Errorf("findClosestRate(%v, %d) = %d, want %d", tt.rates, tt.target, got, tt.want)
			}
		})
	}
}

// Helper function
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstringHelper(s, substr))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

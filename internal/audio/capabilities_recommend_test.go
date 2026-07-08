package audio

import (
	"testing"
)

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

// Test RecommendSettings edge cases
func TestRecommendSettingsEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		caps     *Capabilities
		tier     QualityTier
		wantRate int
		wantChan int
	}{
		{
			name: "empty capabilities uses defaults",
			caps: &Capabilities{
				Formats:     []string{},
				SampleRates: []int{},
				Channels:    []int{},
			},
			tier:     QualityNormal,
			wantRate: 48000, // Uses preset default
			wantChan: 2,
		},
		{
			name: "all channels higher than desired - uses minimum available",
			caps: &Capabilities{
				Formats:     []string{"S16_LE"},
				SampleRates: []int{48000},
				Channels:    []int{4, 6, 8}, // All > 2 (desired for normal)
			},
			tier:     QualityNormal,
			wantRate: 48000,
			wantChan: 4, // No supported count <= 2, so fall back to the minimum (4)
		},
		{
			name: "format fallback to S24_LE",
			caps: &Capabilities{
				Formats:     []string{"S24_LE", "S32_LE"}, // No S16_LE
				SampleRates: []int{48000},
				Channels:    []int{2},
			},
			tier:     QualityNormal,
			wantRate: 48000,
			wantChan: 2,
		},
		{
			name: "format fallback to first available",
			caps: &Capabilities{
				Formats:     []string{"FLOAT_LE"}, // No S16_LE or S24_LE
				SampleRates: []int{48000},
				Channels:    []int{2},
			},
			tier:     QualityNormal,
			wantRate: 48000,
			wantChan: 2,
		},
		{
			name: "invalid tier uses normal",
			caps: &Capabilities{
				Formats:     []string{"S16_LE"},
				SampleRates: []int{48000},
				Channels:    []int{2},
			},
			tier:     QualityTier("invalid"),
			wantRate: 48000,
			wantChan: 2,
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
		})
	}
}

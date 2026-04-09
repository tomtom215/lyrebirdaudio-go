package audio

import (
	"testing"
)

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

// Test SupportsRate boundary conditions
func TestSupportsRateBoundaries(t *testing.T) {
	tests := []struct {
		name string
		caps *Capabilities
		rate int
		want bool
	}{
		{
			name: "exact MinRate",
			caps: &Capabilities{
				SampleRates: []int{},
				MinRate:     8000,
				MaxRate:     96000,
			},
			rate: 8000,
			want: true,
		},
		{
			name: "exact MaxRate",
			caps: &Capabilities{
				SampleRates: []int{},
				MinRate:     8000,
				MaxRate:     96000,
			},
			rate: 96000,
			want: true,
		},
		{
			name: "just below MinRate",
			caps: &Capabilities{
				SampleRates: []int{},
				MinRate:     8000,
				MaxRate:     96000,
			},
			rate: 7999,
			want: false,
		},
		{
			name: "just above MaxRate",
			caps: &Capabilities{
				SampleRates: []int{},
				MinRate:     8000,
				MaxRate:     96000,
			},
			rate: 96001,
			want: false,
		},
		{
			name: "no range set and not in list",
			caps: &Capabilities{
				SampleRates: []int{44100, 48000},
				MinRate:     0,
				MaxRate:     0,
			},
			rate: 96000,
			want: false,
		},
		{
			name: "MinRate equals MaxRate (single rate range)",
			caps: &Capabilities{
				SampleRates: []int{},
				MinRate:     48000,
				MaxRate:     48000,
			},
			rate: 48000,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.caps.SupportsRate(tt.rate); got != tt.want {
				t.Errorf("SupportsRate(%d) = %v, want %v", tt.rate, got, tt.want)
			}
		})
	}
}

// Test SupportsChannels boundary conditions
func TestSupportsChannelsBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		caps     *Capabilities
		channels int
		want     bool
	}{
		{
			name: "exact MinChannels",
			caps: &Capabilities{
				Channels:    []int{},
				MinChannels: 1,
				MaxChannels: 8,
			},
			channels: 1,
			want:     true,
		},
		{
			name: "exact MaxChannels",
			caps: &Capabilities{
				Channels:    []int{},
				MinChannels: 1,
				MaxChannels: 8,
			},
			channels: 8,
			want:     true,
		},
		{
			name: "just below MinChannels",
			caps: &Capabilities{
				Channels:    []int{},
				MinChannels: 2,
				MaxChannels: 8,
			},
			channels: 1,
			want:     false,
		},
		{
			name: "just above MaxChannels",
			caps: &Capabilities{
				Channels:    []int{},
				MinChannels: 1,
				MaxChannels: 8,
			},
			channels: 9,
			want:     false,
		},
		{
			name: "no range set and not in list",
			caps: &Capabilities{
				Channels:    []int{1, 2},
				MinChannels: 0,
				MaxChannels: 0,
			},
			channels: 4,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.caps.SupportsChannels(tt.channels); got != tt.want {
				t.Errorf("SupportsChannels(%d) = %v, want %v", tt.channels, got, tt.want)
			}
		})
	}
}

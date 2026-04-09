package audio

import (
	"testing"
)

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

// Test formatIntSlice directly
func TestFormatIntSlice(t *testing.T) {
	tests := []struct {
		name  string
		slice []int
		want  string
	}{
		{"empty slice", []int{}, "(none)"},
		{"single value", []int{48000}, "48000"},
		{"multiple values", []int{44100, 48000, 96000}, "44100, 48000, 96000"},
		{"negative values", []int{-1, 0, 1}, "-1, 0, 1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatIntSlice(tt.slice)
			if got != tt.want {
				t.Errorf("formatIntSlice(%v) = %q, want %q", tt.slice, got, tt.want)
			}
		})
	}
}

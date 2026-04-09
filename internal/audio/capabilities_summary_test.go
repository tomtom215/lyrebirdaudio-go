package audio

import (
	"testing"
)

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

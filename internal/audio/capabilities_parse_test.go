package audio

import (
	"os"
	"path/filepath"
	"testing"
)

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

// Test parsePCMInfo directly for improved coverage
func TestParsePCMInfo(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantFormats []string
		wantRates   []int
		wantChans   []int
		wantErr     bool
	}{
		{
			name:        "capture stream detected",
			content:     "card: 0\nname: USB Audio\nstream: CAPTURE\n",
			wantFormats: []string{"S16_LE", "S24_LE"},
			wantRates:   []int{44100, 48000},
			wantChans:   []int{1, 2},
			wantErr:     false,
		},
		{
			name:        "playback stream only (no capture)",
			content:     "card: 0\nname: USB Audio\nstream: PLAYBACK\n",
			wantFormats: nil, // No formats set for playback-only
			wantRates:   nil,
			wantChans:   nil,
			wantErr:     false,
		},
		{
			name:        "empty file",
			content:     "",
			wantFormats: nil,
			wantRates:   nil,
			wantChans:   nil,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			path := filepath.Join(tmpDir, "info")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}

			caps := &Capabilities{}
			err := parsePCMInfo(path, caps)

			if (err != nil) != tt.wantErr {
				t.Errorf("parsePCMInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if tt.wantFormats != nil {
					for _, f := range tt.wantFormats {
						if !contains(caps.Formats, f) {
							t.Errorf("Formats should contain %s, got %v", f, caps.Formats)
						}
					}
				}
			}
		})
	}
}

func TestParsePCMInfoFileNotFound(t *testing.T) {
	caps := &Capabilities{}
	err := parsePCMInfo("/nonexistent/path/info", caps)
	if err == nil {
		t.Error("Expected error for nonexistent file, got nil")
	}
}

// Test parseStreamFile error handling
func TestParseStreamFileErrors(t *testing.T) {
	// Test file not found
	caps := &Capabilities{}
	err := parseStreamFile("/nonexistent/stream0", caps)
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

// Test parseStreamFile with no capture capabilities (no formats found)
func TestParseStreamFileNoCaptureCapabilities(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stream0")

	// Content with no format information at all
	content := `USB Audio
  Status: Stop
  Interface 1
    Altset 1
    Endpoint: 1 OUT (ASYNC)
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	caps := &Capabilities{}
	err := parseStreamFile(path, caps)

	// Should return error for no capture capabilities (no formats found)
	if err == nil {
		t.Error("Expected error for no capture capabilities")
	}
}

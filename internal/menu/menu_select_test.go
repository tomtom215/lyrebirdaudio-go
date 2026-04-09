package menu

import (
	"bytes"
	"strings"
	"testing"
)

func TestSelect(t *testing.T) {
	tests := []struct {
		input   string
		options []string
		want    int
	}{
		{"1\n", []string{"a", "b", "c"}, 0},
		{"2\n", []string{"a", "b", "c"}, 1},
		{"3\n", []string{"a", "b", "c"}, 2},
		{"0\n", []string{"a", "b", "c"}, -1},   // Out of range
		{"4\n", []string{"a", "b", "c"}, -1},   // Out of range
		{"abc\n", []string{"a", "b", "c"}, -1}, // Invalid
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			input := strings.NewReader(tt.input)
			output := &bytes.Buffer{}

			got := Select(input, output, "Choose:", tt.options)
			if got != tt.want {
				t.Errorf("Select() with input %q = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestSelectWithScanner(t *testing.T) {
	tests := []struct {
		input   string
		options []string
		want    int
	}{
		{"1\n", []string{"a", "b", "c"}, 0},
		{"2\n", []string{"a", "b", "c"}, 1},
		{"3\n", []string{"a", "b", "c"}, 2},
		{"0\n", []string{"a", "b", "c"}, -1},   // Out of range
		{"4\n", []string{"a", "b", "c"}, -1},   // Out of range
		{"abc\n", []string{"a", "b", "c"}, -1}, // Invalid
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			input := strings.NewReader(tt.input)
			output := &bytes.Buffer{}

			got := selectWithScanner(input, output, "Choose:", tt.options)
			if got != tt.want {
				t.Errorf("selectWithScanner() with input %q = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestSelectEOF(t *testing.T) {
	input := strings.NewReader("")
	output := &bytes.Buffer{}

	got := selectWithScanner(input, output, "Choose:", []string{"a", "b"})
	if got != -1 {
		t.Errorf("selectWithScanner() should return -1 on EOF, got %d", got)
	}
}

func TestSelectOutput(t *testing.T) {
	input := strings.NewReader("1\n")
	output := &bytes.Buffer{}

	selectWithScanner(input, output, "Choose an option:", []string{"First", "Second", "Third"})

	outputStr := output.String()

	// Check prompt is displayed
	if !strings.Contains(outputStr, "Choose an option:") {
		t.Error("Output should contain prompt")
	}

	// Check options are displayed
	if !strings.Contains(outputStr, "1. First") {
		t.Error("Output should contain first option")
	}
	if !strings.Contains(outputStr, "2. Second") {
		t.Error("Output should contain second option")
	}
	if !strings.Contains(outputStr, "3. Third") {
		t.Error("Output should contain third option")
	}
}

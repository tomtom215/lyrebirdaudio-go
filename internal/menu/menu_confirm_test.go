package menu

import (
	"bytes"
	"strings"
	"testing"
)

func TestConfirm(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"y\n", true},
		{"Y\n", true},
		{"yes\n", true},
		{"YES\n", true},
		{"n\n", false},
		{"N\n", false},
		{"no\n", false},
		{"\n", false},
		{"maybe\n", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			input := strings.NewReader(tt.input)
			output := &bytes.Buffer{}

			got := Confirm(input, output, "Test?")
			if got != tt.want {
				t.Errorf("Confirm() with input %q = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestConfirmWithScanner(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"y\n", true},
		{"Y\n", true},
		{"yes\n", true},
		{"YES\n", true},
		{"n\n", false},
		{"N\n", false},
		{"no\n", false},
		{"\n", false},
		{"maybe\n", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			input := strings.NewReader(tt.input)
			output := &bytes.Buffer{}

			got := confirmWithScanner(input, output, "Test?")
			if got != tt.want {
				t.Errorf("confirmWithScanner() with input %q = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestConfirmEOF(t *testing.T) {
	input := strings.NewReader("")
	output := &bytes.Buffer{}

	got := confirmWithScanner(input, output, "Test?")
	if got {
		t.Error("confirmWithScanner() should return false on EOF")
	}
}

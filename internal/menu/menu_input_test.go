package menu

import (
	"bytes"
	"strings"
	"testing"
)

func TestInput(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		prompt string
		want   string
	}{
		{"simple", "hello\n", "Enter value", "hello"},
		{"with spaces", "hello world\n", "Enter value", "hello world"},
		{"trimmed", "  hello  \n", "Enter value", "hello"},
		{"empty", "\n", "Enter value", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := strings.NewReader(tt.input)
			output := &bytes.Buffer{}

			got := Input(input, output, tt.prompt)
			if got != tt.want {
				t.Errorf("Input() with input %q = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestInputWithScanner(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		prompt string
		want   string
	}{
		{"simple", "hello\n", "Enter value", "hello"},
		{"with spaces", "hello world\n", "Enter value", "hello world"},
		{"trimmed", "  hello  \n", "Enter value", "hello"},
		{"empty", "\n", "Enter value", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := strings.NewReader(tt.input)
			output := &bytes.Buffer{}

			got := inputWithScanner(input, output, tt.prompt)
			if got != tt.want {
				t.Errorf("inputWithScanner() with input %q = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestInputEOF(t *testing.T) {
	input := strings.NewReader("")
	output := &bytes.Buffer{}

	got := inputWithScanner(input, output, "Enter value")
	if got != "" {
		t.Errorf("inputWithScanner() should return empty string on EOF, got %q", got)
	}
}

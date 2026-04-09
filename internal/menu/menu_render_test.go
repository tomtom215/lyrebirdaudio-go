package menu

import (
	"bytes"
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	output := &bytes.Buffer{}

	m := New("Test Menu",
		WithOutput(output),
		WithClearScreen(false),
	)
	m.AddItem(MenuItem{Key: "1", Label: "Option One"})
	m.AddSeparator()
	m.AddItem(MenuItem{Key: "2", Label: "Option Two"})
	m.Footer = "Test Footer"

	m.render()

	outputStr := output.String()

	// Check for box characters
	if !strings.Contains(outputStr, "╔") {
		t.Error("Output should contain box border")
	}

	// Check for title
	if !strings.Contains(outputStr, "Test Menu") {
		t.Error("Output should contain title")
	}

	// Check for items
	if !strings.Contains(outputStr, "Option One") {
		t.Error("Output should contain first option")
	}
	if !strings.Contains(outputStr, "Option Two") {
		t.Error("Output should contain second option")
	}

	// Check for footer
	if !strings.Contains(outputStr, "Test Footer") {
		t.Error("Output should contain footer")
	}
}

func TestCenterText(t *testing.T) {
	tests := []struct {
		text  string
		width int
		check func(string) bool
	}{
		{
			text:  "test",
			width: 10,
			check: func(s string) bool { return len(s) == 10 },
		},
		{
			text:  "long text that is longer than width",
			width: 10,
			check: func(s string) bool { return s == "long text that is longer than width" },
		},
		{
			text:  "",
			width: 10,
			check: func(s string) bool { return len(s) == 10 },
		},
	}

	for _, tt := range tests {
		result := centerText(tt.text, tt.width)
		if !tt.check(result) {
			t.Errorf("centerText(%q, %d) = %q, unexpected result", tt.text, tt.width, result)
		}
	}
}

func TestClearScreen(t *testing.T) {
	output := &bytes.Buffer{}

	clearScreen(output)

	// Check that ANSI escape sequence was written
	if !strings.Contains(output.String(), "\033[2J") {
		t.Error("clearScreen should write ANSI clear sequence")
	}
	if !strings.Contains(output.String(), "\033[H") {
		t.Error("clearScreen should write ANSI home sequence")
	}
}

func TestWaitForKey(t *testing.T) {
	input := strings.NewReader("\n")
	output := &bytes.Buffer{}

	WaitForKey(input, output, "")

	if !strings.Contains(output.String(), "Press Enter to continue...") {
		t.Error("WaitForKey should show default prompt")
	}
}

func TestWaitForKeyCustomPrompt(t *testing.T) {
	input := strings.NewReader("\n")
	output := &bytes.Buffer{}

	WaitForKey(input, output, "Custom prompt: ")

	if !strings.Contains(output.String(), "Custom prompt: ") {
		t.Error("WaitForKey should show custom prompt")
	}
}

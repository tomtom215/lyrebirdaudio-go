package menu

import (
	"bytes"
	"strings"
	"testing"
)

func TestActionError(t *testing.T) {
	input := strings.NewReader("1\n\n0\n") // Select action, press enter, exit
	output := &bytes.Buffer{}

	m := New("Test",
		WithInput(input),
		WithOutput(output),
		WithClearScreen(false),
	)

	m.AddItem(MenuItem{
		Key:   "1",
		Label: "Error Action",
		Action: func() error {
			return &testError{msg: "test error"}
		},
	})
	m.AddItem(MenuItem{Key: "0", Label: "Exit"})

	err := m.displayWithScanner()
	if err != nil {
		t.Fatalf("displayWithScanner() error: %v", err)
	}

	// Check that error was displayed
	if !strings.Contains(output.String(), "Error:") {
		t.Error("Output should contain error message")
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// TestRunCommand exercises RunCommand with a real but harmless command.
func TestRunCommand(t *testing.T) {
	output := &bytes.Buffer{}

	// echo is always available and produces deterministic output.
	err := RunCommand(output, "echo", "hello")
	if err != nil {
		t.Fatalf("RunCommand(echo hello) error: %v", err)
	}
	if !strings.Contains(output.String(), "hello") {
		t.Errorf("RunCommand(echo hello) output = %q, want 'hello'", output.String())
	}
}

// TestRunCommandFailure verifies that RunCommand propagates a non-zero exit code.
func TestRunCommandFailure(t *testing.T) {
	output := &bytes.Buffer{}

	err := RunCommand(output, "false") // POSIX "false" always exits non-zero
	if err == nil {
		t.Error("RunCommand(false) expected non-nil error for non-zero exit")
	}
}

// TestRunCommandNotFound verifies that RunCommand returns an error for missing binaries.
func TestRunCommandNotFound(t *testing.T) {
	output := &bytes.Buffer{}

	err := RunCommand(output, "this-binary-should-never-exist-lyrebird-test")
	if err == nil {
		t.Error("RunCommand(nonexistent) expected non-nil error")
	}
}

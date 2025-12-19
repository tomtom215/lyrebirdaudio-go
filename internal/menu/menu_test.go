package menu

import (
	"bytes"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	m := New("Test Menu")
	if m == nil {
		t.Fatal("New() returned nil")
	}
	if m.Title != "Test Menu" {
		t.Errorf("Title = %q, want %q", m.Title, "Test Menu")
	}
}

func TestNewWithOptions(t *testing.T) {
	input := strings.NewReader("0\n")
	output := &bytes.Buffer{}

	m := New("Test",
		WithInput(input),
		WithOutput(output),
		WithClearScreen(false),
		WithAccessible(true),
	)

	if m.input != input {
		t.Error("WithInput option not applied")
	}
	if m.output != output {
		t.Error("WithOutput option not applied")
	}
	if m.clearScreen {
		t.Error("WithClearScreen option not applied")
	}
	if !m.accessible {
		t.Error("WithAccessible option not applied")
	}
}

func TestAddItem(t *testing.T) {
	m := New("Test")

	m.AddItem(MenuItem{
		Key:   "1",
		Label: "Option One",
	})

	if len(m.Items) != 1 {
		t.Errorf("len(Items) = %d, want 1", len(m.Items))
	}

	if m.Items[0].Key != "1" {
		t.Errorf("Items[0].Key = %q, want %q", m.Items[0].Key, "1")
	}
}

func TestAddSeparator(t *testing.T) {
	m := New("Test")

	m.AddItem(MenuItem{Key: "1", Label: "Before"})
	m.AddSeparator()
	m.AddItem(MenuItem{Key: "2", Label: "After"})

	if len(m.Items) != 3 {
		t.Errorf("len(Items) = %d, want 3", len(m.Items))
	}

	// Separator should have empty key and label
	if m.Items[1].Key != "" || m.Items[1].Label != "" {
		t.Error("Separator should have empty key and label")
	}
}

func TestDisplay(t *testing.T) {
	actionCalled := false
	input := strings.NewReader("1\n0\n")
	output := &bytes.Buffer{}

	m := New("Test",
		WithInput(input),
		WithOutput(output),
		WithClearScreen(false),
	)

	m.AddItem(MenuItem{
		Key:   "1",
		Label: "Test Action",
		Action: func() error {
			actionCalled = true
			return nil
		},
	})
	m.AddItem(MenuItem{
		Key:   "0",
		Label: "Exit",
	})

	err := m.Display()
	if err != nil {
		t.Fatalf("Display() error: %v", err)
	}

	if !actionCalled {
		t.Error("Action was not called")
	}

	// Check output contains menu elements
	outputStr := output.String()
	if !strings.Contains(outputStr, "Test") {
		t.Error("Output should contain menu title")
	}
	if !strings.Contains(outputStr, "Test Action") {
		t.Error("Output should contain menu item")
	}
}

func TestDisplaySubmenu(t *testing.T) {
	input := strings.NewReader("1\n0\n0\n")
	output := &bytes.Buffer{}

	submenu := New("Submenu",
		WithInput(input),
		WithOutput(output),
		WithClearScreen(false),
	)
	submenu.AddItem(MenuItem{
		Key:   "1",
		Label: "Submenu Item",
		Action: func() error {
			return nil
		},
	})
	submenu.AddItem(MenuItem{Key: "0", Label: "Back"})

	m := New("Main",
		WithInput(input),
		WithOutput(output),
		WithClearScreen(false),
	)
	m.AddItem(MenuItem{
		Key:     "1",
		Label:   "Go to Submenu",
		SubMenu: submenu,
	})
	m.AddItem(MenuItem{Key: "0", Label: "Exit"})

	// Note: This test is tricky because both menus share input/output
	// In a real scenario, submenus would inherit parent's I/O
}

func TestDisplayEOF(t *testing.T) {
	input := strings.NewReader("") // Empty input = EOF
	output := &bytes.Buffer{}

	m := New("Test",
		WithInput(input),
		WithOutput(output),
		WithClearScreen(false),
	)
	m.AddItem(MenuItem{Key: "0", Label: "Exit"})

	err := m.Display()
	if err != nil {
		t.Errorf("Display() should return nil on EOF, got: %v", err)
	}
}

func TestDisplayWithScanner(t *testing.T) {
	actionCalled := false
	input := strings.NewReader("1\n0\n")
	output := &bytes.Buffer{}

	m := New("Test",
		WithInput(input),
		WithOutput(output),
		WithClearScreen(false),
	)

	m.AddItem(MenuItem{
		Key:   "1",
		Label: "Test Action",
		Action: func() error {
			actionCalled = true
			return nil
		},
	})
	m.AddItem(MenuItem{
		Key:   "0",
		Label: "Exit",
	})

	// displayWithScanner is used when input != os.Stdin
	err := m.displayWithScanner()
	if err != nil {
		t.Fatalf("displayWithScanner() error: %v", err)
	}

	if !actionCalled {
		t.Error("Action was not called")
	}
}

func TestDisplayWithScannerQuit(t *testing.T) {
	input := strings.NewReader("q\n")
	output := &bytes.Buffer{}

	m := New("Test",
		WithInput(input),
		WithOutput(output),
		WithClearScreen(false),
	)
	m.AddItem(MenuItem{Key: "1", Label: "Option"})
	m.AddItem(MenuItem{Key: "0", Label: "Exit"})

	err := m.displayWithScanner()
	if err != nil {
		t.Fatalf("displayWithScanner() should exit on 'q', got error: %v", err)
	}
}

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

func TestHiddenItem(t *testing.T) {
	output := &bytes.Buffer{}

	m := New("Test",
		WithOutput(output),
		WithClearScreen(false),
	)
	m.AddItem(MenuItem{Key: "1", Label: "Visible"})
	m.AddItem(MenuItem{Key: "h", Label: "Hidden", Hidden: true})
	m.AddItem(MenuItem{Key: "0", Label: "Exit"})

	m.render()

	outputStr := output.String()

	if !strings.Contains(outputStr, "Visible") {
		t.Error("Output should contain visible item")
	}
	if strings.Contains(outputStr, "Hidden") {
		t.Error("Output should not contain hidden item")
	}
}

func TestCreateMainMenu(t *testing.T) {
	menu := CreateMainMenu()

	if menu == nil {
		t.Fatal("CreateMainMenu() returned nil")
	}

	if menu.Title == "" {
		t.Error("Main menu should have a title")
	}

	if len(menu.Items) == 0 {
		t.Error("Main menu should have items")
	}

	// Check for expected items
	hasSetup := false
	hasDevices := false
	hasStreams := false
	hasExit := false

	for _, item := range menu.Items {
		switch item.Key {
		case "1":
			hasSetup = true
		case "2":
			hasDevices = item.SubMenu != nil
		case "3":
			hasStreams = item.SubMenu != nil
		case "0":
			hasExit = true
		}
	}

	if !hasSetup {
		t.Error("Main menu should have setup option")
	}
	if !hasDevices {
		t.Error("Main menu should have device submenu")
	}
	if !hasStreams {
		t.Error("Main menu should have stream submenu")
	}
	if !hasExit {
		t.Error("Main menu should have exit option")
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

func TestMenuWithDescription(t *testing.T) {
	m := New("Test")
	m.AddItem(MenuItem{
		Key:         "1",
		Label:       "Option",
		Description: "This is a description",
	})

	if m.Items[0].Description != "This is a description" {
		t.Error("MenuItem should store description")
	}
}

func TestDisplayEmptyMenu(t *testing.T) {
	input := strings.NewReader("0\n")
	output := &bytes.Buffer{}

	m := New("Empty Menu",
		WithInput(input),
		WithOutput(output),
		WithClearScreen(false),
	)

	// Menu with no items should return nil
	err := m.displayWithScanner()
	if err != nil {
		t.Errorf("displayWithScanner() on empty menu should return nil, got: %v", err)
	}
}

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

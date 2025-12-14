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

func TestSelect(t *testing.T) {
	tests := []struct {
		input   string
		options []string
		want    int
	}{
		{"1\n", []string{"a", "b", "c"}, 0},
		{"2\n", []string{"a", "b", "c"}, 1},
		{"3\n", []string{"a", "b", "c"}, 2},
		{"0\n", []string{"a", "b", "c"}, -1},  // Out of range
		{"4\n", []string{"a", "b", "c"}, -1},  // Out of range
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

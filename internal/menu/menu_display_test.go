package menu

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

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

// TestDisplayHuhPathEmptyOptions exercises the len(options)==0 early-return in Display().
// When every item is hidden or a separator, the huh path returns nil immediately
// without ever calling huh.NewForm (which would require a real TTY).
func TestDisplayHuhPathEmptyOptions(t *testing.T) {
	// Force the interactive (huh) branch: m.input is os.Stdin and the terminal
	// check reports a TTY, so Display() will NOT call displayWithScanner().
	// All items are hidden/separators, so options is empty → returns nil
	// immediately, before huh.NewForm is ever reached.
	orig := terminalCheck
	terminalCheck = func(uintptr) bool { return true }
	defer func() { terminalCheck = orig }()

	output := &bytes.Buffer{}
	m := &Menu{
		Title:       "HuhPathTest",
		input:       os.Stdin,
		output:      output,
		clearScreen: false,
	}
	m.AddItem(MenuItem{Key: "h", Label: "Hidden", Hidden: true})
	m.AddSeparator()

	err := m.Display()
	if err != nil {
		t.Errorf("Display() with all-hidden items should return nil, got: %v", err)
	}
}

// TestDisplayAllHiddenScanner exercises displayWithScanner with hidden-only items.
func TestDisplayAllHiddenScanner(t *testing.T) {
	input := strings.NewReader("")
	m := New("All Hidden",
		WithInput(input),
		WithOutput(&bytes.Buffer{}),
		WithClearScreen(false),
	)
	m.AddItem(MenuItem{Key: "h", Label: "Hidden", Hidden: true})
	m.AddSeparator()

	if err := m.displayWithScanner(); err != nil {
		t.Errorf("displayWithScanner() with hidden-only menu: %v", err)
	}
}

// TestDisplayWithScannerSubMenu exercises the submenu path in displayWithScanner.
func TestDisplayWithScannerSubMenu(t *testing.T) {
	// Input: choose item "1" (which has a submenu), then exit submenu with "0", then exit main with "0".
	input := strings.NewReader("1\n0\n0\n")
	output := &bytes.Buffer{}

	submenu := New("Sub",
		WithInput(input),
		WithOutput(output),
		WithClearScreen(false),
	)
	submenu.AddItem(MenuItem{Key: "0", Label: "Back"})

	m := New("Main",
		WithInput(input),
		WithOutput(output),
		WithClearScreen(false),
	)
	m.AddItem(MenuItem{
		Key:     "1",
		Label:   "Open Sub",
		SubMenu: submenu,
	})
	m.AddItem(MenuItem{Key: "0", Label: "Exit"})

	if err := m.displayWithScanner(); err != nil {
		t.Errorf("displayWithScanner() with submenu: %v", err)
	}
}

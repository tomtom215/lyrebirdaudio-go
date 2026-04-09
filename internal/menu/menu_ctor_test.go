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

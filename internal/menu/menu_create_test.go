package menu

import (
	"testing"
)

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

// TestCreateDeviceMenuStructure verifies createDeviceMenu returns expected items.
func TestCreateDeviceMenuStructure(t *testing.T) {
	m := createDeviceMenu()
	if m == nil {
		t.Fatal("createDeviceMenu() returned nil")
	}
	if m.Title == "" {
		t.Error("createDeviceMenu() title should not be empty")
	}

	// Verify at least items 1–4 and item "0" (Back) exist.
	keys := make(map[string]bool)
	for _, item := range m.Items {
		if item.Key != "" {
			keys[item.Key] = true
		}
	}
	for _, k := range []string{"1", "2", "3", "4", "0"} {
		if !keys[k] {
			t.Errorf("createDeviceMenu() missing item key %q", k)
		}
	}
}

// TestCreateStreamMenuStructure verifies createStreamMenu returns expected items.
func TestCreateStreamMenuStructure(t *testing.T) {
	m := createStreamMenu()
	if m == nil {
		t.Fatal("createStreamMenu() returned nil")
	}
	if m.Title == "" {
		t.Error("createStreamMenu() title should not be empty")
	}
	keys := make(map[string]bool)
	for _, item := range m.Items {
		if item.Key != "" {
			keys[item.Key] = true
		}
	}
	for _, k := range []string{"1", "2", "3", "4", "5", "0"} {
		if !keys[k] {
			t.Errorf("createStreamMenu() missing item key %q", k)
		}
	}
}

// TestCreateDiagnosticsMenuStructure verifies createDiagnosticsMenu returns expected items.
func TestCreateDiagnosticsMenuStructure(t *testing.T) {
	m := createDiagnosticsMenu()
	if m == nil {
		t.Fatal("createDiagnosticsMenu() returned nil")
	}
	keys := make(map[string]bool)
	for _, item := range m.Items {
		if item.Key != "" {
			keys[item.Key] = true
		}
	}
	for _, k := range []string{"1", "2", "3", "4", "0"} {
		if !keys[k] {
			t.Errorf("createDiagnosticsMenu() missing item key %q", k)
		}
	}
}

// TestCreateConfigMenuStructure verifies createConfigMenu returns expected items.
func TestCreateConfigMenuStructure(t *testing.T) {
	m := createConfigMenu()
	if m == nil {
		t.Fatal("createConfigMenu() returned nil")
	}
	keys := make(map[string]bool)
	for _, item := range m.Items {
		if item.Key != "" {
			keys[item.Key] = true
		}
	}
	for _, k := range []string{"1", "2", "3", "4", "0"} {
		if !keys[k] {
			t.Errorf("createConfigMenu() missing item key %q", k)
		}
	}
}

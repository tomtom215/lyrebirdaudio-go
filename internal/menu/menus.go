// SPDX-License-Identifier: MIT

package menu

import (
	"os"
	"strings"
)

// CreateMainMenu creates the main LyreBird menu.
func CreateMainMenu() *Menu {
	menu := New("LyreBirdAudio Management Menu")

	// 1. Quick Setup
	menu.AddItem(MenuItem{
		Key:   "1",
		Label: "Quick Setup Wizard",
		Action: func() error {
			return RunCommand(os.Stdout, "lyrebird", "setup", "--auto")
		},
	})

	// 2. Device Management submenu
	deviceMenu := createDeviceMenu()
	menu.AddItem(MenuItem{
		Key:     "2",
		Label:   "Device Management",
		SubMenu: deviceMenu,
	})

	// 3. Stream Control submenu
	streamMenu := createStreamMenu()
	menu.AddItem(MenuItem{
		Key:     "3",
		Label:   "Stream Control",
		SubMenu: streamMenu,
	})

	// 4. Diagnostics submenu
	diagMenu := createDiagnosticsMenu()
	menu.AddItem(MenuItem{
		Key:     "4",
		Label:   "System Diagnostics",
		SubMenu: diagMenu,
	})

	// 5. Configuration submenu
	configMenu := createConfigMenu()
	menu.AddItem(MenuItem{
		Key:     "5",
		Label:   "Configuration",
		SubMenu: configMenu,
	})

	// 6. Updates
	menu.AddItem(MenuItem{
		Key:   "6",
		Label: "Check for Updates",
		Action: func() error {
			return RunCommand(os.Stdout, "lyrebird", "update", "--check")
		},
	})

	menu.AddSeparator()

	// 7. About
	menu.AddItem(MenuItem{
		Key:   "7",
		Label: "About / Version",
		Action: func() error {
			return RunCommand(os.Stdout, "lyrebird", "version")
		},
	})

	// 0. Exit
	menu.AddItem(MenuItem{
		Key:    "0",
		Label:  "Exit",
		Action: nil, // nil action exits menu
	})

	return menu
}

// createDeviceMenu creates the device management submenu.
func createDeviceMenu() *Menu {
	menu := New("Device Management")

	menu.AddItem(MenuItem{
		Key:   "1",
		Label: "List Detected Devices",
		Action: func() error {
			err := RunCommand(os.Stdout, "lyrebird", "devices")
			WaitForKey(os.Stdin, os.Stdout, "")
			return err
		},
	})

	menu.AddItem(MenuItem{
		Key:   "2",
		Label: "Detect Device Capabilities",
		Action: func() error {
			err := RunCommand(os.Stdout, "lyrebird", "detect")
			WaitForKey(os.Stdin, os.Stdout, "")
			return err
		},
	})

	menu.AddItem(MenuItem{
		Key:   "3",
		Label: "Create udev Rules",
		Action: func() error {
			if !Confirm(os.Stdin, os.Stdout, "This requires root privileges. Continue?") {
				return nil
			}
			err := RunCommand(os.Stdout, "sudo", "lyrebird", "usb-map")
			WaitForKey(os.Stdin, os.Stdout, "")
			return err
		},
	})

	menu.AddItem(MenuItem{
		Key:   "4",
		Label: "Preview udev Rules (dry-run)",
		Action: func() error {
			err := RunCommand(os.Stdout, "lyrebird", "usb-map", "--dry-run")
			WaitForKey(os.Stdin, os.Stdout, "")
			return err
		},
	})

	menu.AddSeparator()

	menu.AddItem(MenuItem{
		Key:   "0",
		Label: "Back to Main Menu",
	})

	return menu
}

// createStreamMenu creates the stream control submenu.
func createStreamMenu() *Menu {
	menu := New("Stream Control")

	menu.AddItem(MenuItem{
		Key:   "1",
		Label: "Show Stream Status",
		Action: func() error {
			err := RunCommand(os.Stdout, "lyrebird", "status")
			WaitForKey(os.Stdin, os.Stdout, "")
			return err
		},
	})

	menu.AddItem(MenuItem{
		Key:   "2",
		Label: "Start Streaming Service",
		Action: func() error {
			return RunCommand(os.Stdout, "sudo", "systemctl", "start", "lyrebird-stream")
		},
	})

	menu.AddItem(MenuItem{
		Key:   "3",
		Label: "Stop Streaming Service",
		Action: func() error {
			return RunCommand(os.Stdout, "sudo", "systemctl", "stop", "lyrebird-stream")
		},
	})

	menu.AddItem(MenuItem{
		Key:   "4",
		Label: "Restart Streaming Service",
		Action: func() error {
			return RunCommand(os.Stdout, "sudo", "systemctl", "restart", "lyrebird-stream")
		},
	})

	menu.AddItem(MenuItem{
		Key:   "5",
		Label: "View Service Logs",
		Action: func() error {
			err := RunCommand(os.Stdout, "sudo", "journalctl", "-u", "lyrebird-stream", "-n", "50", "--no-pager")
			WaitForKey(os.Stdin, os.Stdout, "")
			return err
		},
	})

	menu.AddSeparator()

	menu.AddItem(MenuItem{
		Key:   "0",
		Label: "Back to Main Menu",
	})

	return menu
}

// createDiagnosticsMenu creates the diagnostics submenu.
func createDiagnosticsMenu() *Menu {
	menu := New("System Diagnostics")

	menu.AddItem(MenuItem{
		Key:   "1",
		Label: "Quick Check",
		Action: func() error {
			err := RunCommand(os.Stdout, "lyrebird", "check-system")
			WaitForKey(os.Stdin, os.Stdout, "")
			return err
		},
	})

	menu.AddItem(MenuItem{
		Key:   "2",
		Label: "Full Diagnostics",
		Action: func() error {
			err := RunCommand(os.Stdout, "lyrebird", "diagnose")
			WaitForKey(os.Stdin, os.Stdout, "")
			return err
		},
	})

	menu.AddItem(MenuItem{
		Key:   "3",
		Label: "Test Configuration",
		Action: func() error {
			err := RunCommand(os.Stdout, "lyrebird", "test")
			WaitForKey(os.Stdin, os.Stdout, "")
			return err
		},
	})

	menu.AddItem(MenuItem{
		Key:   "4",
		Label: "View MediaMTX Logs",
		Action: func() error {
			err := RunCommand(os.Stdout, "sudo", "journalctl", "-u", "mediamtx", "-n", "50", "--no-pager")
			WaitForKey(os.Stdin, os.Stdout, "")
			return err
		},
	})

	menu.AddSeparator()

	menu.AddItem(MenuItem{
		Key:   "0",
		Label: "Back to Main Menu",
	})

	return menu
}

// createConfigMenu creates the configuration submenu.
func createConfigMenu() *Menu {
	menu := New("Configuration")

	menu.AddItem(MenuItem{
		Key:   "1",
		Label: "Validate Configuration",
		Action: func() error {
			err := RunCommand(os.Stdout, "lyrebird", "validate")
			WaitForKey(os.Stdin, os.Stdout, "")
			return err
		},
	})

	menu.AddItem(MenuItem{
		Key:   "2",
		Label: "Migrate from Bash Config",
		Action: func() error {
			bashPath := Input(os.Stdin, os.Stdout, "Enter path to bash config file")
			if bashPath == "" {
				return nil
			}
			err := RunCommand(os.Stdout, "lyrebird", "migrate", "--from="+bashPath)
			WaitForKey(os.Stdin, os.Stdout, "")
			return err
		},
	})

	menu.AddItem(MenuItem{
		Key:   "3",
		Label: "Install MediaMTX",
		Action: func() error {
			if !Confirm(os.Stdin, os.Stdout, "Install MediaMTX RTSP server?") {
				return nil
			}
			return RunCommand(os.Stdout, "sudo", "lyrebird", "install-mediamtx")
		},
	})

	menu.AddItem(MenuItem{
		Key:   "4",
		Label: "Edit Config File",
		Action: func() error {
			// $EDITOR may be multi-word (e.g. "code --wait"), so split it into
			// the editor binary plus any leading arguments. Fall back to nano
			// when the variable is unset or blank.
			fields := strings.Fields(os.Getenv("EDITOR"))
			if len(fields) == 0 {
				fields = []string{"nano"}
			}
			// Run via sudo and wire the editor to the real terminal so it is
			// actually usable (see RunInteractiveCommand).
			args := append(fields, "/etc/lyrebird/config.yaml")
			return RunInteractiveCommand("sudo", args...)
		},
	})

	menu.AddSeparator()

	menu.AddItem(MenuItem{
		Key:   "0",
		Label: "Back to Main Menu",
	})

	return menu
}

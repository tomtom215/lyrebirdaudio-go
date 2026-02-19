// SPDX-License-Identifier: MIT

// Package menu provides an interactive terminal menu system using charmbracelet/huh.
//
// This implements a TUI menu matching the functionality of the
// bash lyrebird-orchestrator.sh script, allowing users to navigate
// through options without memorizing CLI commands.
//
// Reference: lyrebird-orchestrator.sh
package menu

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/huh"
)

// MenuItem represents a single menu option.
type MenuItem struct {
	Key         string       // Key identifier (e.g., "1", "q")
	Label       string       // Display label
	Description string       // Optional description
	Action      func() error // Action to execute
	SubMenu     *Menu        // Optional submenu
	Hidden      bool         // If true, not displayed but still accessible
}

// Menu represents a menu with multiple items.
type Menu struct {
	Title       string
	Items       []MenuItem
	Footer      string
	input       io.Reader
	output      io.Writer
	clearScreen bool
	accessible  bool // Enable accessible mode for screen readers
}

// Option is a functional option for configuring menus.
type Option func(*Menu)

// WithInput sets the input reader (for testing).
func WithInput(r io.Reader) Option {
	return func(m *Menu) {
		m.input = r
	}
}

// WithOutput sets the output writer (for testing).
func WithOutput(w io.Writer) Option {
	return func(m *Menu) {
		m.output = w
	}
}

// WithClearScreen enables screen clearing between displays.
func WithClearScreen(clear bool) Option {
	return func(m *Menu) {
		m.clearScreen = clear
	}
}

// WithAccessible enables accessible mode for screen readers.
func WithAccessible(accessible bool) Option {
	return func(m *Menu) {
		m.accessible = accessible
	}
}

// New creates a new menu.
func New(title string, opts ...Option) *Menu {
	m := &Menu{
		Title:       title,
		input:       os.Stdin,
		output:      os.Stdout,
		clearScreen: true,
		accessible:  false,
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// AddItem adds an item to the menu.
func (m *Menu) AddItem(item MenuItem) {
	m.Items = append(m.Items, item)
}

// AddSeparator adds a visual separator.
func (m *Menu) AddSeparator() {
	m.Items = append(m.Items, MenuItem{Key: "", Label: ""})
}

// Display shows the menu and waits for user input.
// Returns when the user selects an action or exits.
func (m *Menu) Display() error {
	// Check if we're in test mode (non-TTY input)
	if m.input != os.Stdin {
		return m.displayWithScanner()
	}

	for {
		if m.clearScreen {
			clearScreen(m.output)
		}

		// Build options for huh.Select
		var options []huh.Option[string]
		for _, item := range m.Items {
			if item.Key == "" && item.Label == "" {
				// Skip separators in huh (they don't support separators directly)
				continue
			}
			if item.Hidden {
				continue
			}
			label := fmt.Sprintf("%s. %s", item.Key, item.Label)
			options = append(options, huh.NewOption(label, item.Key))
		}

		if len(options) == 0 {
			return nil
		}

		var choice string
		selector := huh.NewSelect[string]().
			Title(m.Title).
			Options(options...).
			Value(&choice)

		form := huh.NewForm(huh.NewGroup(selector)).
			WithAccessible(m.accessible)

		err := form.Run()
		if err != nil {
			// Handle Ctrl+C or other interrupts
			if err == huh.ErrUserAborted {
				return nil
			}
			return err
		}

		// Check for exit keys
		if choice == "0" || choice == "q" || choice == "Q" {
			return nil
		}

		// Find and execute the matching item
		for _, item := range m.Items {
			if item.Key == choice {
				if item.SubMenu != nil {
					// Copy options to submenu
					item.SubMenu.accessible = m.accessible
					if err := item.SubMenu.Display(); err != nil {
						return err
					}
				} else if item.Action != nil {
					if err := item.Action(); err != nil {
						_, _ = fmt.Fprintf(m.output, "\nError: %v\n", err)
						WaitForKey(m.input, m.output, "")
					}
				}
				break
			}
		}
	}
}

// displayWithScanner provides a fallback for non-TTY input (testing).
func (m *Menu) displayWithScanner() error {
	scanner := bufio.NewScanner(m.input)

	for {
		if m.clearScreen {
			clearScreen(m.output)
		}

		m.render()

		_, _ = fmt.Fprint(m.output, "\nSelect option: ")

		if !scanner.Scan() {
			return nil // EOF or input closed
		}

		choice := strings.TrimSpace(scanner.Text())
		if choice == "" {
			continue
		}

		// Find matching item
		for _, item := range m.Items {
			if item.Key == choice {
				if item.SubMenu != nil {
					if err := item.SubMenu.Display(); err != nil {
						return err
					}
				} else if item.Action != nil {
					if err := item.Action(); err != nil {
						_, _ = fmt.Fprintf(m.output, "\nError: %v\n", err)
						_, _ = fmt.Fprint(m.output, "Press Enter to continue...")
						scanner.Scan()
					}
				}
				break
			}
		}

		// Check for exit keys
		if choice == "0" || choice == "q" || choice == "Q" {
			return nil
		}
	}
}

// render draws the menu using box characters (for scanner fallback mode).
func (m *Menu) render() {
	// Calculate width based on longest item
	width := len(m.Title)
	for _, item := range m.Items {
		itemLen := len(item.Key) + len(item.Label) + 5
		if itemLen > width {
			width = itemLen
		}
	}
	if width < 40 {
		width = 40
	}

	// Draw box
	border := strings.Repeat("═", width)
	_, _ = fmt.Fprintf(m.output, "╔%s╗\n", border)
	_, _ = fmt.Fprintf(m.output, "║%s║\n", centerText(m.Title, width))
	_, _ = fmt.Fprintf(m.output, "╠%s╣\n", border)

	// Draw items
	for _, item := range m.Items {
		if item.Key == "" && item.Label == "" {
			// Separator
			_, _ = fmt.Fprintf(m.output, "╟%s╢\n", strings.Repeat("─", width))
		} else if item.Hidden {
			continue
		} else {
			text := fmt.Sprintf("  %s. %s", item.Key, item.Label)
			_, _ = fmt.Fprintf(m.output, "║%-*s║\n", width, text)
		}
	}

	_, _ = fmt.Fprintf(m.output, "╚%s╝\n", border)

	if m.Footer != "" {
		_, _ = fmt.Fprintf(m.output, "\n%s\n", m.Footer)
	}
}

// centerText centers text within a given width.
func centerText(text string, width int) string {
	if len(text) >= width {
		return text
	}
	padding := (width - len(text)) / 2
	return strings.Repeat(" ", padding) + text + strings.Repeat(" ", width-len(text)-padding)
}

// clearScreen clears the terminal screen.
func clearScreen(w io.Writer) {
	// ANSI escape sequence to clear screen and move cursor to top-left
	_, _ = fmt.Fprint(w, "\033[2J\033[H")
}

// WaitForKey waits for the user to press Enter.
func WaitForKey(r io.Reader, w io.Writer, prompt string) {
	if prompt == "" {
		prompt = "Press Enter to continue..."
	}
	_, _ = fmt.Fprint(w, prompt)
	bufio.NewScanner(r).Scan()
}

// Confirm asks the user for confirmation using huh.
func Confirm(r io.Reader, w io.Writer, prompt string) bool {
	// If not using stdin, fall back to scanner-based input
	if r != os.Stdin {
		return confirmWithScanner(r, w, prompt)
	}

	var confirmed bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(prompt).
				Affirmative("Yes").
				Negative("No").
				Value(&confirmed),
		),
	)

	if err := form.Run(); err != nil {
		return false
	}
	return confirmed
}

// confirmWithScanner provides scanner-based confirmation for testing.
func confirmWithScanner(r io.Reader, w io.Writer, prompt string) bool {
	_, _ = fmt.Fprintf(w, "%s [y/N]: ", prompt)

	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return false
	}

	response := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return response == "y" || response == "yes"
}

// Select presents options and returns the selected index using huh.
func Select(r io.Reader, w io.Writer, prompt string, options []string) int {
	// If not using stdin, fall back to scanner-based input
	if r != os.Stdin {
		return selectWithScanner(r, w, prompt, options)
	}

	var choice int
	var huhOptions []huh.Option[int]
	for i, opt := range options {
		huhOptions = append(huhOptions, huh.NewOption(opt, i))
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[int]().
				Title(prompt).
				Options(huhOptions...).
				Value(&choice),
		),
	)

	if err := form.Run(); err != nil {
		return -1
	}
	return choice
}

// selectWithScanner provides scanner-based selection for testing.
func selectWithScanner(r io.Reader, w io.Writer, prompt string, options []string) int {
	_, _ = fmt.Fprintln(w, prompt)
	for i, opt := range options {
		_, _ = fmt.Fprintf(w, "  %d. %s\n", i+1, opt)
	}
	_, _ = fmt.Fprint(w, "Selection: ")

	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return -1
	}

	var choice int
	_, err := fmt.Sscanf(strings.TrimSpace(scanner.Text()), "%d", &choice)
	if err != nil || choice < 1 || choice > len(options) {
		return -1
	}

	return choice - 1
}

// Input prompts for text input using huh.
func Input(r io.Reader, w io.Writer, prompt string) string {
	// If not using stdin, fall back to scanner-based input
	if r != os.Stdin {
		return inputWithScanner(r, w, prompt)
	}

	var value string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(prompt).
				Value(&value),
		),
	)

	if err := form.Run(); err != nil {
		return ""
	}
	return value
}

// inputWithScanner provides scanner-based input for testing.
func inputWithScanner(r io.Reader, w io.Writer, prompt string) string {
	_, _ = fmt.Fprintf(w, "%s: ", prompt)

	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return ""
	}
	return strings.TrimSpace(scanner.Text())
}

// RunCommand runs a shell command and displays output.
func RunCommand(w io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...) // #nosec G204 G702 -- caller is responsible for providing safe command name and args
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}

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
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "nano"
			}
			return RunCommand(os.Stdout, "sudo", editor, "/etc/lyrebird/config.yaml")
		},
	})

	menu.AddSeparator()

	menu.AddItem(MenuItem{
		Key:   "0",
		Label: "Back to Main Menu",
	})

	return menu
}

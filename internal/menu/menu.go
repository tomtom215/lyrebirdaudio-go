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
	"errors"
	"fmt"
	"io"
	"os"
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
			if errors.Is(err, huh.ErrUserAborted) {
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

// SPDX-License-Identifier: MIT

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

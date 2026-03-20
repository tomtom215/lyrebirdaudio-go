// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tomtom215/lyrebirdaudio-go/internal/menu"
	"github.com/tomtom215/lyrebirdaudio-go/internal/updater"
)

// runUpdate checks for and installs updates.
func runUpdate(args []string) error {
	// Parse flags
	checkOnly := false
	force := false

	for _, arg := range args {
		switch arg {
		case "--check":
			checkOnly = true
		case "--force":
			force = true
		}
	}

	fmt.Println("LyreBirdAudio Update")
	fmt.Println("====================")
	fmt.Println()

	// Create updater
	u := updater.New(
		updater.WithOwner("tomtom215"),
		updater.WithRepo("lyrebirdaudio-go"),
		updater.WithCurrentVersion(Version),
	)

	ctx := context.Background()

	// Check for updates
	fmt.Println("Checking for updates...")
	info, err := u.CheckForUpdates(ctx)
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	// Display update info
	fmt.Println(updater.FormatUpdateInfo(info))

	if !info.UpdateAvailable {
		return nil
	}

	if checkOnly {
		fmt.Println("\nRun 'lyrebird update' without --check to install the update.")
		return nil
	}

	// Prompt for confirmation unless forced
	if !force {
		fmt.Print("Download and install update? [y/N]: ")
		var response string
		_, _ = fmt.Scanln(&response)
		if strings.ToLower(response) != "y" {
			fmt.Println("Update cancelled.")
			return nil
		}
	}

	// Find current binary path
	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to determine binary path: %w", err)
	}
	binaryPath, err = filepath.EvalSymlinks(binaryPath)
	if err != nil {
		return fmt.Errorf("failed to resolve binary path: %w", err)
	}

	// Check if we need root for system binary
	if strings.HasPrefix(binaryPath, "/usr/") && os.Geteuid() != 0 {
		return fmt.Errorf("update requires root privileges for %s (run with sudo)", binaryPath)
	}

	fmt.Println()
	fmt.Println("Downloading update...")

	// Progress callback
	lastPercent := 0
	progress := func(downloaded, total int64) {
		if total > 0 {
			percent := int(float64(downloaded) / float64(total) * 100)
			if percent > lastPercent+5 || percent == 100 {
				fmt.Printf("\rProgress: %d%%", percent)
				lastPercent = percent
			}
		}
	}

	if err := u.Update(ctx, info, binaryPath, progress); err != nil {
		fmt.Println()
		// Check if there's a backup to rollback to
		if u.HasBackup(binaryPath) {
			fmt.Println("Update failed. Rolling back...")
			if rbErr := u.Rollback(binaryPath); rbErr != nil {
				return fmt.Errorf("update failed (%w) and rollback failed (%w)", err, rbErr)
			}
			fmt.Println("Rolled back to previous version.")
		}
		return fmt.Errorf("update failed: %w", err)
	}

	fmt.Println()
	fmt.Printf("Successfully updated to %s!\n", info.LatestVersion)
	fmt.Println("Restart lyrebird to use the new version.")

	return nil
}

// runMenu launches the interactive management menu.
func runMenu(args []string) error {
	m := menu.CreateMainMenu()
	return m.Display()
}

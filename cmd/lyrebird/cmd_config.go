// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tomtom215/lyrebirdaudio-go/internal/config"
)

// runMigrate migrates configuration from bash to YAML.
func runMigrate(args []string) error {
	// Parse flags
	fromPath := ""
	toPath := defaultConfigPath
	force := false

	for i := 0; i < len(args); i++ {
		switch {
		case strings.HasPrefix(args[i], "--from="):
			fromPath = strings.TrimPrefix(args[i], "--from=")
		case args[i] == "--from" && i+1 < len(args):
			fromPath = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--to="):
			toPath = strings.TrimPrefix(args[i], "--to=")
		case args[i] == "--to" && i+1 < len(args):
			toPath = args[i+1]
			i++
		case args[i] == "--force":
			force = true
		}
	}

	if fromPath == "" {
		return fmt.Errorf("--from path is required")
	}

	// Check if destination exists
	if _, err := os.Stat(toPath); err == nil && !force { // #nosec G703 -- toPath is from CLI flag, not web request
		return fmt.Errorf("destination file exists (use --force to overwrite): %s", toPath)
	}

	// Migrate
	fmt.Printf("Migrating configuration...\n")
	fmt.Printf("  From: %s\n", fromPath)
	fmt.Printf("  To:   %s\n\n", toPath)

	cfg, err := config.MigrateFromBash(fromPath)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	// Validate migrated config
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("migrated config is invalid: %w", err)
	}

	// Create directory if needed
	if err := os.MkdirAll(filepath.Dir(toPath), 0750); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// P-5 fix: Backup existing config before overwriting.
	backupDir := config.GetBackupDir(toPath)
	if _, err := os.Stat(toPath); err == nil {
		if backupPath, backupErr := config.BackupConfig(toPath, backupDir); backupErr != nil {
			fmt.Printf("  [!] Warning: failed to backup existing config: %v\n", backupErr)
		} else {
			fmt.Printf("  [✓] Backed up existing config to %s\n", backupPath)
		}
	}

	// Save
	if err := cfg.Save(toPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("✓ Migration complete\n")
	fmt.Printf("✓ Migrated %d device(s)\n", len(cfg.Devices))
	fmt.Println("\nRun 'lyrebird validate' to verify the configuration")

	return nil
}

// runValidate validates a configuration file.
func runValidate(args []string) error {
	configPath := defaultConfigPath

	// Parse flags
	for i := 0; i < len(args); i++ {
		switch {
		case strings.HasPrefix(args[i], "--config="):
			configPath = strings.TrimPrefix(args[i], "--config=")
		case args[i] == "--config" && i+1 < len(args):
			configPath = args[i+1]
			i++
		}
	}

	fmt.Printf("Validating configuration: %s\n\n", configPath)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	fmt.Println("✓ Configuration is valid")
	fmt.Printf("✓ Loaded %d device configuration(s)\n", len(cfg.Devices))

	// Show summary
	if len(cfg.Devices) > 0 {
		fmt.Println("\nConfigured devices:")
		for name := range cfg.Devices {
			fmt.Printf("  - %s\n", name)
		}
	}

	return nil
}

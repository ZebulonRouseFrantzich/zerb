package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ZebulonRouseFrantzich/zerb/internal/chezmoi"
	"github.com/ZebulonRouseFrantzich/zerb/internal/config"
	"github.com/ZebulonRouseFrantzich/zerb/internal/git"
	"github.com/ZebulonRouseFrantzich/zerb/internal/service"
)

// runConfigAdd handles the `zerb config add` subcommand
func runConfigAdd(args []string) error {
	// Parse flags and paths
	showHelp := false
	dryRun := false

	// Default options
	globalOpts := service.ConfigOptions{
		Recursive: false,
		Template:  false,
		Secrets:   false,
		Private:   false,
	}

	var paths []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--help", "-h":
			showHelp = true
		case "--dry-run", "-n":
			dryRun = true
		case "--recursive", "-r":
			globalOpts.Recursive = true
		case "--template", "-t":
			globalOpts.Template = true
		case "--secrets", "-s":
			globalOpts.Secrets = true
		case "--private", "-p":
			globalOpts.Private = true
		default:
			// Anything not starting with - is a path
			if len(arg) > 0 && arg[0] != '-' {
				paths = append(paths, arg)
			} else {
				return fmt.Errorf("unknown option: %s\nRun 'zerb config add --help' for usage", arg)
			}
		}
	}

	if showHelp {
		printConfigAddHelp()
		return nil
	}

	if len(paths) == 0 {
		return fmt.Errorf("no paths specified; run 'zerb config add --help' for usage")
	}

	// Create context with timeout (2 minutes for potentially large directories)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Get ZERB directory
	zerbDir, err := getZerbDir()
	if err != nil {
		return fmt.Errorf("get ZERB directory: %w", err)
	}

	// Check if ZERB is initialized
	if _, err := os.Stat(zerbDir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("ZERB not initialized\nRun 'zerb init' to set up ZERB first")
		}
		return fmt.Errorf("check ZERB directory: %w", err)
	}

	// Create dependencies
	chezmoiClient := chezmoi.NewClient(zerbDir)
	gitClient := git.NewClient(zerbDir)
	parser := config.NewParser(nil)
	generator := config.NewGenerator()
	clock := service.RealClock{}

	// Create service
	svc := service.NewConfigAddService(
		chezmoiClient,
		gitClient,
		parser,
		generator,
		clock,
		zerbDir,
	)

	// Build options map (apply global options to all paths)
	optionsMap := make(map[string]service.ConfigOptions)
	for _, path := range paths {
		optionsMap[path] = globalOpts
	}

	// Execute
	req := service.AddRequest{
		Paths:   paths,
		Options: optionsMap,
		DryRun:  dryRun,
	}

	result, err := svc.Execute(ctx, req)
	if err != nil {
		return err
	}

	// Print results
	if dryRun {
		fmt.Println("Dry run - no changes made")
		fmt.Println()
	}

	if len(result.AddedPaths) > 0 {
		if dryRun {
			fmt.Println("Would add:")
		} else {
			fmt.Println("Added:")
		}
		for _, path := range result.AddedPaths {
			fmt.Printf("  ✓ %s\n", path)
		}
	}

	if len(result.SkippedPaths) > 0 {
		fmt.Println()
		fmt.Println("Skipped (already tracked):")
		for _, path := range result.SkippedPaths {
			fmt.Printf("  - %s\n", path)
		}
	}

	if !dryRun && len(result.AddedPaths) > 0 {
		fmt.Println()
		if result.CommitHash != "" {
			fmt.Printf("Committed: %s\n", result.CommitHash[:8])
		}
		if result.ConfigVersion != "" {
			fmt.Printf("Config version: %s\n", result.ConfigVersion)
		}
	}

	return nil
}

// printConfigAddHelp prints help for the config add command
func printConfigAddHelp() {
	fmt.Println("Usage: zerb config add [options] <path>...")
	fmt.Println()
	fmt.Println("Add configuration files to ZERB tracking.")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -h, --help       Show this help message")
	fmt.Println("  -n, --dry-run    Show what would be added without making changes")
	fmt.Println("  -r, --recursive  Add directory and all contents recursively")
	fmt.Println("  -t, --template   Enable template processing (for dynamic configs)")
	fmt.Println("  -s, --secrets    Encrypt file with GPG (for sensitive data)")
	fmt.Println("  -p, --private    Set file permissions to 600 (user-only access)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  zerb config add ~/.zshrc              Add shell config")
	fmt.Println("  zerb config add ~/.gitconfig          Add git config")
	fmt.Println("  zerb config add ~/.config/nvim -r     Add nvim directory recursively")
	fmt.Println("  zerb config add ~/.ssh/config -p      Add SSH config as private")
	fmt.Println("  zerb config add ~/.env -s             Add env file as encrypted")
	fmt.Println("  zerb config add --dry-run ~/.bashrc   Preview without changes")
	fmt.Println()
	fmt.Println("Notes:")
	fmt.Println("  - Paths are normalized (~ is expanded to home directory)")
	fmt.Println("  - Directories require --recursive flag")
	fmt.Println("  - Already-tracked files are skipped")
	fmt.Println("  - Changes are committed to git automatically")
	fmt.Println()
	os.Exit(0)
}

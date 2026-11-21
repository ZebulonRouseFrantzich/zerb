package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ZebulonRouseFrantzich/zerb/internal/chezmoi"
	"github.com/ZebulonRouseFrantzich/zerb/internal/config"
	"github.com/ZebulonRouseFrantzich/zerb/internal/git"
	"github.com/ZebulonRouseFrantzich/zerb/internal/service"
)

// configRemoveOpts holds parsed options for config remove command
type configRemoveOpts struct {
	showHelp bool
	dryRun   bool
	purge    bool
	yes      bool
	paths    []string
}

// parseConfigRemoveArgs parses command line arguments for config remove
func parseConfigRemoveArgs(args []string) (*configRemoveOpts, error) {
	opts := &configRemoveOpts{
		paths: make([]string, 0),
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--help", "-h":
			opts.showHelp = true
		case "--dry-run", "-n":
			opts.dryRun = true
		case "--purge":
			opts.purge = true
		case "--yes", "-y":
			opts.yes = true
		default:
			// Anything not starting with - is a path
			if len(arg) > 0 && arg[0] != '-' {
				opts.paths = append(opts.paths, arg)
			} else {
				return nil, fmt.Errorf("unknown option: %s\nRun 'zerb config remove --help' for usage", arg)
			}
		}
	}

	return opts, nil
}

// runConfigRemove handles the `zerb config remove` subcommand
func runConfigRemove(args []string) error {
	opts, err := parseConfigRemoveArgs(args)
	if err != nil {
		return err
	}

	if opts.showHelp {
		printConfigRemoveHelp()
		return nil
	}

	if len(opts.paths) == 0 {
		return fmt.Errorf("no paths specified; run 'zerb config remove --help' for usage")
	}

	// Create context with timeout (2 minutes for potentially large operations)
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
	svc := service.NewConfigRemoveService(
		chezmoiClient,
		gitClient,
		parser,
		generator,
		clock,
		zerbDir,
	)

	// Build request
	req := service.RemoveRequest{
		Paths:  opts.paths,
		DryRun: opts.dryRun,
		Purge:  opts.purge,
	}

	// Show what will be removed and ask for confirmation (unless --yes or --dry-run)
	if !opts.yes && !opts.dryRun {
		confirmed, err := confirmRemove(opts.paths, opts.purge)
		if err != nil {
			return fmt.Errorf("confirmation: %w", err)
		}
		if !confirmed {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Execute
	result, err := svc.Execute(ctx, req)
	if err != nil {
		return err
	}

	// Print results
	printConfigRemoveResult(result, opts.dryRun, opts.purge)

	return nil
}

// confirmRemove prompts the user to confirm the remove operation
func confirmRemove(paths []string, purge bool) (bool, error) {
	fmt.Println("The following configs will be removed from tracking:")
	for _, path := range paths {
		fmt.Printf("  • %s\n", path)
	}
	fmt.Println()

	if purge {
		fmt.Println("⚠️  WARNING: Source files will be DELETED from disk (--purge)")
	} else {
		fmt.Println("Note: Source files will be kept on disk (only untracked from ZERB)")
	}
	fmt.Println()

	fmt.Print("Continue? [y/N] ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes", nil
}

// printConfigRemoveResult prints the result of the remove operation
func printConfigRemoveResult(result *service.RemoveResult, dryRun bool, purge bool) {
	if dryRun {
		fmt.Println("Dry run - no changes made")
		fmt.Println()
	}

	if len(result.RemovedPaths) > 0 {
		if dryRun {
			fmt.Println("Would remove:")
		} else {
			fmt.Println("Removed:")
		}
		for _, path := range result.RemovedPaths {
			fmt.Printf("  ✓ %s\n", path)
		}
	}

	if len(result.SkippedPaths) > 0 {
		fmt.Println()
		fmt.Println("Skipped (not tracked):")
		for _, path := range result.SkippedPaths {
			fmt.Printf("  - %s\n", path)
		}
	}

	if !dryRun && len(result.RemovedPaths) > 0 {
		fmt.Println()
		if result.CommitHash != "" {
			shortHash := result.CommitHash
			if len(shortHash) > 8 {
				shortHash = shortHash[:8]
			}
			fmt.Printf("Committed: %s\n", shortHash)
		}
		if result.ConfigVersion != "" {
			fmt.Printf("Config version: %s\n", result.ConfigVersion)
		}

		if purge {
			fmt.Println()
			fmt.Println("Source files have been deleted from disk.")
		}
	}
}

// printConfigRemoveHelp prints help for the config remove command
func printConfigRemoveHelp() {
	fmt.Println("Usage: zerb config remove [options] <path>...")
	fmt.Println()
	fmt.Println("Remove configuration files from ZERB tracking.")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -h, --help       Show this help message")
	fmt.Println("  -n, --dry-run    Show what would be removed without making changes")
	fmt.Println("  -y, --yes        Skip confirmation prompt")
	fmt.Println("      --purge      Also delete source file from disk (DANGEROUS)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  zerb config remove ~/.zshrc              Remove shell config from tracking")
	fmt.Println("  zerb config remove ~/.gitconfig --yes    Remove git config, skip confirmation")
	fmt.Println("  zerb config remove --dry-run ~/.bashrc   Preview without changes")
	fmt.Println("  zerb config remove --purge ~/.old-config Remove from tracking AND delete file")
	fmt.Println()
	fmt.Println("Notes:")
	fmt.Println("  - By default, source files are KEPT on disk (only untracked from ZERB)")
	fmt.Println("  - Use --purge to also delete the source file (requires confirmation)")
	fmt.Println("  - Paths must already be tracked by ZERB")
	fmt.Println("  - Changes are committed to git automatically")
	fmt.Println()
	os.Exit(0)
}

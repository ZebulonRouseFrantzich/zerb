package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ZebulonRouseFrantzich/zerb/internal/chezmoi"
	"github.com/ZebulonRouseFrantzich/zerb/internal/config"
	"github.com/ZebulonRouseFrantzich/zerb/internal/service"
)

// runConfigList handles the `zerb config list` subcommand
func runConfigList(args []string) error {
	// Parse flags
	showHelp := false
	for _, arg := range args {
		switch arg {
		case "--help", "-h":
			showHelp = true
		}
	}

	if showHelp {
		printConfigListHelp()
		return nil
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get ZERB directory
	zerbDir, err := getZerbDir()
	if err != nil {
		return fmt.Errorf("get ZERB directory: %w", err)
	}

	// Create dependencies
	parser := config.NewParser(nil) // No platform detection needed for listing
	chezmoiClient := chezmoi.NewClient(zerbDir)
	detector := config.NewDefaultStatusDetector(chezmoiClient)

	// Create service and execute
	svc := service.NewConfigListService(parser, detector, zerbDir)
	result, err := svc.List(ctx, service.ListRequest{})
	if err != nil {
		return err
	}

	// Format and print output
	if len(result.Configs) == 0 {
		fmt.Println("No configuration files are being tracked.")
		fmt.Println()
		fmt.Println("To add a config file:")
		fmt.Println("  zerb config add ~/.zshrc")
		return nil
	}

	fmt.Println("Tracked configuration files:")
	fmt.Println()

	for _, cfg := range result.Configs {
		// Format status symbol
		symbol := cfg.Status.Symbol()

		// Format path with options
		path := cfg.ConfigFile.Path
		opts := formatConfigOptions(cfg.ConfigFile)

		if opts != "" {
			fmt.Printf("  %s %s %s\n", symbol, path, opts)
		} else {
			fmt.Printf("  %s %s\n", symbol, path)
		}
	}

	fmt.Println()
	fmt.Println("Legend: ✓ synced, ✗ missing, ? partial")

	return nil
}

// formatConfigOptions formats config file options for display
func formatConfigOptions(cfg config.ConfigFile) string {
	var opts []string

	if cfg.Template {
		opts = append(opts, "template")
	}
	if cfg.Secrets {
		opts = append(opts, "encrypted")
	}
	if cfg.Private {
		opts = append(opts, "private")
	}
	if cfg.Recursive {
		opts = append(opts, "recursive")
	}

	if len(opts) == 0 {
		return ""
	}

	result := "("
	for i, opt := range opts {
		if i > 0 {
			result += ", "
		}
		result += opt
	}
	result += ")"

	return result
}

// printConfigListHelp prints help for the config list command
func printConfigListHelp() {
	fmt.Println("Usage: zerb config list [options]")
	fmt.Println()
	fmt.Println("List all configuration files tracked by ZERB.")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -h, --help    Show this help message")
	fmt.Println()
	fmt.Println("Status indicators:")
	fmt.Println("  ✓  synced   File exists and is managed by ZERB")
	fmt.Println("  ✗  missing  File is declared but doesn't exist on disk")
	fmt.Println("  ?  partial  File exists but not fully managed by ZERB")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  zerb config list          List all tracked configs")
	fmt.Println()
	os.Exit(0)
}

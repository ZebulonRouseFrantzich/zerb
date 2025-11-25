package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ZebulonRouseFrantzich/zerb/internal/drift"
)

// runDrift handles the `zerb drift` subcommand
// Returns an exit code (0 = no drifts, 1 = drifts detected) and an error
func runDrift(args []string) (int, error) {
	// Parse flags
	showHelp := false
	dryRun := false
	forceRefresh := false

	for _, arg := range args {
		switch arg {
		case "--help", "-h":
			showHelp = true
		case "--dry-run", "-n":
			dryRun = true
		case "--refresh":
			forceRefresh = true
		}
	}

	if showHelp {
		printDriftHelp()
		return 0, nil
	}

	// Create context with timeout (2 minutes for potentially slow version detection)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Get ZERB directory
	zerbDir, err := getZerbDir()
	if err != nil {
		return 1, fmt.Errorf("get ZERB directory: %w", err)
	}

	// Check if ZERB is initialized
	activeConfigPath := filepath.Join(zerbDir, "zerb.active.lua")
	if _, err := os.Stat(activeConfigPath); err != nil {
		if os.IsNotExist(err) {
			return 1, fmt.Errorf("ZERB not initialized\nRun 'zerb init' to set up ZERB first")
		}
		return 1, fmt.Errorf("check ZERB initialization: %w", err)
	}

	if dryRun {
		fmt.Println("Drift detection (dry-run mode)")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println()
	}

	// Step 1: Query baseline (declared tools in config)
	fmt.Println("Reading baseline configuration...")
	baseline, err := drift.QueryBaseline(ctx, activeConfigPath)
	if err != nil {
		return 1, fmt.Errorf("query baseline: %w", err)
	}

	if len(baseline) == 0 {
		fmt.Println()
		fmt.Println("No tools declared in configuration.")
		fmt.Println()
		fmt.Println("To add tools:")
		fmt.Println("  zerb add node@20")
		fmt.Println("  zerb add python@3.12")
		return 0, nil
	}

	// Step 2: Query managed tools (ZERB-installed via mise)
	fmt.Println("Querying managed tools...")
	managed, err := drift.QueryManaged(ctx, zerbDir)
	if err != nil {
		// Non-fatal: continue with empty managed list
		fmt.Fprintf(os.Stderr, "Warning: could not query managed tools: %v\n", err)
		managed = []drift.Tool{}
	}

	// Step 3: Query active tools (in PATH)
	fmt.Println("Detecting active tools in environment...")
	toolNames := make([]string, len(baseline))
	for i, spec := range baseline {
		toolNames[i] = spec.Name
	}
	active, err := drift.QueryActive(ctx, toolNames, forceRefresh)
	if err != nil {
		// Non-fatal: continue with empty active list
		fmt.Fprintf(os.Stderr, "Warning: could not query active tools: %v\n", err)
		active = []drift.Tool{}
	}

	// Step 4: Detect drift
	results := drift.DetectDrift(baseline, managed, active, zerbDir)

	// Step 5: Format and print report
	report := drift.FormatDriftReport(results)
	fmt.Print(report)

	// Count drifts for exit code
	driftCount := 0
	for _, r := range results {
		if r.DriftType != drift.DriftOK {
			driftCount++
		}
	}

	// Print remediation hints if there are drifts
	if driftCount > 0 && !dryRun {
		fmt.Println()
		fmt.Println("To fix drifts:")
		fmt.Println("  zerb sync        Sync tools to baseline")
		fmt.Println("  zerb drift --help  Show more options")
	}

	// Return non-zero exit code if drifts detected (for scripting)
	if driftCount > 0 {
		return 1, nil
	}

	return 0, nil
}

// printDriftHelp prints help for the drift command
func printDriftHelp() {
	fmt.Println("Usage: zerb drift [options]")
	fmt.Println()
	fmt.Println("Detect drift between your declared configuration and actual environment.")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -h, --help     Show this help message")
	fmt.Println("  -n, --dry-run  Show what would be detected without side effects")
	fmt.Println("  --refresh      Force refresh version cache (slower but more accurate)")
	fmt.Println()
	fmt.Println("Drift types:")
	fmt.Println("  OK                    Tool matches baseline")
	fmt.Println("  VERSION_MISMATCH      Installed version differs from baseline")
	fmt.Println("  MISSING               Tool declared but not found")
	fmt.Println("  EXTRA                 Tool installed but not in baseline")
	fmt.Println("  EXTERNAL_OVERRIDE     External installation taking precedence")
	fmt.Println("  MANAGED_BUT_NOT_ACTIVE Tool installed but not in PATH")
	fmt.Println("  VERSION_UNKNOWN       Could not detect version")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  zerb drift             Check for drift")
	fmt.Println("  zerb drift --dry-run   Preview drift detection")
	fmt.Println("  zerb drift --refresh   Force version re-detection")
	fmt.Println()
	fmt.Println("Exit codes:")
	fmt.Println("  0  No drifts detected")
	fmt.Println("  1  One or more drifts detected")
	fmt.Println()
}

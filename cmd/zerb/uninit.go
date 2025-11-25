package main

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ZebulonRouseFrantzich/zerb/internal/shell"
)

// UninitFlags holds command-line flags for uninit
type UninitFlags struct {
	force       bool
	keepConfigs bool
	keepCache   bool
	keepBackups bool
	noBackup    bool
	dryRun      bool
}

// validateZerbDirForRemoval checks if zerbDir is safe to remove (no path traversal)
func validateZerbDirForRemoval(zerbDir string) error {
	// Clean the path first
	cleaned := filepath.Clean(zerbDir)

	// Check for path traversal sequences
	if strings.Contains(cleaned, "..") {
		return fmt.Errorf("invalid zerbDir: contains path traversal sequence")
	}

	// Convert to absolute path for validation
	absPath, err := filepath.Abs(cleaned)
	if err != nil {
		return fmt.Errorf("invalid zerbDir: cannot resolve absolute path: %w", err)
	}

	// Ensure the path is not root or system directories
	systemDirs := []string{"/", "\\", "/usr", "/bin", "/sbin", "/etc", "/var", "/lib", "/boot"}
	for _, sysDir := range systemDirs {
		if absPath == sysDir || absPath == filepath.Clean(sysDir) {
			return fmt.Errorf("invalid zerbDir: cannot remove system directory %s", absPath)
		}
	}

	return nil
}

// parseUninitFlags parses command-line flags for uninit command
func parseUninitFlags(args []string) (*UninitFlags, error) {
	flags := &UninitFlags{}

	for _, arg := range args {
		switch arg {
		case "--force", "-f":
			flags.force = true
		case "--keep-configs":
			flags.keepConfigs = true
		case "--keep-cache":
			flags.keepCache = true
		case "--keep-backups":
			flags.keepBackups = true
		case "--no-backup":
			flags.noBackup = true
		case "--dry-run":
			flags.dryRun = true
		case "--help", "-h":
			printUninitHelp()
			return nil, fmt.Errorf("help requested")
		default:
			if strings.HasPrefix(arg, "-") {
				return nil, fmt.Errorf("unknown flag: %s", arg)
			}
		}
	}

	return flags, nil
}

// printUninitHelp prints help text for uninit command
func printUninitHelp() {
	fmt.Println("Usage: zerb uninit [OPTIONS]")
	fmt.Println()
	fmt.Println("Remove ZERB from your system")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --force, -f        Skip confirmation prompts")
	fmt.Println("  --keep-configs     Preserve the configs/ directory")
	fmt.Println("  --keep-cache       Preserve the cache/ directory")
	fmt.Println("  --keep-backups     Don't remove old backup files")
	fmt.Println("  --dry-run          Show what would be removed without removing")
	fmt.Println("  --help, -h         Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  zerb uninit                    # Remove ZERB (with confirmation)")
	fmt.Println("  zerb uninit --keep-configs     # Remove ZERB but keep configs")
	fmt.Println("  zerb uninit --dry-run          # Preview what would be removed")
	fmt.Println("  zerb uninit --force            # Remove without confirmation")
}

// RemovalPlan describes what will be removed
type RemovalPlan struct {
	ZerbDir           string
	ZerbDirSize       int64
	ZerbDirExists     bool
	Binaries          []string
	ConfigCount       int
	CacheSize         int64
	ShellIntegrations []ShellIntegration
	BackupFiles       []string
	ActualBackupPaths []string // Actual backup files created during removal
}

// ShellIntegration describes shell integration to remove
type ShellIntegration struct {
	Shell  string
	RCFile string
	Line   int
}

// analyzeInstallation analyzes the current ZERB installation
func analyzeInstallation(ctx context.Context, zerbDir string) (*RemovalPlan, error) {
	plan := &RemovalPlan{
		ZerbDir: zerbDir,
	}

	// Check if ZERB directory exists
	if _, err := os.Stat(zerbDir); err == nil {
		plan.ZerbDirExists = true

		// Calculate directory size
		size, err := calculateDirectorySize(zerbDir)
		if err == nil {
			plan.ZerbDirSize = size
		}

		// Find binaries
		binDir := filepath.Join(zerbDir, "bin")
		if entries, err := os.ReadDir(binDir); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					plan.Binaries = append(plan.Binaries, entry.Name())
				}
			}
		}

		// Count configs
		configsDir := filepath.Join(zerbDir, "configs")
		if entries, err := os.ReadDir(configsDir); err == nil {
			plan.ConfigCount = len(entries)
		}

		// Calculate cache size
		cacheDir := filepath.Join(zerbDir, "cache")
		if size, err := calculateDirectorySize(cacheDir); err == nil {
			plan.CacheSize = size
		}
	}

	// Detect shell integrations
	homeDir, err := os.UserHomeDir()
	if err == nil {
		shells := []shell.ShellType{shell.ShellBash, shell.ShellZsh, shell.ShellFish}
		for _, sh := range shells {
			rcPath, err := shell.GetRCFilePath(sh)
			if err != nil {
				continue
			}

			hasActivation, err := shell.HasActivationLine(rcPath)
			if err != nil || !hasActivation {
				continue
			}

			// Find line number (for display)
			lineNum := findActivationLineNumber(rcPath)
			plan.ShellIntegrations = append(plan.ShellIntegrations, ShellIntegration{
				Shell:  sh.String(),
				RCFile: rcPath,
				Line:   lineNum,
			})
		}

		// Find backup files
		plan.BackupFiles = findBackupFiles(homeDir)
	}

	return plan, nil
}

// calculateDirectorySize calculates total size of directory
func calculateDirectorySize(path string) (int64, error) {
	var size int64
	err := filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors, continue
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err == nil {
				size += info.Size()
			}
		}
		return nil
	})
	return size, err
}

// findActivationLineNumber finds the line number of the ZERB activation in RC file
func findActivationLineNumber(rcPath string) int {
	file, err := os.Open(rcPath)
	if err != nil {
		return 0
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if strings.Contains(scanner.Text(), shell.ActivationMarker) {
			return lineNum
		}
	}
	return 0
}

// findBackupFiles finds all ZERB backup files in home directory
func findBackupFiles(homeDir string) []string {
	var backups []string

	// Look for .bashrc, .zshrc backup files
	patterns := []string{
		filepath.Join(homeDir, ".bashrc"+shell.BackupSuffix+".*"),
		filepath.Join(homeDir, ".zshrc"+shell.BackupSuffix+".*"),
		filepath.Join(homeDir, ".config", "fish", "config.fish"+shell.BackupSuffix+".*"),
	}

	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		backups = append(backups, matches...)
	}

	return backups
}

// formatSize formats bytes as human-readable size
func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// showRemovalPlan displays what will be removed
func showRemovalPlan(plan *RemovalPlan, flags *UninitFlags) {
	fmt.Println("🗑️  ZERB Uninstallation Plan")
	fmt.Println()

	if !plan.ZerbDirExists {
		fmt.Println("ZERB is not installed (directory not found)")
		return
	}

	fmt.Println("The following will be removed:")

	// ZERB directory
	fmt.Printf("  [×] ZERB directory: %s (%s)\n", plan.ZerbDir, formatSize(plan.ZerbDirSize))
	if len(plan.Binaries) > 0 {
		fmt.Printf("      - bin/ (%d binaries: %s)\n", len(plan.Binaries), strings.Join(plan.Binaries, ", "))
	}
	if plan.ConfigCount > 0 && !flags.keepConfigs {
		fmt.Printf("      - configs/ (%d tracked configs)\n", plan.ConfigCount)
	} else if plan.ConfigCount > 0 && flags.keepConfigs {
		fmt.Printf("      - configs/ (%d tracked configs) [WILL BE PRESERVED]\n", plan.ConfigCount)
	}
	if plan.CacheSize > 0 && !flags.keepCache {
		fmt.Printf("      - cache/ (%s)\n", formatSize(plan.CacheSize))
	} else if plan.CacheSize > 0 && flags.keepCache {
		fmt.Printf("      - cache/ (%s) [WILL BE PRESERVED]\n", formatSize(plan.CacheSize))
	}
	fmt.Println("      - keyrings/, logs/, tmp/")

	// Shell integrations (informational only - not automatically removed)
	if len(plan.ShellIntegrations) > 0 {
		fmt.Println()
		fmt.Println("  [!] Shell integration found in:")
		for _, si := range plan.ShellIntegrations {
			fmt.Printf("      - %s (line %d)\n", si.RCFile, si.Line)
		}
		fmt.Println()
		fmt.Println("      You'll need to manually remove this after uninstall.")
		fmt.Println("      (Instructions will be shown after removal)")
	} else {
		fmt.Println()
		fmt.Println("  [✓] No shell integration detected")
	}

	// Backup files
	if len(plan.BackupFiles) > 0 && !flags.keepBackups {
		fmt.Println()
		fmt.Printf("  [×] Backup files: (%d files)\n", len(plan.BackupFiles))
		for _, backup := range plan.BackupFiles {
			fmt.Printf("      - %s\n", filepath.Base(backup))
		}
	}

	// Total size
	fmt.Println()
	totalSize := plan.ZerbDirSize
	if flags.keepCache {
		totalSize -= plan.CacheSize
	}
	fmt.Printf("Total disk space to be freed: %s\n", formatSize(totalSize))
}

// confirmUninit prompts user for confirmation
func confirmUninit(flags *UninitFlags) (bool, error) {
	if flags.force {
		return true, nil
	}

	fmt.Println()
	fmt.Println("⚠️  WARNING: This will permanently remove ZERB and all its data.")
	fmt.Println()

	// Show helpful flags
	if !flags.keepConfigs || !flags.keepCache {
		fmt.Println("💡 TIP: To preserve your data, cancel and run:")
		tipFlags := []string{}
		if !flags.keepConfigs {
			tipFlags = append(tipFlags, "--keep-configs")
		}
		if !flags.keepCache {
			tipFlags = append(tipFlags, "--keep-cache")
		}
		fmt.Printf("   zerb uninit %s\n", strings.Join(tipFlags, " "))
		fmt.Println()
	}

	fmt.Print("Are you sure you want to continue? (yes/no): ")
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("read input: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "yes" || response == "y", nil
}

// removeShellIntegrations removes ZERB from shell RC files
func removeShellIntegrations(plan *RemovalPlan, flags *UninitFlags) error {
	if len(plan.ShellIntegrations) == 0 {
		return nil
	}

	fmt.Println("Removing shell integration...")

	for _, si := range plan.ShellIntegrations {
		if flags.dryRun {
			fmt.Printf("  [DRY RUN] Would remove from %s\n", si.RCFile)
			continue
		}

		// Create backup unless --no-backup
		if !flags.noBackup {
			backupPath, err := shell.BackupRCFile(si.RCFile)
			if err != nil {
				fmt.Printf("  ⚠  Failed to backup %s: %v\n", si.RCFile, err)
			} else {
				fmt.Printf("  ✓ Backed up to %s\n", filepath.Base(backupPath))
			}
		}

		// Remove activation line
		if err := shell.RemoveActivationLine(si.RCFile); err != nil {
			return fmt.Errorf("remove from %s: %w", si.RCFile, err)
		}

		fmt.Printf("  ✓ Removed from %s\n", si.RCFile)
	}

	return nil
}

// removeZerbDirectory removes the ZERB directory
func removeZerbDirectory(zerbDir string, flags *UninitFlags) error {
	if !flags.dryRun {
		fmt.Println()
		fmt.Println("Removing ZERB directory...")
	}

	// Handle --keep-configs
	if flags.keepConfigs {
		configsDir := filepath.Join(zerbDir, "configs")
		if _, err := os.Stat(configsDir); err == nil {
			timestamp := time.Now().Format("20060102-150405")
			backupDir := filepath.Join(os.Getenv("HOME"), fmt.Sprintf(".zerb-configs-backup-%s", timestamp))

			if flags.dryRun {
				fmt.Printf("  [DRY RUN] Would move configs/ to %s\n", backupDir)
			} else {
				if err := os.Rename(configsDir, backupDir); err != nil {
					fmt.Printf("  ⚠  Failed to preserve configs: %v\n", err)
				} else {
					fmt.Printf("  ✓ Preserved configs to %s\n", backupDir)
				}
			}
		}
	}

	// Handle --keep-cache
	if flags.keepCache {
		cacheDir := filepath.Join(zerbDir, "cache")
		if _, err := os.Stat(cacheDir); err == nil {
			timestamp := time.Now().Format("20060102-150405")
			backupDir := filepath.Join(os.Getenv("HOME"), fmt.Sprintf(".zerb-cache-backup-%s", timestamp))

			if flags.dryRun {
				fmt.Printf("  [DRY RUN] Would move cache/ to %s\n", backupDir)
			} else {
				if err := os.Rename(cacheDir, backupDir); err != nil {
					fmt.Printf("  ⚠  Failed to preserve cache: %v\n", err)
				} else {
					fmt.Printf("  ✓ Preserved cache to %s\n", backupDir)
				}
			}
		}
	}

	// Validate zerbDir before removal to prevent path traversal attacks
	if err := validateZerbDirForRemoval(zerbDir); err != nil {
		return err
	}

	// Remove the directory
	if flags.dryRun {
		fmt.Printf("  [DRY RUN] Would remove %s\n", zerbDir)
	} else {
		if err := os.RemoveAll(zerbDir); err != nil {
			return fmt.Errorf("remove directory: %w", err)
		}
		fmt.Printf("  ✓ Removed %s\n", zerbDir)
	}

	return nil
}

// removeBackupFiles removes old ZERB backup files
func removeBackupFiles(backupFiles []string, flags *UninitFlags) error {
	if len(backupFiles) == 0 || flags.keepBackups {
		return nil
	}

	if !flags.dryRun {
		fmt.Println()
		fmt.Println("Removing backup files...")
	}

	for _, backup := range backupFiles {
		if flags.dryRun {
			fmt.Printf("  [DRY RUN] Would remove %s\n", filepath.Base(backup))
		} else {
			if err := os.Remove(backup); err != nil {
				fmt.Printf("  ⚠  Failed to remove %s: %v\n", filepath.Base(backup), err)
			}
		}
	}

	if !flags.dryRun {
		fmt.Printf("  ✓ Removed %d backup files\n", len(backupFiles))
	}

	return nil
}

// printUninitSuccessMessage prints the success message after uninstall
func printUninitSuccessMessage(plan *RemovalPlan, flags *UninitFlags) {
	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║  ZERB Successfully Uninstalled                             ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()

	fmt.Println("Removed:")
	fmt.Println("  • ZERB directory")
	if len(plan.BackupFiles) > 0 && !flags.keepBackups {
		fmt.Printf("  • %d backup files\n", len(plan.BackupFiles))
	}

	fmt.Println()
	totalSize := plan.ZerbDirSize
	if flags.keepCache {
		totalSize -= plan.CacheSize
	}
	fmt.Printf("Freed %s of disk space\n", formatSize(totalSize))

	// Show manual shell integration removal instructions
	if len(plan.ShellIntegrations) > 0 {
		fmt.Println()
		fmt.Println("⚠️  Don't forget to remove shell integration:")
		fmt.Println()

		for _, si := range plan.ShellIntegrations {
			// Parse shell type from string
			var shellType shell.ShellType
			switch si.Shell {
			case "bash":
				shellType = shell.ShellBash
			case "zsh":
				shellType = shell.ShellZsh
			case "fish":
				shellType = shell.ShellFish
			default:
				continue
			}

			// Get the activation command to show what to remove
			activationCmd, _ := shell.GenerateActivationCommand(shellType)

			fmt.Printf("   From %s:\n", si.RCFile)
			fmt.Printf("     sed -i \"/zerb activate/d\" %s\n", si.RCFile)
			fmt.Println()
			fmt.Printf("     Or edit %s and remove:\n", si.RCFile)
			fmt.Printf("     %s\n", activationCmd)
			fmt.Println()
		}

		if len(plan.ShellIntegrations) > 0 {
			fmt.Println("   Then reload your shell:")
			fmt.Printf("     source %s\n", plan.ShellIntegrations[0].RCFile)
			fmt.Println()
		}
	}

	if flags.keepConfigs {
		fmt.Println()
		fmt.Println("Your configs were preserved and can be found at:")
		timestamp := time.Now().Format("20060102-150405")
		fmt.Printf("  ~/.zerb-configs-backup-%s\n", timestamp)
	}

	if flags.keepCache {
		fmt.Println()
		fmt.Println("Your cache was preserved and can be found at:")
		timestamp := time.Now().Format("20060102-150405")
		fmt.Printf("  ~/.zerb-cache-backup-%s\n", timestamp)
	}

	fmt.Println()
	fmt.Println("To reinstall ZERB, run: zerb init")
}

// runUninit handles the `zerb uninit` subcommand
func runUninit(args []string) error {
	// Parse flags
	flags, err := parseUninitFlags(args)
	if err != nil {
		if err.Error() == "help requested" {
			return nil
		}
		return err
	}

	ctx := context.Background()

	// Get ZERB directory
	zerbDir, err := getZerbDir()
	if err != nil {
		return fmt.Errorf("get ZERB directory: %w", err)
	}

	// Analyze current installation
	plan, err := analyzeInstallation(ctx, zerbDir)
	if err != nil {
		return fmt.Errorf("analyze installation: %w", err)
	}

	// Check if ZERB is installed
	if !plan.ZerbDirExists {
		fmt.Println("ZERB is not installed")
		fmt.Printf("ZERB directory not found: %s\n", zerbDir)
		return nil
	}

	// Show removal plan
	showRemovalPlan(plan, flags)

	// Dry run mode - exit after showing plan
	if flags.dryRun {
		fmt.Println()
		fmt.Println("[DRY RUN] No changes were made")
		return nil
	}

	// Confirmation
	confirmed, err := confirmUninit(flags)
	if err != nil {
		return fmt.Errorf("confirmation: %w", err)
	}
	if !confirmed {
		fmt.Println()
		fmt.Println("Uninstall cancelled")
		return nil
	}

	fmt.Println()

	// Note: Shell integration is NOT automatically removed
	// Users must manually remove it from their rc files
	// Instructions will be shown in the success message

	// Remove ZERB directory
	if err := removeZerbDirectory(zerbDir, flags); err != nil {
		return fmt.Errorf("remove ZERB directory: %w", err)
	}

	// Remove backup files
	if err := removeBackupFiles(plan.BackupFiles, flags); err != nil {
		return fmt.Errorf("remove backup files: %w", err)
	}

	// Print success message
	printUninitSuccessMessage(plan, flags)

	return nil
}

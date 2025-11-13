package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ZebulonRouseFrantzich/zerb/internal/binary"
	"github.com/ZebulonRouseFrantzich/zerb/internal/config"
	"github.com/ZebulonRouseFrantzich/zerb/internal/platform"
	"github.com/ZebulonRouseFrantzich/zerb/internal/shell"
)

// createDirectoryStructure creates all required ZERB directories
// This is idempotent - safe to call multiple times
func createDirectoryStructure(zerbDir string) error {
	if zerbDir == "" {
		return fmt.Errorf("zerbDir cannot be empty")
	}

	// Define all directories to create
	dirs := []string{
		zerbDir,
		filepath.Join(zerbDir, "bin"),
		filepath.Join(zerbDir, "keyrings"),
		filepath.Join(zerbDir, "cache", "downloads"),
		filepath.Join(zerbDir, "cache", "versions"),
		filepath.Join(zerbDir, "configs"),
		filepath.Join(zerbDir, "tmp"),
		filepath.Join(zerbDir, "logs"),
		filepath.Join(zerbDir, "mise"),
		filepath.Join(zerbDir, "chezmoi", "source"),
	}

	// Create each directory with 0755 permissions
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	return nil
}

// isAlreadyInitialized checks if ZERB is already initialized in the given directory
func isAlreadyInitialized(zerbDir string) bool {
	// Check for key indicators of an initialized ZERB environment
	indicators := []string{
		filepath.Join(zerbDir, "bin", "mise"),
		filepath.Join(zerbDir, "configs"),
		filepath.Join(zerbDir, ".zerb-active"),
	}

	// If any key indicator exists, consider it initialized
	for _, path := range indicators {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}

	return false
}

// detectPlatform wraps platform detection with context support
func detectPlatform(ctx context.Context) (*platform.Info, error) {
	detector := platform.NewDetector()
	platformInfo, err := detector.Detect(ctx)
	if err != nil {
		return nil, fmt.Errorf("detect platform: %w", err)
	}
	return platformInfo, nil
}

// installBinaries installs mise and chezmoi binaries
func installBinaries(ctx context.Context, zerbDir string, platformInfo *platform.Info) error {
	// Create binary manager
	binManager, err := binary.NewManager(binary.Config{
		ZerbDir:      zerbDir,
		PlatformInfo: platformInfo,
	})
	if err != nil {
		return fmt.Errorf("create binary manager: %w", err)
	}

	// Extract embedded keyrings
	if err := binManager.EnsureKeyrings(); err != nil {
		return fmt.Errorf("extract keyrings: %w", err)
	}

	// Install mise binary
	if err := binManager.Install(ctx, binary.DownloadOptions{
		Binary:  binary.BinaryMise,
		Version: binary.DefaultVersions.Mise,
	}); err != nil {
		return fmt.Errorf("install mise: %w", err)
	}

	// Install chezmoi binary
	if err := binManager.Install(ctx, binary.DownloadOptions{
		Binary:  binary.BinaryChezmoi,
		Version: binary.DefaultVersions.Chezmoi,
	}); err != nil {
		return fmt.Errorf("install chezmoi: %w", err)
	}

	return nil
}

// generateInitialConfig creates an empty initial configuration
func generateInitialConfig(ctx context.Context, zerbDir string) error {
	// Create initial minimal config
	initialConfig := &config.Config{
		Meta: config.Meta{
			Name:        "My ZERB Environment",
			Description: "Created by zerb init",
		},
		Tools:   []string{}, // Empty initially - user adds tools with 'zerb add'
		Configs: []config.ConfigFile{},
		Git: config.GitConfig{
			Remote: "", // User can configure later
			Branch: "main",
		},
		Options: config.Options{
			BackupRetention: 5,
		},
	}

	// Generate Lua code
	generator := config.NewGenerator()
	luaCode, err := generator.Generate(ctx, initialConfig)
	if err != nil {
		return fmt.Errorf("generate config: %w", err)
	}

	// Create timestamped config filename with milliseconds to ensure uniqueness
	timestamp := time.Now().UTC().Format("20060102T150405.000Z")
	configFilename := fmt.Sprintf("zerb.lua.%s", timestamp)
	configPath := filepath.Join(zerbDir, "configs", configFilename)

	// Write config file
	if err := os.WriteFile(configPath, []byte(luaCode), 0644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	// Create .zerb-active marker file
	markerPath := filepath.Join(zerbDir, ".zerb-active")
	if err := os.WriteFile(markerPath, []byte(timestamp), 0644); err != nil {
		return fmt.Errorf("write marker file: %w", err)
	}

	// Create symlink to active config (idempotent: remove existing first)
	symlinkPath := filepath.Join(zerbDir, "zerb.lua.active")
	symlinkTarget := filepath.Join("configs", configFilename)

	// Remove existing symlink/file if present
	if err := os.Remove(symlinkPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing active symlink: %w", err)
	}

	// Create new symlink
	if err := os.Symlink(symlinkTarget, symlinkPath); err != nil {
		return fmt.Errorf("create symlink: %w", err)
	}

	return nil
}

// setupShellIntegration prompts user to set up shell integration
func setupShellIntegration(ctx context.Context, zerbDir string) error {
	shellManager, err := shell.NewManager(shell.Config{
		ZerbDir: zerbDir,
	})
	if err != nil {
		return fmt.Errorf("create shell manager: %w", err)
	}

	// Use interactive setup with auto-detection
	result, err := shellManager.DetectAndSetup(ctx, shell.SetupOptions{
		Interactive: true,
		Backup:      true,
		DryRun:      false,
	})
	if err != nil {
		return err
	}

	// Print result
	if result.Added {
		fmt.Printf("✓ Added shell integration to %s\n", result.RCFile)
		if result.BackupPath != "" {
			fmt.Printf("  Backup saved to: %s\n", result.BackupPath)
		}
	} else if result.AlreadyPresent {
		fmt.Printf("✓ Shell integration already present in %s\n", result.RCFile)
	}

	return nil
}

// printSuccessMessage prints the success message after initialization
func printSuccessMessage(zerbDir string) {
	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║  ZERB Initialization Complete!                             ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Restart your shell or run: source ~/.bashrc (or ~/.zshrc)")
	fmt.Println("  2. Add tools: zerb add <tool>")
	fmt.Println("  3. Track configs: zerb config add <path>")
	fmt.Println()
	fmt.Printf("ZERB directory: %s\n", zerbDir)
}

// runInit handles the `zerb init` subcommand
func runInit(args []string) error {
	// Create context with timeout (5 minutes for downloads)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Get ZERB directory
	zerbDir, err := getZerbDir()
	if err != nil {
		return fmt.Errorf("get ZERB directory: %w", err)
	}

	fmt.Println("🚀 Initializing ZERB...")
	fmt.Println()

	// Check if already initialized
	if isAlreadyInitialized(zerbDir) {
		return fmt.Errorf("ZERB already initialized at %s\nThe environment is already set up", zerbDir)
	}

	// Step 1: Create directory structure
	fmt.Printf("Creating directory structure...\n")
	if err := createDirectoryStructure(zerbDir); err != nil {
		return fmt.Errorf("create directories: %w", err)
	}
	fmt.Printf("✓ Created %s\n", zerbDir)
	fmt.Printf("✓ Created configs/ subdirectory\n")

	// Step 2: Detect platform
	fmt.Printf("\nDetecting platform...\n")
	platformInfo, err := detectPlatform(ctx)
	if err != nil {
		return fmt.Errorf("detect platform: %w", err)
	}
	distro := platformInfo.GetDistro()
	if distro != nil {
		fmt.Printf("✓ Detected %s (%s family, %s)\n", distro.ID, distro.Family, platformInfo.Arch)
	} else {
		fmt.Printf("✓ Detected %s, %s\n", platformInfo.OS, platformInfo.Arch)
	}

	// Step 3: Install binaries
	fmt.Printf("\nInstalling core components...\n")
	fmt.Printf("  Downloading tool manager and configuration manager...\n")
	if err := installBinaries(ctx, zerbDir, platformInfo); err != nil {
		return fmt.Errorf("install binaries: %w", err)
	}
	fmt.Printf("✓ Installed core components\n")
	fmt.Printf("✓ Extracted verification keys to %s/keyrings/\n", zerbDir)

	// Step 4: Generate initial config
	fmt.Printf("\nGenerating initial configuration...\n")
	if err := generateInitialConfig(ctx, zerbDir); err != nil {
		return fmt.Errorf("generate config: %w", err)
	}
	fmt.Printf("✓ Created initial config\n")

	// Step 5: Shell integration setup
	fmt.Printf("\nSetting up shell integration...\n")
	if err := setupShellIntegration(ctx, zerbDir); err != nil {
		// Non-fatal - user can do manually
		fmt.Printf("⚠  Shell integration setup failed: %v\n", err)
		fmt.Println("\nYou can manually add this to your shell rc file:")
		fmt.Printf("  eval \"$(zerb activate bash)\"  # or zsh, fish\n")
	}

	// Print success message
	printSuccessMessage(zerbDir)

	return nil
}

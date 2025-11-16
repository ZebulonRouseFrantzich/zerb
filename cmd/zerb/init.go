package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	// Check for sensitive data in generated config
	// Note: Empty initial config shouldn't have sensitive data, but we check anyway
	// This will be more important when users modify their configs
	findings := config.DetectSensitiveData(luaCode)
	if len(findings) > 0 {
		warning := config.FormatSensitiveDataWarning(findings)
		fmt.Fprint(os.Stderr, warning)
		// For init, we just warn but don't block since the initial config is safe
		// Users will see this warning if they later add sensitive data
	}

	// Create timestamped config filename with milliseconds to ensure uniqueness
	timestamp := time.Now().UTC().Format("20060102T150405.000Z")
	configFilename := fmt.Sprintf("zerb.lua.%s", timestamp)
	configPath := filepath.Join(zerbDir, "configs", configFilename)

	// Write config file (0600 for security - may contain sensitive data)
	if err := os.WriteFile(configPath, []byte(luaCode), 0600); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	// Create .zerb-active marker file (0600 for consistency)
	markerPath := filepath.Join(zerbDir, ".zerb-active")
	if err := os.WriteFile(markerPath, []byte(configFilename), 0600); err != nil {
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

// detectUserShell detects the user's shell without modifying any files
func detectUserShell() shell.ShellType {
	detection, err := shell.DetectShell()
	if err != nil || !detection.Shell.IsValid() {
		return shell.ShellUnknown
	}
	return detection.Shell
}

// checkZerbOnPath checks if 'zerb' command is accessible on PATH
func checkZerbOnPath() string {
	path, err := exec.LookPath("zerb")
	if err != nil {
		return ""
	}
	return path
}

// isOnPath checks if a directory is on the PATH by properly splitting and comparing paths
func isOnPath(dirPath string, pathEnv string) bool {
	// Clean and get absolute path for comparison
	cleanDir, err := filepath.Abs(filepath.Clean(dirPath))
	if err != nil {
		// If we can't resolve the path, fall back to simple comparison
		cleanDir = filepath.Clean(dirPath)
	}

	// Split PATH using OS-specific separator
	paths := strings.Split(pathEnv, string(os.PathListSeparator))
	for _, p := range paths {
		cleanPath, err := filepath.Abs(filepath.Clean(p))
		if err != nil {
			cleanPath = filepath.Clean(p)
		}
		if cleanPath == cleanDir {
			return true
		}
	}
	return false
}

// printPathWarning prints a warning if zerb is not on PATH
func printPathWarning() {
	homeDir, _ := os.UserHomeDir()

	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║  ⚠ Action Required: zerb not found on PATH                ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Before you can use ZERB, install the binary to your PATH.")
	fmt.Println()
	fmt.Println("Choose one:")
	fmt.Println()

	// Get current executable path
	exePath, err := os.Executable()

	// Option 1: ~/.local/bin
	fmt.Println("Option 1: Install to ~/.local/bin (recommended)")
	fmt.Println()
	if err == nil {
		fmt.Printf("  mkdir -p ~/.local/bin\n")
		fmt.Printf("  cp %s ~/.local/bin/zerb\n", exePath)
	} else {
		fmt.Println("  mkdir -p ~/.local/bin")
		fmt.Println("  cp $(which zerb) ~/.local/bin/zerb")
	}

	// Check if ~/.local/bin is on PATH
	pathEnv := os.Getenv("PATH")
	localBinPath := filepath.Join(homeDir, ".local", "bin")
	if !isOnPath(localBinPath, pathEnv) {
		fmt.Println()
		fmt.Println("  # If ~/.local/bin is not on PATH, add it:")
		fmt.Println("  echo 'export PATH=\"$HOME/.local/bin:$PATH\"' >> ~/.bashrc")
	}

	fmt.Println()

	// Option 2: System-wide
	fmt.Println("Option 2: Install system-wide")
	fmt.Println()
	if err == nil {
		fmt.Printf("  sudo cp %s /usr/local/bin/zerb\n", exePath)
	} else {
		fmt.Println("  sudo cp $(which zerb) /usr/local/bin/zerb")
	}

	fmt.Println()
	fmt.Println("After installing, verify:")
	fmt.Println()
	fmt.Println("  which zerb  # Should show the path to zerb")
	fmt.Println()
}

// printShellIntegrationInstructions prints instructions for manually adding shell integration
func printShellIntegrationInstructions(detectedShell shell.ShellType) {
	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║  Next: Add Shell Integration                              ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("AFTER installing zerb to PATH (see above), add shell integration:")
	fmt.Println()

	if detectedShell.IsValid() {
		// Show instructions for detected shell
		rcFile, _ := shell.GetRCFilePath(detectedShell)
		activationCmd, _ := shell.GenerateActivationCommand(detectedShell)

		fmt.Printf("  echo '%s' >> %s\n", activationCmd, rcFile)
		fmt.Println()
		fmt.Println("Then reload your shell:")
		fmt.Println()
		fmt.Printf("  source %s\n", rcFile)
		fmt.Println()
		fmt.Println("Finally, verify everything works:")
		fmt.Println()
		fmt.Println("  zerb --version")
		fmt.Println()
	} else {
		// Show instructions for all shells (detection failed)
		fmt.Println("Choose your shell:")
		fmt.Println()
		fmt.Println("  # For Bash:")
		bashCmd, _ := shell.GenerateActivationCommand(shell.ShellBash)
		bashRC, _ := shell.GetRCFilePath(shell.ShellBash)
		fmt.Printf("  echo '%s' >> %s\n", bashCmd, bashRC)
		fmt.Println()
		fmt.Println("  # For Zsh:")
		zshCmd, _ := shell.GenerateActivationCommand(shell.ShellZsh)
		zshRC, _ := shell.GetRCFilePath(shell.ShellZsh)
		fmt.Printf("  echo '%s' >> %s\n", zshCmd, zshRC)
		fmt.Println()
		fmt.Println("  # For Fish:")
		fishCmd, _ := shell.GenerateActivationCommand(shell.ShellFish)
		fishRC, _ := shell.GetRCFilePath(shell.ShellFish)
		fmt.Printf("  echo '%s' >> %s\n", fishCmd, fishRC)
		fmt.Println()
		fmt.Println("Then reload and verify:")
		fmt.Println()
		fmt.Println("  source ~/.bashrc  # or ~/.zshrc")
		fmt.Println("  zerb --version")
		fmt.Println()
	}
}

// printSuccessMessage prints the success message after initialization
func printSuccessMessage(zerbDir string, detectedShell shell.ShellType) {
	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║  ZERB Initialization Complete!                             ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("ZERB directory: %s\n", zerbDir)
	fmt.Println()

	// Get current executable path
	exePath, err := os.Executable()
	if err != nil {
		exePath = "zerb" // fallback
	}

	// Shell integration instructions with explicit sequencing
	fmt.Println("Next steps:")
	fmt.Println()

	fmt.Println("  1. FIRST, ensure zerb is installed to ~/.local/bin:")
	fmt.Println()
	fmt.Printf("     cp %s ~/.local/bin/zerb\n", exePath)
	fmt.Println()
	fmt.Println("     # Verify it's installed:")
	fmt.Println("     which zerb  # Should show: ~/.local/bin/zerb")
	fmt.Println()

	if detectedShell.IsValid() {
		rcFile, _ := shell.GetRCFilePath(detectedShell)
		activationCmd, _ := shell.GenerateActivationCommand(detectedShell)

		fmt.Printf("  2. THEN add shell integration to %s:\n", rcFile)
		fmt.Println()
		fmt.Printf("     echo '%s' >> %s\n", activationCmd, rcFile)
		fmt.Println()
		fmt.Println("  3. Reload your shell:")
		fmt.Println()
		fmt.Printf("     source %s\n", rcFile)
		fmt.Println()
	} else {
		fmt.Println("  2. THEN add shell integration (choose your shell):")
		fmt.Println()
		fmt.Println("     # For Bash:")
		bashCmd, _ := shell.GenerateActivationCommand(shell.ShellBash)
		bashRC, _ := shell.GetRCFilePath(shell.ShellBash)
		fmt.Printf("     echo '%s' >> %s\n", bashCmd, bashRC)
		fmt.Println()
		fmt.Println("     # For Zsh:")
		zshCmd, _ := shell.GenerateActivationCommand(shell.ShellZsh)
		zshRC, _ := shell.GetRCFilePath(shell.ShellZsh)
		fmt.Printf("     echo '%s' >> %s\n", zshCmd, zshRC)
		fmt.Println()
		fmt.Println("     # For Fish:")
		fishCmd, _ := shell.GenerateActivationCommand(shell.ShellFish)
		fishRC, _ := shell.GetRCFilePath(shell.ShellFish)
		fmt.Printf("     echo '%s' >> %s\n", fishCmd, fishRC)
		fmt.Println()
		fmt.Println("  3. Reload your shell:")
		fmt.Println()
		fmt.Println("     source ~/.bashrc  # or ~/.zshrc")
		fmt.Println()
	}

	fmt.Println("  4. Verify everything works:")
	fmt.Println()
	fmt.Println("     zerb --version")
	fmt.Println()
	fmt.Println("  5. Start using ZERB:")
	fmt.Println()
	fmt.Println("     zerb add node@20")
	fmt.Println("     zerb config add ~/.zshrc")
	fmt.Println()
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

	// Step 5: Detect shell (for showing appropriate instructions)
	detectedShell := detectUserShell()

	// Step 6: Check if zerb is on PATH and show appropriate success message
	zerbPath := checkZerbOnPath()
	if zerbPath == "" {
		// zerb is not on PATH - print warning with install instructions
		printPathWarning()
		fmt.Println()
		// Still show shell integration instructions after PATH warning
		printShellIntegrationInstructions(detectedShell)
	} else {
		// zerb is on PATH - print success message with shell integration instructions
		printSuccessMessage(zerbDir, detectedShell)
	}

	return nil
}

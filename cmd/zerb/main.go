package main

import (
	"fmt"
	"os"
)

// Version will be set at build time via -ldflags
var Version = "v0.1.0-alpha"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("ZERB %s\n", Version)
		fmt.Println("Zero-hassle Effortless Reproducible Builds")
		return
	}

	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║  ZERB - Zero-hassle Effortless Reproducible Builds      ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("ZERB is currently in active development (pre-pre-alpha).")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  zerb --version    Show version information")
	fmt.Println()
	fmt.Println("Coming soon:")
	fmt.Println("  zerb init        Initialize ZERB configuration")
	fmt.Println("  zerb add         Add tools to your environment")
	fmt.Println("  zerb sync        Sync tools and configs")
	fmt.Println("  zerb drift       Check for environment drift")
}

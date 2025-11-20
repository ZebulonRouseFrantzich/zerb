package main

import (
	"fmt"
	"os"
)

// Version will be set at build time via -ldflags
var Version = "v0.0.1-alpha"

func main() {
	// Handle subcommands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version":
			fmt.Printf("ZERB %s\n", Version)
			fmt.Println("Zero-hassle Effortless Reproducible Builds")
			return
		case "activate":
			// Handle zerb activate subcommand
			if err := runActivate(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "init":
			// Handle zerb init subcommand
			if err := runInit(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "uninit":
			// Handle zerb uninit subcommand
			if err := runUninit(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "drift":
			// Handle zerb drift subcommand
			if err := runDrift(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "config":
			// Handle zerb config subcommand
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "Error: config subcommand requires an action")
				fmt.Fprintln(os.Stderr, "Usage: zerb config add [options] <path>...")
				fmt.Fprintln(os.Stderr, "       zerb config list [options]")
				os.Exit(1)
			}
			switch os.Args[2] {
			case "add":
				if err := runConfigAdd(os.Args[3:]); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
			case "list":
				if err := runConfigList(os.Args[3:]); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
			default:
				fmt.Fprintf(os.Stderr, "Error: unknown config action: %s\n", os.Args[2])
				fmt.Fprintln(os.Stderr, "Usage: zerb config add [options] <path>...")
				fmt.Fprintln(os.Stderr, "       zerb config list [options]")
				os.Exit(1)
			}
			return
		}
	}

	// Default: show help
	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║  ZERB - Zero-hassle Effortless Reproducible Builds      ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("ZERB is currently in active development (pre-pre-alpha).")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  zerb --version             Show version information")
	fmt.Println("  zerb init                  Initialize ZERB environment")
	fmt.Println("  zerb uninit                Remove ZERB from your system")
	fmt.Println("  zerb activate <shell>      Generate shell activation script (bash, zsh, fish)")
	fmt.Println("  zerb drift [options]       Check for environment drift")
	fmt.Println("  zerb config add [options]  Add config files to tracking")
	fmt.Println("  zerb config list [options] List tracked config files")
	fmt.Println()
	fmt.Println("Coming soon:")
	fmt.Println("  zerb add                   Add tools to your environment")
	fmt.Println("  zerb sync                  Sync tools and configs")
}

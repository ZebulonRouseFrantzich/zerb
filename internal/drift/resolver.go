package drift

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ResolutionMode represents how to resolve drifts
type ResolutionMode int

const (
	ResolutionIndividual ResolutionMode = iota
	ResolutionAdoptAll
	ResolutionRevertAll
	ResolutionShowOnly
	ResolutionExit
)

func (r ResolutionMode) String() string {
	switch r {
	case ResolutionIndividual:
		return "Individual"
	case ResolutionAdoptAll:
		return "Adopt All"
	case ResolutionRevertAll:
		return "Revert All"
	case ResolutionShowOnly:
		return "Show Only"
	case ResolutionExit:
		return "Exit"
	default:
		return "Unknown"
	}
}

// PromptResolutionMode prompts user for resolution mode
func PromptResolutionMode() (ResolutionMode, error) {
	fmt.Println("\nHow would you like to resolve these drifts?")
	fmt.Println("  1. Resolve individually (choose action for each drift)")
	fmt.Println("  2. Adopt all changes (update baseline to match environment)")
	fmt.Println("  3. Revert all changes (restore environment to match baseline)")
	fmt.Println("  4. Show details only (no changes)")
	fmt.Println("  5. Exit")
	fmt.Print("\nChoice [1]: ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return ResolutionExit, fmt.Errorf("read input: %w", err)
	}

	input = strings.TrimSpace(input)
	if input == "" {
		input = "1" // Default to individual
	}

	switch input {
	case "1":
		return ResolutionIndividual, nil
	case "2":
		return ResolutionAdoptAll, nil
	case "3":
		return ResolutionRevertAll, nil
	case "4":
		return ResolutionShowOnly, nil
	case "5":
		return ResolutionExit, nil
	default:
		return ResolutionExit, fmt.Errorf("invalid choice: %s", input)
	}
}

// DriftAction represents an action to take for a drift
type DriftAction int

const (
	ActionAdopt  DriftAction = iota // Update baseline to match environment
	ActionRevert                    // Restore environment to match baseline
	ActionSkip                      // Skip this drift
)

func (a DriftAction) String() string {
	switch a {
	case ActionAdopt:
		return "Adopt"
	case ActionRevert:
		return "Revert"
	case ActionSkip:
		return "Skip"
	default:
		return "Unknown"
	}
}

// PromptDriftAction prompts user for action on a single drift
func PromptDriftAction(result DriftResult) (DriftAction, error) {
	fmt.Printf("\n%s\n", formatDriftEntry(result))
	fmt.Println("\nWhat would you like to do?")

	switch result.DriftType {
	case DriftExternalOverride:
		fmt.Println("  1. Adopt (remove from baseline, acknowledge external management)")
		fmt.Println("  2. Revert (reinstall via ZERB, may conflict with system)")
		fmt.Println("  3. Skip (decide later)")

	case DriftVersionMismatch:
		fmt.Println("  1. Adopt (update baseline to match current version)")
		fmt.Println("  2. Revert (reinstall correct version)")
		fmt.Println("  3. Skip (decide later)")

	case DriftMissing:
		fmt.Println("  1. Revert (install missing tool)")
		fmt.Println("  2. Adopt (remove from baseline)")
		fmt.Println("  3. Skip (decide later)")

	case DriftExtra:
		fmt.Println("  1. Adopt (add to baseline)")
		fmt.Println("  2. Revert (uninstall tool)")
		fmt.Println("  3. Skip (decide later)")

	case DriftManagedButNotActive:
		fmt.Println("  1. Skip (PATH issue, needs manual investigation)")
		fmt.Println("  2. Adopt (remove from baseline)")
		fmt.Println("  3. Revert (not applicable)")

	case DriftVersionUnknown:
		fmt.Println("  1. Skip (version detection failed, needs manual investigation)")
		fmt.Println("  2. Adopt (remove from baseline)")
		fmt.Println("  3. Revert (reinstall to fix version detection)")
	}

	fmt.Print("\nChoice [1]: ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return ActionSkip, fmt.Errorf("read input: %w", err)
	}

	input = strings.TrimSpace(input)
	if input == "" {
		input = "1" // Default to first option
	}

	switch input {
	case "1":
		// Default action depends on drift type
		if result.DriftType == DriftMissing {
			return ActionRevert, nil // Install missing tool
		}
		if result.DriftType == DriftManagedButNotActive || result.DriftType == DriftVersionUnknown {
			return ActionSkip, nil // Skip by default for these types
		}
		return ActionAdopt, nil // Default for others
	case "2":
		if result.DriftType == DriftMissing {
			return ActionAdopt, nil // Remove from baseline
		}
		if result.DriftType == DriftManagedButNotActive {
			return ActionAdopt, nil // Remove from baseline
		}
		if result.DriftType == DriftVersionUnknown {
			return ActionAdopt, nil // Remove from baseline
		}
		return ActionRevert, nil // Revert for others
	case "3":
		if result.DriftType == DriftManagedButNotActive {
			return ActionRevert, nil // Not applicable, treat as skip
		}
		if result.DriftType == DriftVersionUnknown {
			return ActionRevert, nil // Reinstall
		}
		return ActionSkip, nil
	default:
		return ActionSkip, fmt.Errorf("invalid choice: %s", input)
	}
}

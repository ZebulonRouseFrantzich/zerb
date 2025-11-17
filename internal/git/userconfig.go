package git

import (
	"os"
)

// DetectGitUser detects git user configuration from environment variables.
// It implements a three-tier fallback system:
// 1. ZERB-specific environment variables (ZERB_GIT_NAME, ZERB_GIT_EMAIL)
// 2. Standard git environment variables (GIT_AUTHOR_NAME, GIT_AUTHOR_EMAIL)
// 3. Placeholder values (ZERB User, zerb@localhost)
//
// Each tier requires BOTH name AND email to be set (all-or-nothing).
// If only one value is present in a tier, that tier is skipped entirely.
//
// This function never reads global git config to maintain ZERB isolation.
func DetectGitUser() GitUserInfo {
	// 1. Try ZERB-specific environment variables first (requires both name and email)
	zerbName := os.Getenv("ZERB_GIT_NAME")
	zerbEmail := os.Getenv("ZERB_GIT_EMAIL")
	if zerbName != "" && zerbEmail != "" {
		return GitUserInfo{
			Name:       zerbName,
			Email:      zerbEmail,
			FromEnv:    true,
			FromConfig: false,
			IsDefault:  false,
		}
	}

	// 2. Try standard git environment variables (requires both name and email)
	gitName := os.Getenv("GIT_AUTHOR_NAME")
	gitEmail := os.Getenv("GIT_AUTHOR_EMAIL")
	if gitName != "" && gitEmail != "" {
		return GitUserInfo{
			Name:       gitName,
			Email:      gitEmail,
			FromEnv:    true,
			FromConfig: false,
			IsDefault:  false,
		}
	}

	// 3. Fallback to placeholders
	return GitUserInfo{
		Name:       "ZERB User",
		Email:      "zerb@localhost",
		FromEnv:    false,
		FromConfig: false,
		IsDefault:  true,
	}
}

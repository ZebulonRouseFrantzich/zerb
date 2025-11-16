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
// This function never reads global git config to maintain ZERB isolation.
func DetectGitUser() GitUserInfo {
	// 1. Try ZERB-specific environment variables first
	if name := os.Getenv("ZERB_GIT_NAME"); name != "" {
		email := os.Getenv("ZERB_GIT_EMAIL")
		if email == "" {
			email = "zerb@localhost" // Fallback email if name is set but email isn't
		}
		return GitUserInfo{
			Name:       name,
			Email:      email,
			FromEnv:    true,
			FromConfig: false,
			IsDefault:  false,
		}
	}

	// 2. Try standard git environment variables
	if name := os.Getenv("GIT_AUTHOR_NAME"); name != "" {
		email := os.Getenv("GIT_AUTHOR_EMAIL")
		if email == "" {
			email = "git@localhost" // Fallback email if name is set but email isn't
		}
		return GitUserInfo{
			Name:       name,
			Email:      email,
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

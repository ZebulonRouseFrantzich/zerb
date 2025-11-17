package git

import (
	"fmt"
	"os"
	"path/filepath"
)

// gitignoreTemplate is the .gitignore template for ZERB repositories.
// It excludes runtime files, generated configs, and derived state while
// tracking source configuration and user dotfiles.
const gitignoreTemplate = `# ZERB .gitignore
# This file is managed by ZERB. Edit with caution.

# Tracked: User configuration (source of truth)
# - configs/                 Timestamped configuration snapshots
# - chezmoi/source/          User's dotfiles

# Runtime artifacts (not tracked)
bin/
cache/
tmp/
logs/

# Tool state (not tracked)
mise/

# Transaction state (ephemeral)
.txn/

# Development environment
.direnv/

# Generated configs (derived from zerb.lua)
mise/config.toml
chezmoi/config.toml

# Symlinks (recreated locally)
zerb.lua.active

# Deprecated marker (to be removed)
.zerb-active

# Embedded/extracted keyrings (identical on all machines)
keyrings/

# Git unavailable marker
.zerb-no-git
`

// WriteGitignore writes the .gitignore template to the specified path.
// It creates parent directories if needed and sets file permissions to 0644.
func WriteGitignore(path string) error {
	// Create parent directories if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	// Write .gitignore file
	if err := os.WriteFile(path, []byte(gitignoreTemplate), 0644); err != nil {
		return fmt.Errorf("write .gitignore: %w", err)
	}

	return nil
}

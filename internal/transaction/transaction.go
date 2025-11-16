// Package transaction provides robust transaction management for config operations
// with locking, atomic writes, and recovery support.
package transaction

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// State represents the current state of a transaction or path operation.
type State string

const (
	StatePending    State = "pending"
	StateInProgress State = "in_progress"
	StateCompleted  State = "completed"
	StateFailed     State = "failed"
)

// ConfigAddTxn represents a transaction for adding configuration files.
type ConfigAddTxn struct {
	Version       int       `json:"version"` // Schema version for future evolution
	ID            string    `json:"id"`      // UUID for unique identification
	Operation     string    `json:"operation"`
	Timestamp     time.Time `json:"timestamp"`
	Paths         []PathTxn `json:"paths"`
	ConfigUpdated bool      `json:"config_updated"`
	GitCommitted  bool      `json:"git_committed"`
}

// PathTxn represents the transaction state for a single path.
type PathTxn struct {
	Path               string   `json:"path"`
	State              State    `json:"state"`
	Recursive          bool     `json:"recursive"`
	Template           bool     `json:"template"`
	Secrets            bool     `json:"secrets"`
	Private            bool     `json:"private"`
	CreatedSourceFiles []string `json:"created_source_files"` // For cleanup on abort
	LastError          string   `json:"last_error,omitempty"`
}

// New creates a new transaction for adding config files.
func New(paths []string, opts map[string]AddOptions) *ConfigAddTxn {
	pathTxns := make([]PathTxn, 0, len(paths))

	for _, path := range paths {
		opt := opts[path]
		pathTxns = append(pathTxns, PathTxn{
			Path:               path,
			State:              StatePending,
			Recursive:          opt.Recursive,
			Template:           opt.Template,
			Secrets:            opt.Secrets,
			Private:            opt.Private,
			CreatedSourceFiles: []string{},
		})
	}

	return &ConfigAddTxn{
		Version:       1,
		ID:            uuid.New().String(),
		Operation:     "config-add",
		Timestamp:     time.Now().UTC(),
		Paths:         pathTxns,
		ConfigUpdated: false,
		GitCommitted:  false,
	}
}

// AddOptions holds options for adding a config file.
// This is separate from chezmoi.AddOptions to keep transaction package independent.
type AddOptions struct {
	Recursive bool
	Template  bool
	Secrets   bool
	Private   bool
}

// Save writes the transaction to disk atomically.
// Uses write-then-rename pattern for atomicity.
func (t *ConfigAddTxn) Save(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create transaction directory: %w", err)
	}

	filename := fmt.Sprintf("txn-config-add-%s.json", t.ID)
	finalPath := filepath.Join(dir, filename)
	tmpPath := finalPath + ".tmp"

	// Marshal to JSON
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal transaction: %w", err)
	}

	// Write to temporary file
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("write temporary transaction file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, finalPath); err != nil {
		os.Remove(tmpPath) // Clean up temp file on error
		return fmt.Errorf("rename transaction file: %w", err)
	}

	// Sync directory for durability
	df, err := os.Open(dir)
	if err == nil {
		if syncErr := df.Sync(); syncErr != nil {
			df.Close()
			return fmt.Errorf("sync directory: %w", syncErr)
		}
		df.Close()
	}

	return nil
}

// Load reads a transaction from disk.
func Load(path string) (*ConfigAddTxn, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read transaction file: %w", err)
	}

	var txn ConfigAddTxn
	if err := json.Unmarshal(data, &txn); err != nil {
		return nil, fmt.Errorf("unmarshal transaction: %w", err)
	}

	return &txn, nil
}

// UpdatePathState updates the state of a specific path in the transaction.
func (t *ConfigAddTxn) UpdatePathState(path string, state State, createdFiles []string, err error) {
	for i := range t.Paths {
		if t.Paths[i].Path == path {
			t.Paths[i].State = state
			if len(createdFiles) > 0 {
				t.Paths[i].CreatedSourceFiles = createdFiles
			}
			if err != nil {
				t.Paths[i].LastError = err.Error()
			} else {
				t.Paths[i].LastError = ""
			}
			break
		}
	}
}

// HasPendingPaths returns true if there are paths in pending or failed state.
func (t *ConfigAddTxn) HasPendingPaths() bool {
	for _, p := range t.Paths {
		if p.State == StatePending || p.State == StateFailed {
			return true
		}
	}
	return false
}

// AllPathsCompleted returns true if all paths are in completed state.
func (t *ConfigAddTxn) AllPathsCompleted() bool {
	for _, p := range t.Paths {
		if p.State != StateCompleted {
			return false
		}
	}
	return len(t.Paths) > 0
}

// GetCreatedFiles returns all files created during this transaction (for cleanup).
func (t *ConfigAddTxn) GetCreatedFiles() []string {
	var files []string
	for _, p := range t.Paths {
		files = append(files, p.CreatedSourceFiles...)
	}
	return files
}

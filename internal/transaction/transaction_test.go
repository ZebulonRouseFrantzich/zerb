package transaction

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewConfigTxn(t *testing.T) {
	t.Run("creates transaction with add operation", func(t *testing.T) {
		paths := []string{"~/.zshrc", "~/.gitconfig"}
		opts := map[string]AddOptions{
			"~/.zshrc":     {Recursive: false, Template: true},
			"~/.gitconfig": {Recursive: false, Private: true},
		}

		txn := New(paths, opts)

		if txn.Version != 1 {
			t.Errorf("expected version 1, got %d", txn.Version)
		}
		if txn.Operation != OperationAdd {
			t.Errorf("expected operation %q, got %q", OperationAdd, txn.Operation)
		}
		if txn.ID == "" {
			t.Error("expected non-empty ID")
		}
		if len(txn.Paths) != 2 {
			t.Errorf("expected 2 paths, got %d", len(txn.Paths))
		}
		if txn.Paths[0].Path != "~/.zshrc" {
			t.Errorf("expected path ~/.zshrc, got %s", txn.Paths[0].Path)
		}
		if txn.Paths[0].State != StatePending {
			t.Errorf("expected state pending, got %s", txn.Paths[0].State)
		}
		if !txn.Paths[0].Template {
			t.Error("expected template to be true for ~/.zshrc")
		}
		if !txn.Paths[1].Private {
			t.Error("expected private to be true for ~/.gitconfig")
		}
	})

	t.Run("creates transaction with remove operation", func(t *testing.T) {
		paths := []string{"~/.zshrc"}
		opts := map[string]RemoveOptions{
			"~/.zshrc": {Purge: true},
		}

		txn := NewRemove(paths, opts)

		if txn.Operation != OperationRemove {
			t.Errorf("expected operation %q, got %q", OperationRemove, txn.Operation)
		}
		if len(txn.Paths) != 1 {
			t.Errorf("expected 1 path, got %d", len(txn.Paths))
		}
		if !txn.Paths[0].Purge {
			t.Error("expected purge to be true")
		}
	})
}

func TestConfigTxnSave(t *testing.T) {
	t.Run("saves transaction to disk", func(t *testing.T) {
		dir := t.TempDir()
		txn := New([]string{"~/.zshrc"}, map[string]AddOptions{
			"~/.zshrc": {},
		})

		err := txn.Save(dir)
		if err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		// Verify file exists
		expectedFile := filepath.Join(dir, "txn-config-add-"+txn.ID+".json")
		if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
			t.Error("transaction file not created")
		}

		// Verify file content
		data, err := os.ReadFile(expectedFile)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}

		var loaded ConfigTxn
		if err := json.Unmarshal(data, &loaded); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if loaded.ID != txn.ID {
			t.Errorf("expected ID %s, got %s", txn.ID, loaded.ID)
		}
	})

	t.Run("saves remove transaction with correct filename", func(t *testing.T) {
		dir := t.TempDir()
		txn := NewRemove([]string{"~/.zshrc"}, map[string]RemoveOptions{
			"~/.zshrc": {},
		})

		err := txn.Save(dir)
		if err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		// Verify file exists with remove prefix
		expectedFile := filepath.Join(dir, "txn-config-remove-"+txn.ID+".json")
		if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
			t.Error("transaction file not created with correct name")
		}
	})
}

func TestConfigTxnLoad(t *testing.T) {
	t.Run("loads transaction from disk", func(t *testing.T) {
		dir := t.TempDir()
		original := New([]string{"~/.zshrc"}, map[string]AddOptions{
			"~/.zshrc": {Template: true},
		})
		original.UpdatePathState("~/.zshrc", StateCompleted, []string{"source/dot_zshrc"}, nil)

		if err := original.Save(dir); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		filePath := filepath.Join(dir, "txn-config-add-"+original.ID+".json")
		loaded, err := Load(filePath)
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		if loaded.ID != original.ID {
			t.Errorf("ID mismatch: expected %s, got %s", original.ID, loaded.ID)
		}
		if loaded.Operation != original.Operation {
			t.Errorf("Operation mismatch: expected %s, got %s", original.Operation, loaded.Operation)
		}
		if loaded.Paths[0].State != StateCompleted {
			t.Errorf("expected state completed, got %s", loaded.Paths[0].State)
		}
	})

	t.Run("returns error for non-existent file", func(t *testing.T) {
		_, err := Load("/non/existent/file.json")
		if err == nil {
			t.Error("expected error for non-existent file")
		}
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "invalid.json")
		os.WriteFile(filePath, []byte("not json"), 0600)

		_, err := Load(filePath)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestConfigTxnUpdatePathState(t *testing.T) {
	t.Run("updates path state", func(t *testing.T) {
		txn := New([]string{"~/.zshrc", "~/.gitconfig"}, map[string]AddOptions{
			"~/.zshrc":     {},
			"~/.gitconfig": {},
		})

		txn.UpdatePathState("~/.zshrc", StateInProgress, nil, nil)
		if txn.Paths[0].State != StateInProgress {
			t.Errorf("expected in_progress, got %s", txn.Paths[0].State)
		}

		createdFiles := []string{"source/dot_zshrc"}
		txn.UpdatePathState("~/.zshrc", StateCompleted, createdFiles, nil)
		if txn.Paths[0].State != StateCompleted {
			t.Errorf("expected completed, got %s", txn.Paths[0].State)
		}
		if len(txn.Paths[0].CreatedSourceFiles) != 1 {
			t.Error("expected created files to be set")
		}
	})

	t.Run("records error on failure", func(t *testing.T) {
		txn := New([]string{"~/.zshrc"}, map[string]AddOptions{
			"~/.zshrc": {},
		})

		testErr := &testError{msg: "permission denied"}
		txn.UpdatePathState("~/.zshrc", StateFailed, nil, testErr)

		if txn.Paths[0].State != StateFailed {
			t.Errorf("expected failed, got %s", txn.Paths[0].State)
		}
		if txn.Paths[0].LastError != "permission denied" {
			t.Errorf("expected error message, got %s", txn.Paths[0].LastError)
		}
	})

	t.Run("ignores unknown path", func(t *testing.T) {
		txn := New([]string{"~/.zshrc"}, map[string]AddOptions{
			"~/.zshrc": {},
		})

		// Should not panic
		txn.UpdatePathState("~/.unknown", StateCompleted, nil, nil)
		if txn.Paths[0].State != StatePending {
			t.Error("should not update state for unknown path")
		}
	})
}

func TestConfigTxnHasPendingPaths(t *testing.T) {
	t.Run("returns true when paths are pending", func(t *testing.T) {
		txn := New([]string{"~/.zshrc"}, map[string]AddOptions{
			"~/.zshrc": {},
		})

		if !txn.HasPendingPaths() {
			t.Error("expected HasPendingPaths to be true")
		}
	})

	t.Run("returns true when paths are failed", func(t *testing.T) {
		txn := New([]string{"~/.zshrc"}, map[string]AddOptions{
			"~/.zshrc": {},
		})
		txn.UpdatePathState("~/.zshrc", StateFailed, nil, nil)

		if !txn.HasPendingPaths() {
			t.Error("expected HasPendingPaths to be true for failed paths")
		}
	})

	t.Run("returns false when all completed", func(t *testing.T) {
		txn := New([]string{"~/.zshrc"}, map[string]AddOptions{
			"~/.zshrc": {},
		})
		txn.UpdatePathState("~/.zshrc", StateCompleted, nil, nil)

		if txn.HasPendingPaths() {
			t.Error("expected HasPendingPaths to be false")
		}
	})
}

func TestConfigTxnAllPathsCompleted(t *testing.T) {
	t.Run("returns false when paths are pending", func(t *testing.T) {
		txn := New([]string{"~/.zshrc"}, map[string]AddOptions{
			"~/.zshrc": {},
		})

		if txn.AllPathsCompleted() {
			t.Error("expected AllPathsCompleted to be false")
		}
	})

	t.Run("returns true when all completed", func(t *testing.T) {
		txn := New([]string{"~/.zshrc", "~/.gitconfig"}, map[string]AddOptions{
			"~/.zshrc":     {},
			"~/.gitconfig": {},
		})
		txn.UpdatePathState("~/.zshrc", StateCompleted, nil, nil)
		txn.UpdatePathState("~/.gitconfig", StateCompleted, nil, nil)

		if !txn.AllPathsCompleted() {
			t.Error("expected AllPathsCompleted to be true")
		}
	})

	t.Run("returns false for empty transaction", func(t *testing.T) {
		txn := &ConfigTxn{Paths: []PathTxn{}}

		if txn.AllPathsCompleted() {
			t.Error("expected AllPathsCompleted to be false for empty transaction")
		}
	})
}

func TestConfigTxnGetCreatedFiles(t *testing.T) {
	txn := New([]string{"~/.zshrc", "~/.gitconfig"}, map[string]AddOptions{
		"~/.zshrc":     {},
		"~/.gitconfig": {},
	})
	txn.UpdatePathState("~/.zshrc", StateCompleted, []string{"source/dot_zshrc"}, nil)
	txn.UpdatePathState("~/.gitconfig", StateCompleted, []string{"source/dot_gitconfig"}, nil)

	files := txn.GetCreatedFiles()
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}
}

func TestBackwardCompatibility(t *testing.T) {
	t.Run("loads old ConfigAddTxn format", func(t *testing.T) {
		// Simulate old format JSON (without operation field or with old name)
		oldFormat := `{
			"version": 1,
			"id": "test-uuid",
			"operation": "config-add",
			"timestamp": "2025-01-01T00:00:00Z",
			"paths": [
				{
					"path": "~/.zshrc",
					"state": "completed",
					"recursive": false,
					"template": true,
					"secrets": false,
					"private": false,
					"created_source_files": ["dot_zshrc"]
				}
			],
			"config_updated": true,
			"git_committed": true
		}`

		dir := t.TempDir()
		filePath := filepath.Join(dir, "old-format.json")
		if err := os.WriteFile(filePath, []byte(oldFormat), 0600); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		txn, err := Load(filePath)
		if err != nil {
			t.Fatalf("failed to load old format: %v", err)
		}

		if txn.ID != "test-uuid" {
			t.Errorf("expected ID test-uuid, got %s", txn.ID)
		}
		if txn.Operation != OperationAdd {
			t.Errorf("expected operation %s, got %s", OperationAdd, txn.Operation)
		}
		if txn.Paths[0].State != StateCompleted {
			t.Errorf("expected completed state, got %s", txn.Paths[0].State)
		}
	})
}

func TestRemoveOptions(t *testing.T) {
	t.Run("creates remove transaction with purge option", func(t *testing.T) {
		paths := []string{"~/.zshrc"}
		opts := map[string]RemoveOptions{
			"~/.zshrc": {Purge: true},
		}

		txn := NewRemove(paths, opts)

		if txn.Paths[0].Purge != true {
			t.Error("expected purge to be true")
		}
	})
}

func TestConfigTxnTimestamp(t *testing.T) {
	before := time.Now().UTC()
	txn := New([]string{"~/.zshrc"}, map[string]AddOptions{"~/.zshrc": {}})
	after := time.Now().UTC()

	if txn.Timestamp.Before(before) || txn.Timestamp.After(after) {
		t.Error("timestamp should be between before and after")
	}
}

// testError implements error interface for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

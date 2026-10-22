package main

import (
	"os"
	"path/filepath"
	"testing"
)

// VAL-002: no proof root is created before the repository exclusion is captured.
func TestRepositoryRequiredBeforeProofRoot(t *testing.T) {
	original := os.Args
	t.Cleanup(func() { os.Args = original })
	for _, repository := range []string{"", filepath.Join(t.TempDir(), "missing-repository")} {
		root := t.TempDir()
		os.Args = []string{"schemaproof", "--root", root}
		if repository != "" {
			os.Args = append(os.Args, "--repository", repository)
		}
		if got := run(); got != 1 {
			t.Fatalf("repository exclusion was not required: status=%d", got)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			t.Fatalf("proof root touched before exclusion admission: entries=%d err=%v", len(entries), err)
		}
	}
}

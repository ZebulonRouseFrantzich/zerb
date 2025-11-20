package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestConfigStatus tests the ConfigStatus type and its String method.
func TestConfigStatus(t *testing.T) {
	tests := []struct {
		name   string
		status ConfigStatus
		want   string
	}{
		{
			name:   "synced status",
			status: StatusSynced,
			want:   "synced",
		},
		{
			name:   "missing status",
			status: StatusMissing,
			want:   "missing",
		},
		{
			name:   "partial status",
			status: StatusPartial,
			want:   "partial",
		},
		{
			name:   "unknown status",
			status: ConfigStatus(999),
			want:   "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.String()
			if got != tt.want {
				t.Errorf("ConfigStatus.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestConfigStatus_Symbol tests the Symbol method.
func TestConfigStatus_Symbol(t *testing.T) {
	tests := []struct {
		name   string
		status ConfigStatus
		want   string
	}{
		{
			name:   "synced symbol",
			status: StatusSynced,
			want:   "✓",
		},
		{
			name:   "missing symbol",
			status: StatusMissing,
			want:   "✗",
		},
		{
			name:   "partial symbol",
			status: StatusPartial,
			want:   "?",
		},
		{
			name:   "unknown symbol",
			status: ConfigStatus(999),
			want:   "?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.Symbol()
			if got != tt.want {
				t.Errorf("ConfigStatus.Symbol() = %q, want %q", got, tt.want)
			}
		})
	}
}

// mockChezmoi implements the Chezmoi interface for testing.
type mockChezmoi struct {
	hasFileFunc func(ctx context.Context, path string) (bool, error)
}

func (m *mockChezmoi) HasFile(ctx context.Context, path string) (bool, error) {
	if m.hasFileFunc != nil {
		return m.hasFileFunc(ctx, path)
	}
	return false, nil
}

// TestDefaultStatusDetector_Synced tests detection of synced configs.
func TestDefaultStatusDetector_Synced(t *testing.T) {
	// Create temp directory for testing
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.conf")

	// Create test file
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Mock chezmoi that returns true (file is managed)
	mockCm := &mockChezmoi{
		hasFileFunc: func(ctx context.Context, path string) (bool, error) {
			return true, nil
		},
	}

	detector := NewDefaultStatusDetector(mockCm)
	configs := []ConfigFile{
		{Path: testFile},
	}

	ctx := context.Background()
	results, err := detector.DetectStatus(ctx, configs)
	if err != nil {
		t.Fatalf("DetectStatus() error = %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Status != StatusSynced {
		t.Errorf("expected status %q, got %q", StatusSynced, results[0].Status)
	}
}

// TestDefaultStatusDetector_Missing tests detection of missing configs.
func TestDefaultStatusDetector_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	missingFile := filepath.Join(tmpDir, "missing.conf")

	// Mock chezmoi (shouldn't be called for missing files)
	mockCm := &mockChezmoi{
		hasFileFunc: func(ctx context.Context, path string) (bool, error) {
			t.Error("HasFile should not be called for missing files")
			return false, nil
		},
	}

	detector := NewDefaultStatusDetector(mockCm)
	configs := []ConfigFile{
		{Path: missingFile},
	}

	ctx := context.Background()
	results, err := detector.DetectStatus(ctx, configs)
	if err != nil {
		t.Fatalf("DetectStatus() error = %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Status != StatusMissing {
		t.Errorf("expected status %q, got %q", StatusMissing, results[0].Status)
	}
}

// TestDefaultStatusDetector_Partial tests detection of partial configs.
func TestDefaultStatusDetector_Partial(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.conf")

	// Create test file
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Mock chezmoi that returns false (file is NOT managed)
	mockCm := &mockChezmoi{
		hasFileFunc: func(ctx context.Context, path string) (bool, error) {
			return false, nil
		},
	}

	detector := NewDefaultStatusDetector(mockCm)
	configs := []ConfigFile{
		{Path: testFile},
	}

	ctx := context.Background()
	results, err := detector.DetectStatus(ctx, configs)
	if err != nil {
		t.Fatalf("DetectStatus() error = %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Status != StatusPartial {
		t.Errorf("expected status %q, got %q", StatusPartial, results[0].Status)
	}
}

// TestDefaultStatusDetector_ChezmoiError tests error handling.
func TestDefaultStatusDetector_ChezmoiError(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.conf")

	// Create test file
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Mock chezmoi that returns an error
	expectedErr := errors.New("chezmoi error")
	mockCm := &mockChezmoi{
		hasFileFunc: func(ctx context.Context, path string) (bool, error) {
			return false, expectedErr
		},
	}

	detector := NewDefaultStatusDetector(mockCm)
	configs := []ConfigFile{
		{Path: testFile},
	}

	ctx := context.Background()
	_, err := detector.DetectStatus(ctx, configs)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error to wrap %v, got %v", expectedErr, err)
	}
}

// TestDefaultStatusDetector_ContextCancellation tests context cancellation.
func TestDefaultStatusDetector_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.conf")

	// Create test file
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mockCm := &mockChezmoi{
		hasFileFunc: func(ctx context.Context, path string) (bool, error) {
			// Check context
			if err := ctx.Err(); err != nil {
				return false, err
			}
			return true, nil
		},
	}

	detector := NewDefaultStatusDetector(mockCm)
	configs := []ConfigFile{
		{Path: testFile},
	}

	_, err := detector.DetectStatus(ctx, configs)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

// TestDefaultStatusDetector_MultipleConfigs tests detection with multiple configs.
func TestDefaultStatusDetector_MultipleConfigs(t *testing.T) {
	tmpDir := t.TempDir()

	syncedFile := filepath.Join(tmpDir, "synced.conf")
	partialFile := filepath.Join(tmpDir, "partial.conf")
	missingFile := filepath.Join(tmpDir, "missing.conf")

	// Create only synced and partial files
	if err := os.WriteFile(syncedFile, []byte("synced"), 0644); err != nil {
		t.Fatalf("failed to create synced file: %v", err)
	}
	if err := os.WriteFile(partialFile, []byte("partial"), 0644); err != nil {
		t.Fatalf("failed to create partial file: %v", err)
	}

	// Mock chezmoi
	mockCm := &mockChezmoi{
		hasFileFunc: func(ctx context.Context, path string) (bool, error) {
			// Only synced file is managed
			return path == syncedFile, nil
		},
	}

	detector := NewDefaultStatusDetector(mockCm)
	configs := []ConfigFile{
		{Path: syncedFile},
		{Path: partialFile},
		{Path: missingFile},
	}

	ctx := context.Background()
	results, err := detector.DetectStatus(ctx, configs)
	if err != nil {
		t.Fatalf("DetectStatus() error = %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Verify each status
	if results[0].Status != StatusSynced {
		t.Errorf("expected synced status for %s, got %q", syncedFile, results[0].Status)
	}
	if results[1].Status != StatusPartial {
		t.Errorf("expected partial status for %s, got %q", partialFile, results[1].Status)
	}
	if results[2].Status != StatusMissing {
		t.Errorf("expected missing status for %s, got %q", missingFile, results[2].Status)
	}
}

package transaction

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireLock(t *testing.T) {
	t.Run("creates lock file", func(t *testing.T) {
		dir := t.TempDir()
		ctx := context.Background()

		lock, err := AcquireLock(ctx, dir)
		if err != nil {
			t.Fatalf("AcquireLock failed: %v", err)
		}
		defer lock.Release()

		// Verify lock file exists with correct name (config.lock, not config-add.lock)
		lockPath := filepath.Join(dir, "config.lock")
		if _, err := os.Stat(lockPath); os.IsNotExist(err) {
			t.Error("lock file not created with correct name")
		}

		// Verify old name doesn't exist
		oldLockPath := filepath.Join(dir, "config-add.lock")
		if _, err := os.Stat(oldLockPath); !os.IsNotExist(err) {
			t.Error("lock file should not use old name config-add.lock")
		}
	})

	t.Run("prevents concurrent locks", func(t *testing.T) {
		dir := t.TempDir()
		ctx := context.Background()

		lock1, err := AcquireLock(ctx, dir)
		if err != nil {
			t.Fatalf("first AcquireLock failed: %v", err)
		}
		defer lock1.Release()

		_, err = AcquireLock(ctx, dir)
		if err == nil {
			t.Error("expected error for concurrent lock")
		}
		if err != ErrLockExists {
			t.Errorf("expected ErrLockExists, got %v", err)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		dir := t.TempDir()
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := AcquireLock(ctx, dir)
		if err == nil {
			t.Error("expected error for cancelled context")
		}
	})

	t.Run("respects context timeout", func(t *testing.T) {
		dir := t.TempDir()
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		// Give time for context to timeout
		time.Sleep(5 * time.Millisecond)

		_, err := AcquireLock(ctx, dir)
		if err == nil {
			t.Error("expected error for timed out context")
		}
	})

	t.Run("creates directory if needed", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "nested", "txn")
		ctx := context.Background()

		lock, err := AcquireLock(ctx, dir)
		if err != nil {
			t.Fatalf("AcquireLock failed: %v", err)
		}
		defer lock.Release()

		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Error("directory not created")
		}
	})

	t.Run("writes lock metadata", func(t *testing.T) {
		dir := t.TempDir()
		ctx := context.Background()

		lock, err := AcquireLock(ctx, dir)
		if err != nil {
			t.Fatalf("AcquireLock failed: %v", err)
		}
		defer lock.Release()

		lockPath := filepath.Join(dir, "config.lock")
		data, err := os.ReadFile(lockPath)
		if err != nil {
			t.Fatalf("failed to read lock file: %v", err)
		}

		content := string(data)
		if len(content) == 0 {
			t.Error("lock file should contain metadata")
		}
	})
}

func TestLockRelease(t *testing.T) {
	t.Run("removes lock file", func(t *testing.T) {
		dir := t.TempDir()
		ctx := context.Background()

		lock, err := AcquireLock(ctx, dir)
		if err != nil {
			t.Fatalf("AcquireLock failed: %v", err)
		}

		lockPath := filepath.Join(dir, "config.lock")
		if _, err := os.Stat(lockPath); os.IsNotExist(err) {
			t.Error("lock file should exist before release")
		}

		if err := lock.Release(); err != nil {
			t.Fatalf("Release failed: %v", err)
		}

		if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
			t.Error("lock file should be removed after release")
		}
	})

	t.Run("allows new lock after release", func(t *testing.T) {
		dir := t.TempDir()
		ctx := context.Background()

		lock1, err := AcquireLock(ctx, dir)
		if err != nil {
			t.Fatalf("first AcquireLock failed: %v", err)
		}
		lock1.Release()

		lock2, err := AcquireLock(ctx, dir)
		if err != nil {
			t.Fatalf("second AcquireLock should succeed: %v", err)
		}
		defer lock2.Release()
	})

	t.Run("is idempotent", func(t *testing.T) {
		dir := t.TempDir()
		ctx := context.Background()

		lock, err := AcquireLock(ctx, dir)
		if err != nil {
			t.Fatalf("AcquireLock failed: %v", err)
		}

		// Release twice should not error
		if err := lock.Release(); err != nil {
			t.Fatalf("first Release failed: %v", err)
		}
		if err := lock.Release(); err != nil {
			t.Fatalf("second Release should not error: %v", err)
		}
	})
}

func TestStaleLockHandling(t *testing.T) {
	t.Run("removes stale lock and acquires new one", func(t *testing.T) {
		dir := t.TempDir()
		ctx := context.Background()

		// Create a stale lock file manually
		lockPath := filepath.Join(dir, "config.lock")
		if err := os.WriteFile(lockPath, []byte("pid=99999\ntimestamp=2020-01-01T00:00:00Z\n"), 0600); err != nil {
			t.Fatalf("failed to create stale lock: %v", err)
		}

		// Set modification time to past (beyond stale threshold)
		staleTime := time.Now().Add(-StaleLockThreshold - time.Minute)
		if err := os.Chtimes(lockPath, staleTime, staleTime); err != nil {
			t.Fatalf("failed to set stale time: %v", err)
		}

		// Should succeed by removing stale lock
		lock, err := AcquireLock(ctx, dir)
		if err != nil {
			t.Fatalf("AcquireLock should succeed with stale lock: %v", err)
		}
		defer lock.Release()
	})

	t.Run("fails for non-stale lock", func(t *testing.T) {
		dir := t.TempDir()
		ctx := context.Background()

		// Create a fresh lock file manually
		lockPath := filepath.Join(dir, "config.lock")
		if err := os.WriteFile(lockPath, []byte("pid=99999\ntimestamp=2020-01-01T00:00:00Z\n"), 0600); err != nil {
			t.Fatalf("failed to create lock: %v", err)
		}

		// Modification time is now (fresh), should fail
		_, err := AcquireLock(ctx, dir)
		if err == nil {
			t.Error("expected error for non-stale lock")
		}
	})
}

func TestLockFileName(t *testing.T) {
	// This test explicitly verifies HR-1: lock file should be config.lock
	t.Run("uses config.lock not config-add.lock", func(t *testing.T) {
		dir := t.TempDir()
		ctx := context.Background()

		lock, err := AcquireLock(ctx, dir)
		if err != nil {
			t.Fatalf("AcquireLock failed: %v", err)
		}
		defer lock.Release()

		// The lock path should use config.lock
		expectedPath := filepath.Join(dir, "config.lock")
		if lock.path != expectedPath {
			t.Errorf("expected lock path %s, got %s", expectedPath, lock.path)
		}
	})
}

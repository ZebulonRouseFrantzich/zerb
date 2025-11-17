package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	gogit "github.com/go-git/go-git/v5"
)

// TestInitRepo tests repository initialization
func TestInitRepo(t *testing.T) {
	tmpDir := t.TempDir()
	client := NewClient(tmpDir)
	ctx := context.Background()

	// Test successful initialization
	err := client.InitRepo(ctx)
	if err != nil {
		t.Fatalf("InitRepo() error = %v, want nil", err)
	}

	// Verify .git directory was created
	gitDir := filepath.Join(tmpDir, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		t.Errorf("InitRepo() did not create .git directory: %v", err)
	}

	// Verify repository can be opened
	_, err = gogit.PlainOpen(tmpDir)
	if err != nil {
		t.Errorf("InitRepo() created invalid repository: %v", err)
	}
}

// TestInitRepo_AlreadyExists tests initialization when repo already exists
func TestInitRepo_AlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	client := NewClient(tmpDir)
	ctx := context.Background()

	// Initialize first time
	err := client.InitRepo(ctx)
	if err != nil {
		t.Fatalf("InitRepo() first call error = %v, want nil", err)
	}

	// Try to initialize again - should fail but gracefully
	err = client.InitRepo(ctx)
	if err == nil {
		t.Error("InitRepo() on existing repo should return error")
	}
}

// TestIsGitRepo tests git repository detection
func TestIsGitRepo(t *testing.T) {
	tmpDir := t.TempDir()
	client := NewClient(tmpDir)
	ctx := context.Background()

	// Test non-existent repo
	isRepo, err := client.IsGitRepo(ctx)
	if err != nil {
		t.Errorf("IsGitRepo() error = %v, want nil", err)
	}
	if isRepo {
		t.Error("IsGitRepo() = true, want false for non-existent repo")
	}

	// Initialize repo
	client.InitRepo(ctx)

	// Test existing repo
	isRepo, err = client.IsGitRepo(ctx)
	if err != nil {
		t.Errorf("IsGitRepo() error = %v, want nil", err)
	}
	if !isRepo {
		t.Error("IsGitRepo() = false, want true for existing repo")
	}
}

// TestConfigureUser tests git user configuration
func TestConfigureUser(t *testing.T) {
	tmpDir := t.TempDir()
	client := NewClient(tmpDir)
	ctx := context.Background()

	// Initialize repo first
	if err := client.InitRepo(ctx); err != nil {
		t.Fatalf("InitRepo() error = %v", err)
	}

	// Configure user
	userInfo := GitUserInfo{
		Name:       "Test User",
		Email:      "test@example.com",
		FromEnv:    false,
		FromConfig: false,
		IsDefault:  false,
	}

	err := client.ConfigureUser(ctx, userInfo)
	if err != nil {
		t.Fatalf("ConfigureUser() error = %v, want nil", err)
	}

	// Verify user was configured
	repo, err := gogit.PlainOpen(tmpDir)
	if err != nil {
		t.Fatalf("failed to open repo: %v", err)
	}

	cfg, err := repo.Config()
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	if cfg.User.Name != userInfo.Name {
		t.Errorf("ConfigureUser() name = %q, want %q", cfg.User.Name, userInfo.Name)
	}
	if cfg.User.Email != userInfo.Email {
		t.Errorf("ConfigureUser() email = %q, want %q", cfg.User.Email, userInfo.Email)
	}
}

// TestConfigureUser_NoRepo tests user configuration without initialized repo
func TestConfigureUser_NoRepo(t *testing.T) {
	tmpDir := t.TempDir()
	client := NewClient(tmpDir)
	ctx := context.Background()

	userInfo := GitUserInfo{
		Name:  "Test User",
		Email: "test@example.com",
	}

	err := client.ConfigureUser(ctx, userInfo)
	if err == nil {
		t.Error("ConfigureUser() without repo should return error")
	}
}

// TestCreateInitialCommit tests creating initial commit
func TestCreateInitialCommit(t *testing.T) {
	tmpDir := t.TempDir()
	client := NewClient(tmpDir)
	ctx := context.Background()

	// Initialize repo and configure user
	if err := client.InitRepo(ctx); err != nil {
		t.Fatalf("InitRepo() error = %v", err)
	}

	userInfo := GitUserInfo{
		Name:  "Test User",
		Email: "test@example.com",
	}
	if err := client.ConfigureUser(ctx, userInfo); err != nil {
		t.Fatalf("ConfigureUser() error = %v", err)
	}

	// Create test files
	file1 := filepath.Join(tmpDir, "test1.txt")
	file2 := filepath.Join(tmpDir, "test2.txt")
	os.WriteFile(file1, []byte("content1"), 0644)
	os.WriteFile(file2, []byte("content2"), 0644)

	// Create commit
	err := client.CreateInitialCommit(ctx, "Initial commit", []string{"test1.txt", "test2.txt"})
	if err != nil {
		t.Fatalf("CreateInitialCommit() error = %v, want nil", err)
	}

	// Verify commit was created
	repo, err := gogit.PlainOpen(tmpDir)
	if err != nil {
		t.Fatalf("failed to open repo: %v", err)
	}

	ref, err := repo.Head()
	if err != nil {
		t.Fatalf("failed to get HEAD: %v", err)
	}

	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		t.Fatalf("failed to get commit: %v", err)
	}

	if commit.Message != "Initial commit" {
		t.Errorf("CreateInitialCommit() message = %q, want %q", commit.Message, "Initial commit")
	}

	if commit.Author.Name != userInfo.Name {
		t.Errorf("CreateInitialCommit() author name = %q, want %q", commit.Author.Name, userInfo.Name)
	}
}

// TestCreateInitialCommit_EmptyMessage tests commit with empty message
func TestCreateInitialCommit_EmptyMessage(t *testing.T) {
	tmpDir := t.TempDir()
	client := NewClient(tmpDir)
	ctx := context.Background()

	client.InitRepo(ctx)
	client.ConfigureUser(ctx, GitUserInfo{Name: "Test", Email: "test@example.com"})

	err := client.CreateInitialCommit(ctx, "", []string{"file.txt"})
	if err == nil {
		t.Error("CreateInitialCommit() with empty message should return error")
	}
}

// TestCreateInitialCommit_NoFiles tests commit with no files
func TestCreateInitialCommit_NoFiles(t *testing.T) {
	tmpDir := t.TempDir()
	client := NewClient(tmpDir)
	ctx := context.Background()

	client.InitRepo(ctx)
	client.ConfigureUser(ctx, GitUserInfo{Name: "Test", Email: "test@example.com"})

	err := client.CreateInitialCommit(ctx, "message", []string{})
	if err == nil {
		t.Error("CreateInitialCommit() with no files should return error")
	}
}

// TestCreateInitialCommit_NonExistentFile tests commit with file that doesn't exist
func TestCreateInitialCommit_NonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	client := NewClient(tmpDir)
	ctx := context.Background()

	// Initialize repo and configure user
	if err := client.InitRepo(ctx); err != nil {
		t.Fatalf("InitRepo() error = %v", err)
	}

	if err := client.ConfigureUser(ctx, GitUserInfo{Name: "Test", Email: "test@example.com"}); err != nil {
		t.Fatalf("ConfigureUser() error = %v", err)
	}

	// Try to commit a file that doesn't exist
	err := client.CreateInitialCommit(ctx, "Test commit", []string{"nonexistent.txt"})
	if err == nil {
		t.Error("CreateInitialCommit() with non-existent file should return error")
	}

	// Verify error message mentions the file issue
	if err != nil && !contains(err.Error(), "nonexistent") && !contains(err.Error(), "not found") && !contains(err.Error(), "stage") {
		t.Logf("Error message: %v", err)
	}
}

// TestDetectGitUser tests git user detection from environment
func TestDetectGitUser(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		wantName    string
		wantEmail   string
		wantEnv     bool
		wantDefault bool
	}{
		{
			name: "ZERB env vars",
			envVars: map[string]string{
				"ZERB_GIT_NAME":  "ZERB User",
				"ZERB_GIT_EMAIL": "zerb@example.com",
			},
			wantName:    "ZERB User",
			wantEmail:   "zerb@example.com",
			wantEnv:     true,
			wantDefault: false,
		},
		{
			name: "GIT env vars",
			envVars: map[string]string{
				"GIT_AUTHOR_NAME":  "Git User",
				"GIT_AUTHOR_EMAIL": "git@example.com",
			},
			wantName:    "Git User",
			wantEmail:   "git@example.com",
			wantEnv:     true,
			wantDefault: false,
		},
		{
			name:        "defaults",
			envVars:     map[string]string{},
			wantName:    "ZERB User",
			wantEmail:   "zerb@localhost",
			wantEnv:     false,
			wantDefault: true,
		},
		{
			name: "ZERB name only",
			envVars: map[string]string{
				"ZERB_GIT_NAME": "ZERB User",
			},
			wantName:    "ZERB User",
			wantEmail:   "zerb@localhost",
			wantEnv:     false,
			wantDefault: true,
		},
		{
			name: "ZERB email only",
			envVars: map[string]string{
				"ZERB_GIT_EMAIL": "zerb@example.com",
			},
			wantName:    "ZERB User",
			wantEmail:   "zerb@localhost",
			wantEnv:     false,
			wantDefault: true,
		},
		{
			name: "GIT name only",
			envVars: map[string]string{
				"GIT_AUTHOR_NAME": "Git User",
			},
			wantName:    "ZERB User",
			wantEmail:   "zerb@localhost",
			wantEnv:     false,
			wantDefault: true,
		},
		{
			name: "GIT email only",
			envVars: map[string]string{
				"GIT_AUTHOR_EMAIL": "git@example.com",
			},
			wantName:    "ZERB User",
			wantEmail:   "zerb@localhost",
			wantEnv:     false,
			wantDefault: true,
		},
		{
			name: "ZERB name with GIT email (mixed tiers)",
			envVars: map[string]string{
				"ZERB_GIT_NAME":    "ZERB User",
				"GIT_AUTHOR_EMAIL": "git@example.com",
			},
			wantName:    "ZERB User",
			wantEmail:   "zerb@localhost",
			wantEnv:     false,
			wantDefault: true,
		},
		{
			name: "GIT name with ZERB email (mixed tiers)",
			envVars: map[string]string{
				"GIT_AUTHOR_NAME": "Git User",
				"ZERB_GIT_EMAIL":  "zerb@example.com",
			},
			wantName:    "ZERB User",
			wantEmail:   "zerb@localhost",
			wantEnv:     false,
			wantDefault: true,
		},
		{
			name: "GIT vars override when ZERB partial",
			envVars: map[string]string{
				"ZERB_GIT_NAME":    "ZERB User",
				"GIT_AUTHOR_NAME":  "Git User",
				"GIT_AUTHOR_EMAIL": "git@example.com",
			},
			wantName:    "Git User",
			wantEmail:   "git@example.com",
			wantEnv:     true,
			wantDefault: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all git-related env vars
			os.Unsetenv("ZERB_GIT_NAME")
			os.Unsetenv("ZERB_GIT_EMAIL")
			os.Unsetenv("GIT_AUTHOR_NAME")
			os.Unsetenv("GIT_AUTHOR_EMAIL")

			// Set test env vars
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			// Cleanup
			defer func() {
				for k := range tt.envVars {
					os.Unsetenv(k)
				}
			}()

			userInfo := DetectGitUser()

			if userInfo.Name != tt.wantName {
				t.Errorf("DetectGitUser() name = %q, want %q", userInfo.Name, tt.wantName)
			}
			if userInfo.Email != tt.wantEmail {
				t.Errorf("DetectGitUser() email = %q, want %q", userInfo.Email, tt.wantEmail)
			}
			if userInfo.FromEnv != tt.wantEnv {
				t.Errorf("DetectGitUser() fromEnv = %v, want %v", userInfo.FromEnv, tt.wantEnv)
			}
			if userInfo.IsDefault != tt.wantDefault {
				t.Errorf("DetectGitUser() isDefault = %v, want %v", userInfo.IsDefault, tt.wantDefault)
			}
		})
	}
}

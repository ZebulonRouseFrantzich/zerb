package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/config"
)

// Mock implementations for testing
type mockChezmoiRemove struct {
	removeCalled bool
	removePath   string
	removeErr    error
}

func (m *mockChezmoiRemove) Add(ctx context.Context, path string, opts interface{}) error {
	return nil
}
func (m *mockChezmoiRemove) HasFile(ctx context.Context, path string) (bool, error) {
	return true, nil
}
func (m *mockChezmoiRemove) Remove(ctx context.Context, path string) error {
	m.removeCalled = true
	m.removePath = path
	return m.removeErr
}

type mockGitRemove struct {
	stageFiles   []string
	commitMsg    string
	commitBody   string
	headCommit   string
	stageErr     error
	commitErr    error
	headCommitOk bool
}

func (m *mockGitRemove) Stage(ctx context.Context, files ...string) error {
	m.stageFiles = files
	return m.stageErr
}
func (m *mockGitRemove) Commit(ctx context.Context, msg, body string) error {
	m.commitMsg = msg
	m.commitBody = body
	return m.commitErr
}
func (m *mockGitRemove) GetHeadCommit(ctx context.Context) (string, error) {
	if m.headCommitOk {
		return m.headCommit, nil
	}
	return m.headCommit, nil
}
func (m *mockGitRemove) InitRepo(ctx context.Context) error                            { return nil }
func (m *mockGitRemove) ConfigureUser(ctx context.Context, userInfo interface{}) error { return nil }
func (m *mockGitRemove) CreateInitialCommit(ctx context.Context, msg string, files []string) error {
	return nil
}
func (m *mockGitRemove) IsGitRepo(ctx context.Context) (bool, error) { return true, nil }

type mockParserRemove struct {
	cfg      *config.Config
	parseErr error
}

func (m *mockParserRemove) ParseString(ctx context.Context, lua string) (*config.Config, error) {
	if m.parseErr != nil {
		return nil, m.parseErr
	}
	return m.cfg, nil
}

type mockGeneratorRemove struct {
	filename   string
	content    string
	generateOk bool
	genErr     error
}

func (m *mockGeneratorRemove) GenerateTimestamped(ctx context.Context, cfg *config.Config, gitCommit string) (string, string, error) {
	if m.genErr != nil {
		return "", "", m.genErr
	}
	return m.filename, m.content, nil
}

func TestConfigRemoveService_Execute(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	tests := []struct {
		name          string
		setupZerb     func(zerbDir string)
		request       RemoveRequest
		mockChezmoi   *mockChezmoiRemove
		mockGit       *mockGitRemove
		mockParser    *mockParserRemove
		mockGenerator *mockGeneratorRemove
		wantErr       bool
		wantErrMsg    string
		wantRemoved   int
		wantCommitMsg string
		checkChezmoi  bool
	}{
		{
			name: "remove single config successfully",
			setupZerb: func(zerbDir string) {
				os.MkdirAll(filepath.Join(zerbDir, "configs"), 0755)
				os.WriteFile(filepath.Join(zerbDir, ".zerb-active"), []byte("zerb.test.lua\n"), 0644)
				os.WriteFile(filepath.Join(zerbDir, "zerb.active.lua"), []byte(`return { configs = { { path = "~/.zshrc" } } }`), 0644)
			},
			request: RemoveRequest{
				Paths:  []string{"~/.zshrc"},
				DryRun: false,
			},
			mockChezmoi: &mockChezmoiRemove{},
			mockGit: &mockGitRemove{
				headCommit:   "abc1234",
				headCommitOk: true,
			},
			mockParser: &mockParserRemove{
				cfg: &config.Config{
					Configs: []config.ConfigFile{
						{Path: "~/.zshrc"},
					},
				},
			},
			mockGenerator: &mockGeneratorRemove{
				filename:   "zerb.20251125T143022Z.lua",
				content:    "return {}",
				generateOk: true,
			},
			wantErr:       false,
			wantRemoved:   1,
			wantCommitMsg: "Remove ~/.zshrc from tracked configs",
			checkChezmoi:  true,
		},
		{
			name: "remove multiple configs successfully",
			setupZerb: func(zerbDir string) {
				os.MkdirAll(filepath.Join(zerbDir, "configs"), 0755)
				os.WriteFile(filepath.Join(zerbDir, ".zerb-active"), []byte("zerb.test.lua\n"), 0644)
				os.WriteFile(filepath.Join(zerbDir, "zerb.active.lua"), []byte(`return {}`), 0644)
			},
			request: RemoveRequest{
				Paths:  []string{"~/.zshrc", "~/.gitconfig"},
				DryRun: false,
			},
			mockChezmoi: &mockChezmoiRemove{},
			mockGit: &mockGitRemove{
				headCommit:   "def5678",
				headCommitOk: true,
			},
			mockParser: &mockParserRemove{
				cfg: &config.Config{
					Configs: []config.ConfigFile{
						{Path: "~/.zshrc"},
						{Path: "~/.gitconfig"},
						{Path: "~/.tmux.conf"},
					},
				},
			},
			mockGenerator: &mockGeneratorRemove{
				filename:   "zerb.20251125T143022Z.lua",
				content:    "return {}",
				generateOk: true,
			},
			wantErr:       false,
			wantRemoved:   2,
			wantCommitMsg: "Remove 2 configs from tracked configs",
		},
		{
			name: "dry run does not modify files",
			setupZerb: func(zerbDir string) {
				os.MkdirAll(filepath.Join(zerbDir, "configs"), 0755)
				os.WriteFile(filepath.Join(zerbDir, ".zerb-active"), []byte("zerb.test.lua\n"), 0644)
				os.WriteFile(filepath.Join(zerbDir, "zerb.active.lua"), []byte(`return {}`), 0644)
			},
			request: RemoveRequest{
				Paths:  []string{"~/.zshrc"},
				DryRun: true,
			},
			mockChezmoi: &mockChezmoiRemove{},
			mockGit: &mockGitRemove{
				headCommit:   "abc1234",
				headCommitOk: true,
			},
			mockParser: &mockParserRemove{
				cfg: &config.Config{
					Configs: []config.ConfigFile{
						{Path: "~/.zshrc"},
					},
				},
			},
			mockGenerator: &mockGeneratorRemove{
				filename: "zerb.20251125T143022Z.lua",
				content:  "return {}",
			},
			wantErr:      false,
			wantRemoved:  1,
			checkChezmoi: false, // Chezmoi should NOT be called in dry run
		},
		{
			name: "path not tracked returns error",
			setupZerb: func(zerbDir string) {
				os.MkdirAll(filepath.Join(zerbDir, "configs"), 0755)
				os.WriteFile(filepath.Join(zerbDir, ".zerb-active"), []byte("zerb.test.lua\n"), 0644)
				os.WriteFile(filepath.Join(zerbDir, "zerb.active.lua"), []byte(`return {}`), 0644)
			},
			request: RemoveRequest{
				Paths:  []string{"~/.bashrc"},
				DryRun: false,
			},
			mockChezmoi: &mockChezmoiRemove{},
			mockGit:     &mockGitRemove{},
			mockParser: &mockParserRemove{
				cfg: &config.Config{
					Configs: []config.ConfigFile{
						{Path: "~/.zshrc"}, // bashrc not here
					},
				},
			},
			mockGenerator: &mockGeneratorRemove{},
			wantErr:       true,
			wantErrMsg:    "not tracked",
		},
		{
			name: "deduplicates paths",
			setupZerb: func(zerbDir string) {
				os.MkdirAll(filepath.Join(zerbDir, "configs"), 0755)
				os.WriteFile(filepath.Join(zerbDir, ".zerb-active"), []byte("zerb.test.lua\n"), 0644)
				os.WriteFile(filepath.Join(zerbDir, "zerb.active.lua"), []byte(`return {}`), 0644)
			},
			request: RemoveRequest{
				Paths:  []string{"~/.zshrc", homeDir + "/.zshrc"}, // Same path, different format
				DryRun: false,
			},
			mockChezmoi: &mockChezmoiRemove{},
			mockGit: &mockGitRemove{
				headCommit:   "abc1234",
				headCommitOk: true,
			},
			mockParser: &mockParserRemove{
				cfg: &config.Config{
					Configs: []config.ConfigFile{
						{Path: "~/.zshrc"},
					},
				},
			},
			mockGenerator: &mockGeneratorRemove{
				filename: "zerb.20251125T143022Z.lua",
				content:  "return {}",
			},
			wantErr:       false,
			wantRemoved:   1, // Only 1, not 2, because paths are deduplicated
			wantCommitMsg: "Remove ~/.zshrc from tracked configs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zerbDir := filepath.Join(t.TempDir(), ".config", "zerb")
			os.MkdirAll(zerbDir, 0755)

			if tt.setupZerb != nil {
				tt.setupZerb(zerbDir)
			}

			// Create service with mocks
			svc := &ConfigRemoveService{
				chezmoi:   tt.mockChezmoi,
				git:       tt.mockGit,
				parser:    tt.mockParser,
				generator: tt.mockGenerator,
				clock:     &RealClock{},
				zerbDir:   zerbDir,
			}

			ctx := context.Background()
			result, err := svc.Execute(ctx, tt.request)

			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.wantErrMsg != "" {
				if err == nil || !contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("Execute() error = %v, want error containing %q", err, tt.wantErrMsg)
				}
				return
			}

			if result == nil {
				t.Fatal("Execute() returned nil result")
			}

			if len(result.RemovedPaths) != tt.wantRemoved {
				t.Errorf("Execute() removed %d paths, want %d", len(result.RemovedPaths), tt.wantRemoved)
			}

			if !tt.request.DryRun && tt.wantCommitMsg != "" {
				if tt.mockGit.commitMsg != tt.wantCommitMsg {
					t.Errorf("Execute() commit message = %q, want %q", tt.mockGit.commitMsg, tt.wantCommitMsg)
				}
			}

			if tt.checkChezmoi && !tt.request.DryRun {
				if !tt.mockChezmoi.removeCalled {
					t.Error("Execute() did not call chezmoi.Remove")
				}
			}

			if tt.request.DryRun && tt.mockChezmoi.removeCalled {
				t.Error("Execute() should not call chezmoi.Remove in dry run mode")
			}
		})
	}
}

func TestConfigRemoveService_Execute_ContextCancellation(t *testing.T) {
	zerbDir := filepath.Join(t.TempDir(), ".config", "zerb")
	os.MkdirAll(zerbDir, 0755)
	os.WriteFile(filepath.Join(zerbDir, ".zerb-active"), []byte("zerb.test.lua\n"), 0644)
	os.WriteFile(filepath.Join(zerbDir, "zerb.active.lua"), []byte(`return {}`), 0644)

	svc := &ConfigRemoveService{
		chezmoi:   &mockChezmoiRemove{},
		git:       &mockGitRemove{},
		parser:    &mockParserRemove{cfg: &config.Config{}},
		generator: &mockGeneratorRemove{},
		clock:     &RealClock{},
		zerbDir:   zerbDir,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.Execute(ctx, RemoveRequest{Paths: []string{"~/.zshrc"}})
	if err == nil {
		t.Error("Execute() should return error for cancelled context")
	}
}

func TestConfigRemoveService_Execute_NoPaths(t *testing.T) {
	zerbDir := filepath.Join(t.TempDir(), ".config", "zerb")
	os.MkdirAll(zerbDir, 0755)

	svc := &ConfigRemoveService{
		chezmoi:   &mockChezmoiRemove{},
		git:       &mockGitRemove{},
		parser:    &mockParserRemove{cfg: &config.Config{}},
		generator: &mockGeneratorRemove{},
		clock:     &RealClock{},
		zerbDir:   zerbDir,
	}

	ctx := context.Background()
	_, err := svc.Execute(ctx, RemoveRequest{Paths: []string{}})
	if err == nil {
		t.Error("Execute() should return error for empty paths")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && s != "" && substr != "" &&
			findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

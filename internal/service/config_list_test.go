package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/config"
)

// mockParser implements ConfigParser for testing.
type mockListParser struct {
	parseFunc func(ctx context.Context, lua string) (*config.Config, error)
}

func (m *mockListParser) ParseString(ctx context.Context, lua string) (*config.Config, error) {
	if m.parseFunc != nil {
		return m.parseFunc(ctx, lua)
	}
	return &config.Config{}, nil
}

// mockStatusDetector implements config.StatusDetector for testing.
type mockStatusDetector struct {
	detectFunc func(ctx context.Context, configs []config.ConfigFile) ([]config.ConfigWithStatus, error)
}

func (m *mockStatusDetector) DetectStatus(ctx context.Context, configs []config.ConfigFile) ([]config.ConfigWithStatus, error) {
	if m.detectFunc != nil {
		return m.detectFunc(ctx, configs)
	}
	return []config.ConfigWithStatus{}, nil
}

func TestConfigListService_List(t *testing.T) {
	tmpDir := t.TempDir()

	// Create active marker
	activeMarker := filepath.Join(tmpDir, ".zerb-active")
	activeFilename := "zerb.20250116T143022Z.lua"
	if err := os.WriteFile(activeMarker, []byte(activeFilename), 0644); err != nil {
		t.Fatalf("failed to create active marker: %v", err)
	}

	// Create active config file
	configsDir := filepath.Join(tmpDir, "configs")
	if err := os.MkdirAll(configsDir, 0755); err != nil {
		t.Fatalf("failed to create configs dir: %v", err)
	}
	activeConfigPath := filepath.Join(configsDir, activeFilename)
	configContent := `return { configs = {} }`
	if err := os.WriteFile(activeConfigPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to create active config: %v", err)
	}

	// Create mock parser
	mockParser := &mockListParser{
		parseFunc: func(ctx context.Context, lua string) (*config.Config, error) {
			return &config.Config{
				Configs: []config.ConfigFile{
					{Path: "~/.zshrc"},
					{Path: "~/.gitconfig", Template: true},
				},
			}, nil
		},
	}

	// Create mock status detector
	mockDetector := &mockStatusDetector{
		detectFunc: func(ctx context.Context, configs []config.ConfigFile) ([]config.ConfigWithStatus, error) {
			results := make([]config.ConfigWithStatus, len(configs))
			for i, cfg := range configs {
				results[i] = config.ConfigWithStatus{
					ConfigFile: cfg,
					Status:     config.StatusSynced,
				}
			}
			return results, nil
		},
	}

	service := NewConfigListService(mockParser, mockDetector, tmpDir)

	ctx := context.Background()
	req := ListRequest{}
	result, err := service.List(ctx, req)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(result.Configs) != 2 {
		t.Errorf("expected 2 configs, got %d", len(result.Configs))
	}

	if result.ActiveVersion != activeFilename {
		t.Errorf("expected active version %q, got %q", activeFilename, result.ActiveVersion)
	}
}

func TestConfigListService_List_NotInitialized(t *testing.T) {
	tmpDir := t.TempDir()

	mockParser := &mockListParser{}
	mockDetector := &mockStatusDetector{}

	service := NewConfigListService(mockParser, mockDetector, tmpDir)

	ctx := context.Background()
	req := ListRequest{}
	_, err := service.List(ctx, req)
	if err == nil {
		t.Fatal("expected error for uninitialized ZERB, got nil")
	}

	if !errors.Is(err, ErrNotInitialized) {
		t.Errorf("expected ErrNotInitialized, got %v", err)
	}
}

func TestConfigListService_List_NoConfigs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create active marker
	activeMarker := filepath.Join(tmpDir, ".zerb-active")
	activeFilename := "zerb.20250116T143022Z.lua"
	if err := os.WriteFile(activeMarker, []byte(activeFilename), 0644); err != nil {
		t.Fatalf("failed to create active marker: %v", err)
	}

	// Create active config file with no configs
	configsDir := filepath.Join(tmpDir, "configs")
	if err := os.MkdirAll(configsDir, 0755); err != nil {
		t.Fatalf("failed to create configs dir: %v", err)
	}
	activeConfigPath := filepath.Join(configsDir, activeFilename)
	configContent := `return { configs = {} }`
	if err := os.WriteFile(activeConfigPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to create active config: %v", err)
	}

	// Mock parser returns empty configs
	mockParser := &mockListParser{
		parseFunc: func(ctx context.Context, lua string) (*config.Config, error) {
			return &config.Config{
				Configs: []config.ConfigFile{},
			}, nil
		},
	}

	mockDetector := &mockStatusDetector{}

	service := NewConfigListService(mockParser, mockDetector, tmpDir)

	ctx := context.Background()
	req := ListRequest{}
	result, err := service.List(ctx, req)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(result.Configs) != 0 {
		t.Errorf("expected 0 configs, got %d", len(result.Configs))
	}
}

func TestConfigListService_List_ParseError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create active marker
	activeMarker := filepath.Join(tmpDir, ".zerb-active")
	activeFilename := "zerb.20250116T143022Z.lua"
	if err := os.WriteFile(activeMarker, []byte(activeFilename), 0644); err != nil {
		t.Fatalf("failed to create active marker: %v", err)
	}

	// Create active config file
	configsDir := filepath.Join(tmpDir, "configs")
	if err := os.MkdirAll(configsDir, 0755); err != nil {
		t.Fatalf("failed to create configs dir: %v", err)
	}
	activeConfigPath := filepath.Join(configsDir, activeFilename)
	if err := os.WriteFile(activeConfigPath, []byte("invalid lua"), 0644); err != nil {
		t.Fatalf("failed to create active config: %v", err)
	}

	// Mock parser returns error
	parseError := errors.New("parse error")
	mockParser := &mockListParser{
		parseFunc: func(ctx context.Context, lua string) (*config.Config, error) {
			return nil, parseError
		},
	}

	mockDetector := &mockStatusDetector{}

	service := NewConfigListService(mockParser, mockDetector, tmpDir)

	ctx := context.Background()
	req := ListRequest{}
	_, err := service.List(ctx, req)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestConfigListService_List_DetectorError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create active marker
	activeMarker := filepath.Join(tmpDir, ".zerb-active")
	activeFilename := "zerb.20250116T143022Z.lua"
	if err := os.WriteFile(activeMarker, []byte(activeFilename), 0644); err != nil {
		t.Fatalf("failed to create active marker: %v", err)
	}

	// Create active config file
	configsDir := filepath.Join(tmpDir, "configs")
	if err := os.MkdirAll(configsDir, 0755); err != nil {
		t.Fatalf("failed to create configs dir: %v", err)
	}
	activeConfigPath := filepath.Join(configsDir, activeFilename)
	configContent := `return { configs = {} }`
	if err := os.WriteFile(activeConfigPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to create active config: %v", err)
	}

	mockParser := &mockListParser{
		parseFunc: func(ctx context.Context, lua string) (*config.Config, error) {
			return &config.Config{
				Configs: []config.ConfigFile{{Path: "~/.zshrc"}},
			}, nil
		},
	}

	// Mock detector returns error
	detectorError := errors.New("detector error")
	mockDetector := &mockStatusDetector{
		detectFunc: func(ctx context.Context, configs []config.ConfigFile) ([]config.ConfigWithStatus, error) {
			return nil, detectorError
		},
	}

	service := NewConfigListService(mockParser, mockDetector, tmpDir)

	ctx := context.Background()
	req := ListRequest{}
	_, err := service.List(ctx, req)
	if err == nil {
		t.Fatal("expected detector error, got nil")
	}
}

func TestConfigListService_List_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	mockParser := &mockListParser{}
	mockDetector := &mockStatusDetector{}

	service := NewConfigListService(mockParser, mockDetector, tmpDir)

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := ListRequest{}
	_, err := service.List(ctx, req)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestConfigListService_List_TildePathNormalization(t *testing.T) {
	// Set HOME for tilde expansion
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	tmpDir := t.TempDir()

	// Create active marker
	activeMarker := filepath.Join(tmpDir, ".zerb-active")
	activeFilename := "zerb.20250116T143022Z.lua"
	if err := os.WriteFile(activeMarker, []byte(activeFilename), 0644); err != nil {
		t.Fatalf("failed to create active marker: %v", err)
	}

	// Create active config file
	configsDir := filepath.Join(tmpDir, "configs")
	if err := os.MkdirAll(configsDir, 0755); err != nil {
		t.Fatalf("failed to create configs dir: %v", err)
	}
	activeConfigPath := filepath.Join(configsDir, activeFilename)
	configContent := `return { configs = {} }`
	if err := os.WriteFile(activeConfigPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to create active config: %v", err)
	}

	// Create mock parser that returns tilde paths
	mockParser := &mockListParser{
		parseFunc: func(ctx context.Context, lua string) (*config.Config, error) {
			return &config.Config{
				Configs: []config.ConfigFile{
					{Path: "~/.zshrc"},
					{Path: "~/.config/nvim/init.lua"},
				},
			}, nil
		},
	}

	// Create mock detector that verifies paths are normalized (expanded)
	var receivedPaths []string
	mockDetector := &mockStatusDetector{
		detectFunc: func(ctx context.Context, configs []config.ConfigFile) ([]config.ConfigWithStatus, error) {
			// Record paths received by detector
			receivedPaths = make([]string, len(configs))
			for i, cfg := range configs {
				receivedPaths[i] = cfg.Path
			}

			results := make([]config.ConfigWithStatus, len(configs))
			for i, cfg := range configs {
				results[i] = config.ConfigWithStatus{
					ConfigFile: cfg,
					Status:     config.StatusSynced,
				}
			}
			return results, nil
		},
	}

	service := NewConfigListService(mockParser, mockDetector, tmpDir)

	ctx := context.Background()
	req := ListRequest{}
	_, err := service.List(ctx, req)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	// Verify paths were normalized (no tilde)
	for _, path := range receivedPaths {
		if filepath.HasPrefix(path, "~") {
			t.Errorf("path %q was not normalized (still has tilde)", path)
		}
		if !filepath.IsAbs(path) {
			t.Errorf("path %q is not absolute after normalization", path)
		}
	}

	// Verify expected absolute paths
	expectedPaths := []string{
		filepath.Join(tmpHome, ".zshrc"),
		filepath.Join(tmpHome, ".config/nvim/init.lua"),
	}

	if len(receivedPaths) != len(expectedPaths) {
		t.Fatalf("expected %d paths, got %d", len(expectedPaths), len(receivedPaths))
	}

	for i, expected := range expectedPaths {
		if receivedPaths[i] != expected {
			t.Errorf("path[%d] = %q, want %q", i, receivedPaths[i], expected)
		}
	}
}

func TestConfigListService_List_EmptyActiveMarker(t *testing.T) {
	tmpDir := t.TempDir()

	// Create empty active marker
	activeMarker := filepath.Join(tmpDir, ".zerb-active")
	if err := os.WriteFile(activeMarker, []byte("   \n"), 0644); err != nil {
		t.Fatalf("failed to create active marker: %v", err)
	}

	mockParser := &mockListParser{}
	mockDetector := &mockStatusDetector{}

	service := NewConfigListService(mockParser, mockDetector, tmpDir)

	ctx := context.Background()
	req := ListRequest{}
	_, err := service.List(ctx, req)
	if err == nil {
		t.Fatal("expected error for empty active marker, got nil")
	}

	if !errors.Is(err, context.Canceled) && err.Error() != "active marker is empty - corrupted state" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestConfigListService_List_MissingActiveConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create active marker pointing to non-existent config
	activeMarker := filepath.Join(tmpDir, ".zerb-active")
	activeFilename := "zerb.99999999T999999Z.lua"
	if err := os.WriteFile(activeMarker, []byte(activeFilename), 0644); err != nil {
		t.Fatalf("failed to create active marker: %v", err)
	}

	// Create configs directory but not the config file
	configsDir := filepath.Join(tmpDir, "configs")
	if err := os.MkdirAll(configsDir, 0755); err != nil {
		t.Fatalf("failed to create configs dir: %v", err)
	}

	mockParser := &mockListParser{}
	mockDetector := &mockStatusDetector{}

	service := NewConfigListService(mockParser, mockDetector, tmpDir)

	ctx := context.Background()
	req := ListRequest{}
	_, err := service.List(ctx, req)
	if err == nil {
		t.Fatal("expected error for missing active config, got nil")
	}
}

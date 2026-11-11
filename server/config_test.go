package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigLoadAndSave(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	s := &Server{repoRoot: tempDir}

	cfg := s.loadConfig()
	if cfg.ViewMode != "line-by-line" || cfg.DiffMode != "all" || cfg.SidebarOpen == nil || !*cfg.SidebarOpen || cfg.SidebarWidth != 290 {
		t.Fatalf("Expected default config line-by-line/all with sidebar open and width 290, got %+v", cfg)
	}

	falseVal := false
	newCfg := AppConfig{
		ViewMode:     "side-by-side",
		DiffMode:     "staged",
		SidebarOpen:  &falseVal,
		SidebarWidth: 420,
	}
	if err := s.saveConfig(newCfg); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	configFile := filepath.Join(tempDir, ConfigFileName)
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		t.Fatalf("Config file was not created: %s", configFile)
	}

	loaded := s.loadConfig()
	if loaded.ViewMode != "side-by-side" || loaded.DiffMode != "staged" || loaded.SidebarOpen == nil || *loaded.SidebarOpen != false || loaded.SidebarWidth != 420 {
		t.Fatalf("Expected loaded config to match, got %+v", loaded)
	}
}

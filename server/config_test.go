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
	if cfg.ViewMode != "line-by-line" || cfg.DiffMode != "all" {
		t.Fatalf("Expected default config line-by-line/all, got %+v", cfg)
	}

	newCfg := AppConfig{
		ViewMode: "side-by-side",
		DiffMode: "staged",
	}
	if err := s.saveConfig(newCfg); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	configFile := filepath.Join(tempDir, ConfigFileName)
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		t.Fatalf("Config file was not created: %s", configFile)
	}

	loaded := s.loadConfig()
	if loaded.ViewMode != "side-by-side" || loaded.DiffMode != "staged" {
		t.Fatalf("Expected loaded config to match, got %+v", loaded)
	}
}

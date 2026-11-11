package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

const (
	ConfigAppDirName = "review"
	ConfigFileName   = "config.json"
)

type AppConfig struct {
	ViewMode     string `json:"viewMode"`
	DiffMode     string `json:"diffMode"`
	SidebarOpen  *bool  `json:"sidebarOpen,omitempty"`
	SidebarWidth int    `json:"sidebarWidth,omitempty"`
	ContextLines int    `json:"contextLines,omitempty"`
}

func DefaultConfig() AppConfig {
	defaultOpen := true
	return AppConfig{
		ViewMode:     "line-by-line",
		DiffMode:     "all",
		SidebarOpen:  &defaultOpen,
		SidebarWidth: 290,
		ContextLines: 3,
	}
}

func ConfigFilePath() (string, error) {
	baseDir := os.Getenv("REVIEW_CONFIG_DIR")
	if baseDir == "" {
		var err error
		baseDir, err = os.UserConfigDir()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(baseDir, ConfigAppDirName, ConfigFileName), nil
}

func (s *Server) loadConfig() AppConfig {
	cfg := DefaultConfig()
	cfgPath, err := ConfigFilePath()
	if err != nil {
		return cfg
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	if cfg.ViewMode != "side-by-side" && cfg.ViewMode != "line-by-line" {
		cfg.ViewMode = "line-by-line"
	}
	if cfg.DiffMode != "all" && cfg.DiffMode != "unstaged" && cfg.DiffMode != "staged" && cfg.DiffMode != "branch" {
		cfg.DiffMode = "all"
	}
	if cfg.SidebarOpen == nil {
		defaultOpen := true
		cfg.SidebarOpen = &defaultOpen
	}
	if cfg.SidebarWidth < 160 || cfg.SidebarWidth > 800 {
		cfg.SidebarWidth = 290
	}
	if cfg.ContextLines <= 0 {
		cfg.ContextLines = 3
	}
	return cfg
}

func (s *Server) saveConfig(cfg AppConfig) error {
	if cfg.ViewMode != "side-by-side" && cfg.ViewMode != "line-by-line" {
		cfg.ViewMode = "line-by-line"
	}
	if cfg.DiffMode != "all" && cfg.DiffMode != "unstaged" && cfg.DiffMode != "staged" && cfg.DiffMode != "branch" {
		cfg.DiffMode = "all"
	}
	if cfg.SidebarOpen == nil {
		defaultOpen := true
		cfg.SidebarOpen = &defaultOpen
	}
	if cfg.SidebarWidth < 160 || cfg.SidebarWidth > 800 {
		cfg.SidebarWidth = 290
	}
	if cfg.ContextLines <= 0 {
		cfg.ContextLines = 3
	}
	cfgPath, err := ConfigFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(cfgPath, data, 0644)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.loadConfig())
	case http.MethodPost:
		var cfg AppConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid config JSON"})
			return
		}
		if err := s.saveConfig(cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, s.loadConfig())
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

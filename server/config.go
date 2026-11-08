package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

const ConfigFileName = ".local-review.json"

type AppConfig struct {
	ViewMode string `json:"viewMode"`
	DiffMode string `json:"diffMode"`
}

func DefaultConfig() AppConfig {
	return AppConfig{
		ViewMode: "line-by-line",
		DiffMode: "all",
	}
}

func (s *Server) loadConfig() AppConfig {
	cfg := DefaultConfig()
	data, err := os.ReadFile(filepath.Join(s.repoRoot, ConfigFileName))
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	if cfg.ViewMode != "side-by-side" && cfg.ViewMode != "line-by-line" {
		cfg.ViewMode = "line-by-line"
	}
	if cfg.DiffMode != "all" && cfg.DiffMode != "unstaged" && cfg.DiffMode != "staged" {
		cfg.DiffMode = "all"
	}
	return cfg
}

func (s *Server) saveConfig(cfg AppConfig) error {
	if cfg.ViewMode != "side-by-side" && cfg.ViewMode != "line-by-line" {
		cfg.ViewMode = "line-by-line"
	}
	if cfg.DiffMode != "all" && cfg.DiffMode != "unstaged" && cfg.DiffMode != "staged" {
		cfg.DiffMode = "all"
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(s.repoRoot, ConfigFileName), data, 0644)
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

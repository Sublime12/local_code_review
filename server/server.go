package server

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"

	"github.com/Sublime12/local_code_review/git"
)

type Server struct {
	repoRoot   string
	defaultRef string
	store      *CommentStore
	webFS      embed.FS
}

func NewServer(repoRoot string, webFS embed.FS, defaultRef ...string) *Server {
	ref := ""
	if len(defaultRef) > 0 {
		ref = defaultRef[0]
	}
	return &Server{
		repoRoot:   repoRoot,
		defaultRef: ref,
		store:      NewCommentStore(),
		webFS:      webFS,
	}
}

// Handler configures and returns the HTTP request multiplexer
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/diff", s.handleDiff)
	mux.HandleFunc("/api/context", s.handleContext)
	mux.HandleFunc("/api/comments", s.handleComments)
	mux.HandleFunc("/api/export", s.handleExport)
	mux.HandleFunc("/api/clear", s.handleClear)
	mux.HandleFunc("/api/config", s.handleConfig)

	subFS, err := fs.Sub(s.webFS, "web")
	if err != nil {
		subFS = s.webFS
	} else if _, statErr := fs.Stat(subFS, "index.html"); statErr != nil {
		subFS = s.webFS
	}
	fileServer := http.FileServer(http.FS(subFS))
	mux.Handle("/", fileServer)

	return mux
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	branch := git.GetCurrentBranch()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"repoRoot": s.repoRoot,
		"branch":   branch,
		"ref":      s.defaultRef,
	})
}

func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	ref := r.URL.Query().Get("ref")
	if ref == "" && !r.URL.Query().Has("ref") {
		ref = s.defaultRef
	}
	contextStr := r.URL.Query().Get("context")

	contextLines := 3
	if cVal, err := strconv.Atoi(contextStr); err == nil && cVal >= 0 {
		contextLines = cVal
	}

	diffText, err := git.GetDiff(mode, ref, contextLines)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	fileLines := git.GetDiffFileLines(s.repoRoot, diffText)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"diff":      diffText,
		"mode":      mode,
		"ref":       ref,
		"context":   contextLines,
		"fileLines": fileLines,
	})
}

func (s *Server) handleContext(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("file")
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	if filePath != "" && (startStr == "" || endStr == "") {
		total, err := git.GetFileLineCount(s.repoRoot, filePath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"file":       filePath,
			"totalLines": total,
		})
		return
	}

	if filePath != "" && startStr != "" && endStr != "" {
		startLine, err1 := strconv.Atoi(startStr)
		endLine, err2 := strconv.Atoi(endStr)
		if err1 != nil || err2 != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid start or end line number"})
			return
		}
		lines, total, err := git.GetLineRange(s.repoRoot, filePath, startLine, endLine)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"file":       filePath,
			"start":      startLine,
			"end":        endLine,
			"totalLines": total,
			"context":    lines,
		})
		return
	}

	lineStr := r.URL.Query().Get("line")
	radiusStr := r.URL.Query().Get("radius")

	if filePath == "" || lineStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file and line are required"})
		return
	}

	line, err := strconv.Atoi(lineStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid line number"})
		return
	}

	radius := 2
	if rVal, err := strconv.Atoi(radiusStr); err == nil && rVal > 0 {
		radius = rVal
	}

	lines, err := git.GetLineContext(s.repoRoot, filePath, line, radius)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"file":    filePath,
		"line":    line,
		"context": lines,
	})
}

func (s *Server) handleComments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		comments := s.store.List()
		writeJSON(w, http.StatusOK, comments)

	case http.MethodPost:
		var c Comment
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid comment body"})
			return
		}

		if c.FilePath == "" || c.Line <= 0 || c.Text == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "filePath, line, and text are required"})
			return
		}

		if len(c.Snippet) == 0 {
			lines, _ := git.GetLineContext(s.repoRoot, c.FilePath, c.Line, 2)
			c.Snippet = lines
		}

		s.store.AddOrUpdate(&c)
		writeJSON(w, http.StatusOK, c)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id parameter required"})
			return
		}
		s.store.Delete(id)
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SaveToFile bool   `json:"saveToFile"`
		Filename   string `json:"filename"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	prompt := s.store.GeneratePrompt()

	if req.SaveToFile {
		err := s.store.SaveToFile(s.repoRoot, req.Filename)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"prompt": prompt,
		"count":  len(s.store.List()),
		"saved":  req.SaveToFile,
	})
}

func (s *Server) handleClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.store.Clear()
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

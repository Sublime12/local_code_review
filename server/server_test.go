package server

import (
	"bytes"
	"embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodebaseAndCommentEndpoints(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "server-codebase-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	cmd := exec.Command("git", "-C", tempDir, "init")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, string(out))
	}

	if err := os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "logo.bin"), []byte{0x00, 0x01, 0x02}, 0644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(tempDir, embed.FS{})
	srv.SetDefaultMode("codebase")
	handler := srv.Handler()

	// /api/status should include default mode
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/status status = %d", rec.Code)
	}
	var statusResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &statusResp); err != nil {
		t.Fatal(err)
	}
	if statusResp["mode"] != "codebase" {
		t.Errorf("Expected status mode 'codebase', got %v", statusResp["mode"])
	}

	// /api/codebase/files should list both untracked non-ignored files
	req = httptest.NewRequest(http.MethodGet, "/api/codebase/files", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/codebase/files status = %d: %s", rec.Code, rec.Body.String())
	}
	var filesResp struct {
		Files       []string        `json:"files"`
		FileLines   map[string]int  `json:"fileLines"`
		BinaryFiles map[string]bool `json:"binaryFiles"`
		TotalLines  int             `json:"totalLines"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &filesResp); err != nil {
		t.Fatal(err)
	}
	if len(filesResp.Files) != 2 || filesResp.Files[0] != "logo.bin" || filesResp.Files[1] != "main.go" {
		t.Fatalf("Unexpected files list: %+v", filesResp.Files)
	}
	if filesResp.TotalLines != 3 || filesResp.FileLines["main.go"] != 3 || !filesResp.BinaryFiles["logo.bin"] {
		t.Fatalf("Unexpected codebase summary stats: totalLines=%d fileLines=%+v binaryFiles=%+v", filesResp.TotalLines, filesResp.FileLines, filesResp.BinaryFiles)
	}

	// /api/codebase/file for text file
	req = httptest.NewRequest(http.MethodGet, "/api/codebase/file?path=main.go", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/codebase/file status = %d: %s", rec.Code, rec.Body.String())
	}
	var fileResp struct {
		Path       string `json:"path"`
		Binary     bool   `json:"binary"`
		TotalLines int    `json:"totalLines"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &fileResp); err != nil {
		t.Fatal(err)
	}
	if fileResp.Binary || fileResp.TotalLines != 3 {
		t.Fatalf("Unexpected fileResp: %+v", fileResp)
	}

	// /api/codebase/file for binary file
	req = httptest.NewRequest(http.MethodGet, "/api/codebase/file?path=logo.bin", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET binary file status = %d", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &fileResp); err != nil {
		t.Fatal(err)
	}
	if !fileResp.Binary {
		t.Errorf("Expected binary=true for logo.bin")
	}

	// /api/codebase/file blocks traversal
	req = httptest.NewRequest(http.MethodGet, "/api/codebase/file?path=../secret.txt", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for path traversal, got %d", rec.Code)
	}

	// /api/comments accepts general, file-level, and inline comments
	postComment := func(payload string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/comments", bytes.NewBufferString(payload))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}

	if w := postComment(`{"text":"   "}`); w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for empty comment text, got %d", w.Code)
	}
	if w := postComment(`{"filePath":"","line":0,"text":"Adopt layered architecture"}`); w.Code != http.StatusOK {
		t.Errorf("Expected 200 for general comment, got %d: %s", w.Code, w.Body.String())
	}
	if w := postComment(`{"filePath":"main.go","line":0,"text":"Move entrypoint logic to internal/app"}`); w.Code != http.StatusOK {
		t.Errorf("Expected 200 for file comment, got %d: %s", w.Code, w.Body.String())
	}
	if w := postComment(`{"filePath":"main.go","line":3,"text":"Handle errors here"}`); w.Code != http.StatusOK {
		t.Errorf("Expected 200 for inline comment, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/export", bytes.NewBufferString(`{}`))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/export failed: %d", rec.Code)
	}
	var exportResp struct {
		Prompt string `json:"prompt"`
		Count  int    `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &exportResp); err != nil {
		t.Fatal(err)
	}
	if exportResp.Count != 3 {
		t.Errorf("Expected 3 comments exported, got %d", exportResp.Count)
	}
	for _, want := range []string{
		"review (general): Adopt layered architecture",
		"review: Move entrypoint logic to internal/app\nmain.go",
		"review: Handle errors here\nmain.go:3",
	} {
		if !strings.Contains(exportResp.Prompt, want) {
			t.Errorf("Prompt missing %q:\n%s", want, exportResp.Prompt)
		}
	}
}

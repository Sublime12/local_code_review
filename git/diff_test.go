package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetLineContext(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	content := `line 1
line 2
line 3
line 4
line 5
line 6
line 7
line 8
line 9
line 10`

	testFile := "test.txt"
	if err := os.WriteFile(filepath.Join(tempDir, testFile), []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Test middle line (line 5, radius 2 -> lines 3, 4, 5, 6, 7)
	lines, err := GetLineContext(tempDir, testFile, 5, 2)
	if err != nil {
		t.Fatalf("GetLineContext failed: %v", err)
	}

	if len(lines) != 5 {
		t.Fatalf("Expected 5 lines, got %d", len(lines))
	}
	if lines[0].LineNumber != 3 || lines[0].Content != "line 3" {
		t.Errorf("Expected first line to be line 3, got: %+v", lines[0])
	}
	if lines[2].LineNumber != 5 || !lines[2].IsTarget {
		t.Errorf("Expected target line to be line 5 with IsTarget=true, got: %+v", lines[2])
	}
	if lines[4].LineNumber != 7 || lines[4].Content != "line 7" {
		t.Errorf("Expected last line to be line 7, got: %+v", lines[4])
	}

	// Test boundary start (line 1, radius 2 -> lines 1, 2, 3)
	linesStart, err := GetLineContext(tempDir, testFile, 1, 2)
	if err != nil {
		t.Fatalf("GetLineContext failed: %v", err)
	}
	if len(linesStart) != 3 {
		t.Fatalf("Expected 3 lines, got %d", len(linesStart))
	}
	if linesStart[0].LineNumber != 1 || !linesStart[0].IsTarget {
		t.Errorf("Expected target line to be line 1, got: %+v", linesStart[0])
	}

	// Test boundary end (line 10, radius 2 -> lines 8, 9, 10)
	linesEnd, err := GetLineContext(tempDir, testFile, 10, 2)
	if err != nil {
		t.Fatalf("GetLineContext failed: %v", err)
	}
	if len(linesEnd) != 3 {
		t.Fatalf("Expected 3 lines, got %d", len(linesEnd))
	}
	if linesEnd[2].LineNumber != 10 || !linesEnd[2].IsTarget {
		t.Errorf("Expected target line to be line 10, got: %+v", linesEnd[2])
	}

	rangeLines, totalLines, err := GetLineRange(tempDir, testFile, 2, 6)
	if err != nil {
		t.Fatalf("GetLineRange failed: %v", err)
	}
	if totalLines != 10 {
		t.Errorf("Expected total lines 10, got %d", totalLines)
	}
	if len(rangeLines) != 5 || rangeLines[0].LineNumber != 2 || rangeLines[4].LineNumber != 6 {
		t.Errorf("Unexpected rangeLines: %+v", rangeLines)
	}

	count, err := GetFileLineCount(tempDir, testFile)
	if err != nil {
		t.Fatalf("GetFileLineCount failed: %v", err)
	}
	if count != 10 {
		t.Errorf("Expected file line count 10, got %d", count)
	}

	diffSample := "--- a/test.txt\n+++ b/test.txt\n@@ -1,3 +1,3 @@\n"
	diffCounts := GetDiffFileLines(tempDir, diffSample)
	if diffCounts["test.txt"] != 10 {
		t.Errorf("Expected diff file lines for test.txt to be 10, got %d", diffCounts["test.txt"])
	}
}

func TestCodebaseFileListingAndReading(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "git-codebase-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", tempDir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
		}
	}

	runGit("init")
	if err := os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte("*.ignored\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "pkg"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "pkg", "main.go"), []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "untracked.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "secret.ignored"), []byte("ignored"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "long.min.js"), []byte(strings.Repeat("a", 100*1024)), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "bin.dat"), []byte{0x89, 0x50, 0x4E, 0x47, 0x00, 0x01}, 0644); err != nil {
		t.Fatal(err)
	}

	runGit("add", ".gitignore", "pkg/main.go")

	files, err := ListRepoFiles(tempDir)
	if err != nil {
		t.Fatalf("ListRepoFiles failed: %v", err)
	}

	expected := []string{".gitignore", "bin.dat", "long.min.js", "pkg/main.go", "untracked.txt"}
	if len(files) != len(expected) {
		t.Fatalf("Expected %v, got %v", expected, files)
	}
	for i := range expected {
		if files[i] != expected[i] {
			t.Errorf("Expected files[%d] = %q, got %q", i, expected[i], files[i])
		}
	}

	lines, isBinary, err := GetFileContent(tempDir, "pkg/main.go")
	if err != nil {
		t.Fatalf("GetFileContent failed: %v", err)
	}
	if isBinary || len(lines) != 2 || lines[0].LineNumber != 1 || lines[0].Content != "package main" || lines[1].LineNumber != 2 {
		t.Errorf("Unexpected content: isBinary=%v lines=%+v", isBinary, lines)
	}

	longLines, isBinary, err := GetFileContent(tempDir, "long.min.js")
	if err != nil || isBinary || len(longLines) != 1 || len(longLines[0].Content) != 100*1024 {
		t.Errorf("Failed reading >64KB single line file: err=%v isBinary=%v len=%d", err, isBinary, len(longLines))
	}

	_, isBinary, err = GetFileContent(tempDir, "bin.dat")
	if err != nil || !isBinary {
		t.Errorf("Expected bin.dat to be detected as binary, got isBinary=%v err=%v", isBinary, err)
	}

	if _, _, err := GetFileContent(tempDir, "../outside.txt"); err == nil {
		t.Errorf("Expected path traversal error, got nil")
	}

	fileLines, binaryFiles, totalLines := SummarizeRepoFiles(tempDir, files)
	if !binaryFiles["bin.dat"] {
		t.Errorf("Expected bin.dat to be marked binary in SummarizeRepoFiles")
	}
	if fileLines["pkg/main.go"] != 2 || fileLines["untracked.txt"] != 1 || fileLines["long.min.js"] != 1 || fileLines[".gitignore"] != 1 {
		t.Errorf("Unexpected fileLines map: %+v", fileLines)
	}
	if totalLines != 5 {
		t.Errorf("Expected totalLines = 5, got %d", totalLines)
	}
}

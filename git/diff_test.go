package git

import (
	"os"
	"path/filepath"
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
}

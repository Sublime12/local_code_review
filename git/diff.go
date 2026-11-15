package git

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// CodeLine represents a single line of source code for context
type CodeLine struct {
	LineNumber int    `json:"lineNumber"`
	Content    string `json:"content"`
	IsTarget   bool   `json:"isTarget"`
}

// IsInsideWorkTree verifies whether the current directory is inside a Git repository
func IsInsideWorkTree() (bool, error) {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

// GetRepoRoot returns the top-level path of the Git repository
func GetRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// HasHeadCommit checks whether the repository has at least one commit
func HasHeadCommit() bool {
	cmd := exec.Command("git", "rev-parse", "--verify", "HEAD")
	return cmd.Run() == nil
}

// GetCurrentBranch returns the active git branch name
func GetCurrentBranch() string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "HEAD"
	}
	return strings.TrimSpace(string(out))
}

func GetDiff(mode string, ref string, contextLines ...int) (string, error) {
	ctx := 3
	if len(contextLines) > 0 && contextLines[0] >= 0 {
		ctx = contextLines[0]
	}
	var args []string
	hasHead := HasHeadCommit()
	diffBase := []string{"diff", "--diff-algorithm=histogram", fmt.Sprintf("-U%d", ctx)}

	switch mode {
	case "staged":
		args = append(diffBase, "--cached")
	case "unstaged":
		args = diffBase
	case "branch":
		if ref == "" {
			ref = "main"
		}
		args = append(diffBase, ref)
	case "all":
		fallthrough
	default:
		if ref != "" {
			args = append(diffBase, ref)
		} else if hasHead {
			args = append(diffBase, "HEAD")
		} else {
			args = diffBase
		}
	}

	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, string(out))
	}

	diffText := string(out)

	if mode == "all" || mode == "unstaged" || mode == "" {
		untrackedDiff, _ := getUntrackedDiff()
		if untrackedDiff != "" {
			if diffText != "" && !strings.HasSuffix(diffText, "\n") {
				diffText += "\n"
			}
			diffText += untrackedDiff
		}
	}

	return diffText, nil
}

func getUntrackedDiff() (string, error) {
	cmd := exec.Command("git", "status", "--porcelain", "-uall")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "?? ") {
			filePath := strings.TrimSpace(line[3:])
			diffCmd := exec.Command("git", "diff", "--diff-algorithm=histogram", "--no-index", "--", "/dev/null", filePath)
			diffOut, _ := diffCmd.CombinedOutput()
			sb.Write(diffOut)
		}
	}

	return sb.String(), nil
}

const maxScannerLineSize = 8 * 1024 * 1024

func resolveRepoFile(repoRoot, filePath string) (string, error) {
	trimmed := strings.TrimSpace(filePath)
	if trimmed == "" || filepath.IsAbs(trimmed) {
		return "", fmt.Errorf("invalid file path")
	}

	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", err
	}
	absTarget, err := filepath.Abs(filepath.Join(absRoot, filepath.FromSlash(trimmed)))
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes repository root")
	}

	evalRoot, rootErr := filepath.EvalSymlinks(absRoot)
	evalTarget, targetErr := filepath.EvalSymlinks(absTarget)
	if rootErr == nil && targetErr == nil {
		evalRel, relErr := filepath.Rel(evalRoot, evalTarget)
		if relErr != nil || evalRel == "." || evalRel == ".." || strings.HasPrefix(evalRel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("path escapes repository root")
		}
	}

	return absTarget, nil
}

func isBinaryFile(file *os.File) (bool, error) {
	buf := make([]byte, 8192)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return false, err
	}
	isBin := bytes.IndexByte(buf[:n], 0) >= 0
	if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
		return false, seekErr
	}
	return isBin, nil
}

func scanAllLines(r io.Reader) ([]string, error) {
	var lines []string
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxScannerLineSize)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func openValidatedRepoFile(repoRoot, filePath string) (*os.File, error) {
	fullPath, err := resolveRepoFile(repoRoot, filePath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory")
	}

	return os.Open(fullPath)
}

func readRepoFileLines(repoRoot, filePath string) ([]string, bool, error) {
	file, err := openValidatedRepoFile(repoRoot, filePath)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()

	binary, err := isBinaryFile(file)
	if err != nil {
		return nil, false, err
	}
	if binary {
		return nil, true, nil
	}

	lines, err := scanAllLines(file)
	if err != nil {
		return nil, false, err
	}
	return lines, false, nil
}

func CountFileLines(repoRoot, filePath string) (int, bool, error) {
	file, err := openValidatedRepoFile(repoRoot, filePath)
	if err != nil {
		return 0, false, err
	}
	defer file.Close()

	buf := make([]byte, 32*1024)
	lines := 0
	totalBytes := 0
	var lastByte byte
	checkedBinary := false

	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			if !checkedBinary {
				checkedBinary = true
				checkLen := n
				if checkLen > 8192 {
					checkLen = 8192
				}
				if bytes.IndexByte(chunk[:checkLen], 0) >= 0 {
					return 0, true, nil
				}
			}
			lines += bytes.Count(chunk, []byte{'\n'})
			totalBytes += n
			lastByte = chunk[n-1]
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return 0, false, readErr
		}
	}

	if totalBytes > 0 && lastByte != '\n' {
		lines++
	}
	return lines, false, nil
}

func SummarizeRepoFiles(repoRoot string, files []string) (map[string]int, map[string]bool, int) {
	fileLines := make(map[string]int, len(files))
	binaryFiles := make(map[string]bool)
	totalLines := 0

	for _, f := range files {
		count, isBinary, err := CountFileLines(repoRoot, f)
		if err != nil {
			continue
		}
		if isBinary {
			binaryFiles[f] = true
			continue
		}
		fileLines[f] = count
		totalLines += count
	}

	return fileLines, binaryFiles, totalLines
}

// GetLineContext reads lines around targetLine (1-indexed) in filePath with given radius
func GetLineContext(repoRoot, filePath string, targetLine int, radius int) ([]CodeLine, error) {
	allLines, binary, err := readRepoFileLines(repoRoot, filePath)
	if err != nil || binary {
		return nil, err
	}

	total := len(allLines)
	if total == 0 {
		return []CodeLine{}, nil
	}

	if targetLine < 1 {
		targetLine = 1
	}
	if targetLine > total {
		targetLine = total
	}

	start := targetLine - radius
	if start < 1 {
		start = 1
	}
	end := targetLine + radius
	if end > total {
		end = total
	}

	var result []CodeLine
	for l := start; l <= end; l++ {
		result = append(result, CodeLine{
			LineNumber: l,
			Content:    allLines[l-1],
			IsTarget:   l == targetLine,
		})
	}

	return result, nil
}

func GetLineRange(repoRoot, filePath string, startLine, endLine int) ([]CodeLine, int, error) {
	allLines, binary, err := readRepoFileLines(repoRoot, filePath)
	if err != nil || binary {
		return nil, 0, err
	}

	total := len(allLines)
	if total == 0 {
		return []CodeLine{}, 0, nil
	}

	if startLine < 1 {
		startLine = 1
	}
	if endLine > total {
		endLine = total
	}
	if startLine > endLine {
		return []CodeLine{}, total, nil
	}

	var result []CodeLine
	for l := startLine; l <= endLine; l++ {
		result = append(result, CodeLine{
			LineNumber: l,
			Content:    allLines[l-1],
		})
	}

	return result, total, nil
}

func GetFileLineCount(repoRoot, filePath string) (int, error) {
	allLines, binary, err := readRepoFileLines(repoRoot, filePath)
	if err != nil || binary {
		return 0, err
	}
	return len(allLines), nil
}

func ListRepoFiles(repoRoot string) ([]string, error) {
	cmd := exec.Command("git", "-C", repoRoot, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files failed: %w", err)
	}

	seen := make(map[string]struct{})
	var files []string
	for _, entry := range bytes.Split(out, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		relPath := filepath.ToSlash(string(entry))
		if _, exists := seen[relPath]; exists {
			continue
		}
		fullPath, err := resolveRepoFile(repoRoot, relPath)
		if err != nil {
			continue
		}
		info, err := os.Stat(fullPath)
		if err != nil || info.IsDir() {
			continue
		}
		seen[relPath] = struct{}{}
		files = append(files, relPath)
	}

	sort.Strings(files)
	return files, nil
}

func GetFileContent(repoRoot, filePath string) ([]CodeLine, bool, error) {
	allLines, binary, err := readRepoFileLines(repoRoot, filePath)
	if err != nil {
		return nil, false, err
	}
	if binary {
		return nil, true, nil
	}

	lines := make([]CodeLine, len(allLines))
	for i, content := range allLines {
		lines[i] = CodeLine{
			LineNumber: i + 1,
			Content:    content,
		}
	}
	return lines, false, nil
}

func GetDiffFileLines(repoRoot, diffText string) map[string]int {
	result := make(map[string]int)
	scanner := bufio.NewScanner(strings.NewReader(diffText))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "+++ b/") {
			p := strings.TrimPrefix(line, "+++ b/")
			if p != "/dev/null" && p != "dev/null" && p != "" {
				if count, err := GetFileLineCount(repoRoot, p); err == nil {
					result[p] = count
				}
			}
		}
	}
	return result
}

package git

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// GetLineContext reads lines around targetLine (1-indexed) in filePath with given radius
func GetLineContext(repoRoot, filePath string, targetLine int, radius int) ([]CodeLine, error) {
	fullPath := filepath.Join(repoRoot, filePath)
	file, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var allLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
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

func GetLineRange(repoRoot, filePath string, startLine, endLine int) ([]CodeLine, error) {
	fullPath := filepath.Join(repoRoot, filePath)
	file, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var allLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	total := len(allLines)
	if total == 0 {
		return []CodeLine{}, nil
	}

	if startLine < 1 {
		startLine = 1
	}
	if endLine > total {
		endLine = total
	}
	if startLine > endLine {
		return []CodeLine{}, nil
	}

	var result []CodeLine
	for l := startLine; l <= endLine; l++ {
		result = append(result, CodeLine{
			LineNumber: l,
			Content:    allLines[l-1],
		})
	}

	return result, nil
}

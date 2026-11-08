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

// GetDiff runs git diff based on the specified mode (all, staged, unstaged, branch)
func GetDiff(mode string, ref string) (string, error) {
	var args []string
	hasHead := HasHeadCommit()

	switch mode {
	case "staged":
		args = []string{"diff", "-U3", "--cached"}
	case "unstaged":
		args = []string{"diff", "-U3"}
	case "branch":
		if ref == "" {
			ref = "main"
		}
		args = []string{"diff", "-U3", ref}
	case "all":
		fallthrough
	default:
		if ref != "" {
			args = []string{"diff", "-U3", ref}
		} else if hasHead {
			args = []string{"diff", "-U3", "HEAD"}
		} else {
			args = []string{"diff", "-U3"}
		}
	}

	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, string(out))
	}

	diffText := string(out)

	// If diff is empty and mode is unstaged or all, include untracked new files
	if strings.TrimSpace(diffText) == "" && (mode == "all" || mode == "unstaged" || mode == "") {
		untrackedDiff, _ := getUntrackedDiff()
		if untrackedDiff != "" {
			diffText = untrackedDiff
		}
	}

	return diffText, nil
}

// getUntrackedDiff provides a diff representation for newly created, untracked files
func getUntrackedDiff() (string, error) {
	cmd := exec.Command("git", "status", "--porcelain")
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
			// Generate diff against /dev/null for untracked files
			diffCmd := exec.Command("git", "diff", "--no-index", "--", "/dev/null", filePath)
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

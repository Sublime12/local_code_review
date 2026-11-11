package e2e

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sublime12/local_code_review/git"
	"github.com/Sublime12/local_code_review/server"
	"github.com/Sublime12/local_code_review/web"
	"github.com/mxschmitt/playwright-go"
)

var (
	pw            *playwright.Playwright
	browser       playwright.Browser
	testRepoRoot  string
	testConfigDir string
)

func TestMain(m *testing.M) {
	var err error
	testRepoRoot, err = git.GetRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to find git repo root: %v\n", err)
		os.Exit(1)
	}
	if err := os.Chdir(testRepoRoot); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to chdir to repo root: %v\n", err)
		os.Exit(1)
	}

	testConfigDir, err = os.MkdirTemp("", "review-e2e-config-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create temp config dir: %v\n", err)
		os.Exit(1)
	}
	_ = os.Setenv("REVIEW_CONFIG_DIR", testConfigDir)

	readmePath := filepath.Join(testRepoRoot, "README.md")
	originalReadme, readErr := os.ReadFile(readmePath)
	if readErr == nil {
		_ = os.WriteFile(readmePath, append(originalReadme, []byte("\n<!-- test-diff-marker -->\n")...), 0644)
	}

	pw, err = playwright.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start Playwright: %v\n", err)
		os.Exit(1)
	}

	browser, err = pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to launch Chromium: %v\n", err)
		_ = pw.Stop()
		os.Exit(1)
	}

	code := m.Run()

	_ = browser.Close()
	_ = pw.Stop()

	if readErr == nil {
		_ = os.WriteFile(readmePath, originalReadme, 0644)
	}

	_ = os.Remove(filepath.Join(testRepoRoot, ".review.md"))
	_ = os.RemoveAll(testConfigDir)

	os.Exit(code)
}

func startServer(t *testing.T, extraArgs ...string) (string, func()) {
	t.Helper()

	defaultRef := ""
	for i := 0; i < len(extraArgs)-1; i++ {
		if extraArgs[i] == "-ref" {
			defaultRef = extraArgs[i+1]
		}
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to allocate port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port

	srv := server.NewServer(testRepoRoot, web.Assets, defaultRef)
	httpServer := &http.Server{
		Handler: srv.Handler(),
	}

	go func() {
		_ = httpServer.Serve(l)
	}()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	cleanup := func() {
		_ = httpServer.Close()
		_ = l.Close()
	}

	return baseURL, cleanup
}

func resetServerState(t *testing.T, baseURL string) {
	t.Helper()
	resp, err := http.Post(baseURL+"/api/clear", "application/json", bytes.NewBuffer([]byte("{}")))
	if err == nil {
		_ = resp.Body.Close()
	}
	_ = os.Remove(filepath.Join(testRepoRoot, ".review.md"))
	if cfgPath, err := server.ConfigFilePath(); err == nil {
		_ = os.Remove(cfgPath)
	}
}

func newTestPage(t *testing.T) playwright.Page {
	t.Helper()
	context, err := browser.NewContext(playwright.BrowserNewContextOptions{
		Permissions: []string{"clipboard-read", "clipboard-write"},
	})
	if err != nil {
		t.Fatalf("Failed to create browser context: %v", err)
	}
	t.Cleanup(func() { _ = context.Close() })
	page, err := context.NewPage()
	if err != nil {
		t.Fatalf("Failed to create page: %v", err)
	}
	return page
}

func getFirstClickableLine(page playwright.Page) playwright.Locator {
	return page.Locator(".d2h-code-linenumber:not(.d2h-info), .d2h-code-side-linenumber:not(.d2h-info):not(.d2h-emptyplaceholder)").First()
}

func TestDashboardLoad(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()
	resetServerState(t, baseURL)

	page := newTestPage(t)

	if _, err := page.Goto(baseURL); err != nil {
		t.Fatalf("Failed to navigate to %s: %v", baseURL, err)
	}

	title, err := page.Title()
	if err != nil || !strings.Contains(title, "Local Code Review") {
		t.Errorf("Unexpected title: %q", title)
	}

	brand, err := page.TextContent(".brand h1")
	if err != nil || !strings.Contains(brand, "Local Code Review") {
		t.Errorf("Unexpected brand text: %q", brand)
	}

	branch, err := page.TextContent(".branch-badge")
	if err != nil || strings.TrimSpace(branch) == "" {
		t.Errorf("Branch badge is empty: %q", branch)
	}

	if err := page.Locator(".d2h-file-wrapper .d2h-file-name").First().WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("File wrapper did not appear: %v", err)
	}
	fileNames, err := page.Locator(".d2h-file-wrapper .d2h-file-name").AllTextContents()
	if err != nil || len(fileNames) == 0 {
		t.Fatalf("Expected file wrappers with names, got: %v (err: %v)", fileNames, err)
	}
	for i, name := range fileNames {
		if strings.TrimSpace(name) == "" {
			t.Errorf("File name at index %d is empty", i)
		}
	}

	viewSelect := page.Locator("select").Nth(1)
	selectedView, err := viewSelect.InputValue()
	if err != nil || selectedView != "line-by-line" {
		t.Errorf("Expected default view mode to be 'line-by-line', got: %q", selectedView)
	}
}

func TestUnifiedViewCommentFlow(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()
	resetServerState(t, baseURL)

	page := newTestPage(t)

	if _, err := page.Goto(baseURL); err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	line := getFirstClickableLine(page)
	if err := line.Click(); err != nil {
		t.Fatalf("Failed to click line number: %v", err)
	}

	textarea := page.Locator(".inline-comment-box textarea")
	testComment := "Go E2E unified view test comment"
	if err := textarea.Fill(testComment); err != nil {
		t.Fatalf("Failed to fill comment: %v", err)
	}

	if err := page.Locator(".inline-comment-box .btn-save").Click(); err != nil {
		t.Fatalf("Failed to click save button: %v", err)
	}

	commentCard := page.Locator(".existing-comment-card")
	if err := commentCard.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("Comment card did not become visible in unified view: %v", err)
	}

	cardText, _ := commentCard.TextContent()
	if !strings.Contains(cardText, testComment) {
		t.Errorf("Card text %q does not contain %q", cardText, testComment)
	}

	badge := page.Locator(".badge")
	badgeText, _ := badge.TextContent()
	if strings.TrimSpace(badgeText) != "1" {
		t.Errorf("Expected badge text '1', got %q", badgeText)
	}

	delBtn := commentCard.Locator("button:has-text('Delete')")
	if err := delBtn.Click(); err != nil {
		t.Fatalf("Failed to click delete: %v", err)
	}

	if err := commentCard.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden}); err != nil {
		t.Fatalf("Comment card did not disappear: %v", err)
	}
}

func TestSideBySideCommentFlow(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()
	resetServerState(t, baseURL)

	page := newTestPage(t)

	if _, err := page.Goto(baseURL); err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	viewSelect := page.Locator("select").Nth(1)
	if _, err := viewSelect.SelectOption(playwright.SelectOptionValues{Values: playwright.StringSlice("side-by-side")}); err != nil {
		t.Fatalf("Failed to select side-by-side view: %v", err)
	}

	lineNumbers := page.Locator(".d2h-code-side-linenumber:not(.d2h-info):not(.d2h-emptyplaceholder)")
	if err := lineNumbers.First().WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("Side-by-side line numbers did not appear: %v", err)
	}
	count, err := lineNumbers.Count()
	if err != nil || count == 0 {
		t.Fatalf("Expected line numbers, got count: %d (err: %v)", count, err)
	}

	firstLine := lineNumbers.First()
	if err := firstLine.Click(); err != nil {
		t.Fatalf("Failed to click line number: %v", err)
	}

	commentBox := page.Locator(".inline-comment-box")
	visible, err := commentBox.IsVisible()
	if err != nil || !visible {
		t.Fatalf("Inline comment box not visible")
	}

	textarea := page.Locator(".inline-comment-box textarea")
	testComment := "Go E2E side-by-side test comment"
	if err := textarea.Fill(testComment); err != nil {
		t.Fatalf("Failed to fill comment: %v", err)
	}

	if err := page.Locator(".inline-comment-box .btn-save").Click(); err != nil {
		t.Fatalf("Failed to click save button: %v", err)
	}

	commentCard := page.Locator(".existing-comment-card")
	if err := commentCard.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("Comment card did not become visible: %v", err)
	}

	cardText, _ := commentCard.TextContent()
	if !strings.Contains(cardText, testComment) {
		t.Errorf("Comment card text %q does not contain %q", cardText, testComment)
	}
}

func TestPersistentCommentsAcrossViewSwitch(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()
	resetServerState(t, baseURL)

	page := newTestPage(t)

	if _, err := page.Goto(baseURL); err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	line := getFirstClickableLine(page)
	if err := line.Click(); err != nil {
		t.Fatalf("Failed to click line number: %v", err)
	}

	textarea := page.Locator(".inline-comment-box textarea")
	commentText := "Comment persistent across view switch"
	_ = textarea.Fill(commentText)
	_ = page.Locator(".inline-comment-box .btn-save").Click()

	commentCard := page.Locator(".existing-comment-card")
	if err := commentCard.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("Comment card not visible: %v", err)
	}

	viewSelect := page.Locator("select").Nth(1)
	_, _ = viewSelect.SelectOption(playwright.SelectOptionValues{Values: playwright.StringSlice("side-by-side")})

	if err := commentCard.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("Comment card disappeared after switching to side-by-side view: %v", err)
	}

	_, _ = viewSelect.SelectOption(playwright.SelectOptionValues{Values: playwright.StringSlice("line-by-line")})
	if err := commentCard.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("Comment card disappeared after switching back to line-by-line view: %v", err)
	}
}

func TestConfigPersistence(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()
	resetServerState(t, baseURL)

	page := newTestPage(t)

	if _, err := page.Goto(baseURL); err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	viewSelect := page.Locator("select").Nth(1)
	if _, err := viewSelect.SelectOption(playwright.SelectOptionValues{Values: playwright.StringSlice("side-by-side")}); err != nil {
		t.Fatalf("Failed to switch view: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	cfgPath, err := server.ConfigFilePath()
	if err != nil {
		t.Fatalf("Failed to get config path: %v", err)
	}
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Fatalf("Expected config file %s to exist on disk", cfgPath)
	}

	if _, err := page.Reload(); err != nil {
		t.Fatalf("Failed to reload: %v", err)
	}

	val, err := page.Locator("select").Nth(1).InputValue()
	if err != nil || val != "side-by-side" {
		t.Errorf("Expected viewMode to persist as 'side-by-side', got %q", val)
	}
}

func TestSaveToFileAndToast(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()
	resetServerState(t, baseURL)

	page := newTestPage(t)

	if _, err := page.Goto(baseURL); err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	line := getFirstClickableLine(page)
	_ = line.Click()
	commentText := "Save to file security audit comment"
	_ = page.Locator(".inline-comment-box textarea").Fill(commentText)
	_ = page.Locator(".inline-comment-box .btn-save").Click()

	_ = page.Locator(".existing-comment-card").WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible})

	if err := page.Locator("button:has-text('Save to .review.md')").Click(); err != nil {
		t.Fatalf("Failed to click save to file: %v", err)
	}

	saveToast := page.Locator(".toast:has-text('Saved review to .review.md')")
	if err := saveToast.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("Toast notification did not appear: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(testRepoRoot, ".review.md"))
	if err != nil {
		t.Fatalf("Failed to read .review.md: %v", err)
	}
	if !strings.Contains(string(content), commentText) {
		t.Errorf(".review.md does not contain %q. Full content:\n%s", commentText, string(content))
	}
}

func TestCopyForAgentAndToast(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()
	resetServerState(t, baseURL)

	page := newTestPage(t)

	if _, err := page.Goto(baseURL); err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	line := getFirstClickableLine(page)
	_ = line.Click()
	_ = page.Locator(".inline-comment-box textarea").Fill("Prompt export test")
	_ = page.Locator(".inline-comment-box .btn-save").Click()

	_ = page.Locator(".existing-comment-card").WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible})

	if err := page.Locator("button:has-text('Copy for Agent')").Click(); err != nil {
		t.Fatalf("Failed to click copy for agent: %v", err)
	}

	copyToast := page.Locator(".toast:has-text('Copied formatted prompt')")
	if err := copyToast.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("Toast notification did not appear: %v", err)
	}
}

func TestCtrlEnterAndCancel(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()
	resetServerState(t, baseURL)

	page := newTestPage(t)

	if _, err := page.Goto(baseURL); err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	line := getFirstClickableLine(page)
	_ = line.Click()

	cancelBtn := page.Locator(".inline-comment-box .btn-cancel")
	if err := cancelBtn.Click(); err != nil {
		t.Fatalf("Failed to click cancel: %v", err)
	}
	visible, _ := page.Locator(".inline-comment-box").IsVisible()
	if visible {
		t.Fatalf("Comment box did not close on cancel")
	}

	_ = getFirstClickableLine(page).Click()
	textarea := page.Locator(".inline-comment-box textarea")
	_ = textarea.Fill("Saved with Ctrl+Enter in Go E2E")
	if err := textarea.Press("Control+Enter"); err != nil {
		t.Fatalf("Failed to press Control+Enter: %v", err)
	}

	commentCard := page.Locator(".existing-comment-card")
	if err := commentCard.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("Comment card did not appear after Ctrl+Enter: %v", err)
	}
}

func TestFileExplorerTreeAndNavigation(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()
	resetServerState(t, baseURL)

	page := newTestPage(t)

	if _, err := page.Goto(baseURL); err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	sidebar := page.Locator(".file-explorer-sidebar")
	if err := sidebar.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("Sidebar is not visible: %v", err)
	}

	title, err := page.TextContent(".sidebar-title-text, .sidebar-title span")
	if err != nil || !strings.Contains(title, "Files changed") {
		t.Fatalf("Unexpected sidebar title: %q", title)
	}

	fileRows := page.Locator(".tree-row.is-file")
	if err := fileRows.First().WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("File rows did not appear in tree: %v", err)
	}
	fileCount, err := fileRows.Count()
	if err != nil || fileCount == 0 {
		t.Fatalf("Expected file rows in tree, got count: %d", fileCount)
	}

	firstFile := fileRows.First()
	if err := firstFile.Click(); err != nil {
		t.Fatalf("Failed to click first file: %v", err)
	}

	activeFile := page.Locator(".tree-row.is-file.is-active")
	activeCount, err := activeFile.Count()
	if err != nil || activeCount == 0 {
		t.Fatalf("Expected active file row after click")
	}

	folderRows := page.Locator(".tree-row.is-folder")
	folderCount, _ := folderRows.Count()
	if folderCount > 0 {
		firstFolder := folderRows.First()
		initialVisibleFiles, _ := fileRows.Count()
		if err := firstFolder.Click(); err != nil {
			t.Fatalf("Failed to click folder row: %v", err)
		}
		afterCollapseFiles, _ := fileRows.Count()
		if afterCollapseFiles > initialVisibleFiles {
			t.Fatalf("File count should not increase after collapsing folder")
		}

		if err := firstFolder.Click(); err != nil {
			t.Fatalf("Failed to re-expand folder: %v", err)
		}
	}
}

func TestFileExplorerFilter(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()
	resetServerState(t, baseURL)

	page := newTestPage(t)

	if _, err := page.Goto(baseURL); err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	filterInput := page.Locator(".sidebar-filter-input")
	if err := filterInput.Fill("nonexistent_filename_xyz"); err != nil {
		t.Fatalf("Failed to fill filter: %v", err)
	}

	emptyNotice := page.Locator(".tree-empty")
	if err := emptyNotice.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("Empty notice did not appear for non-matching filter: %v", err)
	}

	clearBtn := page.Locator(".filter-clear-btn")
	if err := clearBtn.Click(); err != nil {
		t.Fatalf("Failed to click clear filter: %v", err)
	}

	if err := emptyNotice.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden}); err != nil {
		t.Fatalf("Empty notice did not hide after clearing filter: %v", err)
	}
}

func TestFileExplorerSidebarToggleAndPersistence(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()
	resetServerState(t, baseURL)

	page := newTestPage(t)

	if _, err := page.Goto(baseURL); err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	sidebar := page.Locator(".file-explorer-sidebar")
	if err := sidebar.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("Sidebar not visible initially: %v", err)
	}

	collapseBtn := page.Locator(".icon-btn[title='Collapse sidebar']")
	if err := collapseBtn.Click(); err != nil {
		t.Fatalf("Failed to click collapse button: %v", err)
	}

	if err := sidebar.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden}); err != nil {
		t.Fatalf("Sidebar did not hide after collapse: %v", err)
	}

	expandTab := page.Locator(".sidebar-expand-tab")
	if err := expandTab.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("Expand tab not visible when sidebar is collapsed: %v", err)
	}

	if err := expandTab.Click(); err != nil {
		t.Fatalf("Failed to click expand tab: %v", err)
	}

	if err := sidebar.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("Sidebar did not reopen from expand tab: %v", err)
	}

	topToggleBtn := page.Locator(".sidebar-toggle-btn")
	if err := topToggleBtn.Click(); err != nil {
		t.Fatalf("Failed to click top toggle button: %v", err)
	}

	if err := sidebar.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden}); err != nil {
		t.Fatalf("Sidebar did not hide after top toggle click: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	if _, err := page.Goto(baseURL); err != nil {
		t.Fatalf("Failed to reload: %v", err)
	}

	time.Sleep(300 * time.Millisecond)
	isVisible, err := page.Locator(".file-explorer-sidebar").IsVisible()
	if err != nil || isVisible {
		t.Fatalf("Sidebar should remain closed after reload based on saved config (isVisible: %v, err: %v)", isVisible, err)
	}
}

func TestUnifiedLineNumbersSingleRowAlignment(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()
	resetServerState(t, baseURL)

	page := newTestPage(t)

	if _, err := page.Goto(baseURL); err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	cntxCell := page.Locator("td.d2h-code-linenumber.d2h-cntx").First()
	if err := cntxCell.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("Context line number cell not visible: %v", err)
	}

	result, err := page.Evaluate(`() => {
		const cell = document.querySelector('td.d2h-code-linenumber.d2h-cntx');
		if (!cell) return { error: 'no context cell' };
		const tr = cell.closest('tr');
		const num1 = cell.querySelector('.line-num1');
		const num2 = cell.querySelector('.line-num2');
		const codeLine = tr.querySelector('.d2h-code-line');
		const r1 = num1.getBoundingClientRect();
		const r2 = num2.getBoundingClientRect();
		const rc = codeLine.getBoundingClientRect();
		const rtr = tr.getBoundingClientRect();
		return {
			top1: r1.top,
			top2: r2.top,
			topCode: rc.top,
			left1: r1.left,
			left2: r2.left,
			leftCode: rc.left,
			rowHeight: rtr.height
		};
	}`)
	if err != nil {
		t.Fatalf("Failed to evaluate bounding rects: %v", err)
	}

	metrics, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("Unexpected metrics type: %T", result)
	}
	if errMsg, hasErr := metrics["error"]; hasErr {
		t.Fatalf("Evaluate error: %v", errMsg)
	}

	top1 := toFloat64(metrics["top1"])
	top2 := toFloat64(metrics["top2"])
	topCode := toFloat64(metrics["topCode"])
	left1 := toFloat64(metrics["left1"])
	left2 := toFloat64(metrics["left2"])
	rowHeight := toFloat64(metrics["rowHeight"])

	if top1 != top2 {
		t.Errorf("Expected line-num1 and line-num2 on same horizontal line, got top1=%v, top2=%v", top1, top2)
	}
	if top1 != topCode {
		t.Errorf("Expected line numbers and code on same horizontal line, got top1=%v, topCode=%v", top1, topCode)
	}
	if left1 >= left2 {
		t.Errorf("Expected line-num1 to the left of line-num2, got left1=%v, left2=%v", left1, left2)
	}
	if rowHeight > 24 {
		t.Errorf("Expected single-line row height (<= 24px), got %v", rowHeight)
	}
}

func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	default:
		return 0
	}
}

func TestStickyFileHeader(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()
	resetServerState(t, baseURL)

	page := newTestPage(t)

	if _, err := page.Goto(baseURL); err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	fileHeader := page.Locator(".d2h-file-header").First()
	if err := fileHeader.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("File header not visible: %v", err)
	}

	res, err := page.Evaluate(`() => {
		const wrapper = document.querySelector('.d2h-file-wrapper');
		const header = wrapper.querySelector('.d2h-file-header');
		const topHeader = document.querySelector('.top-header');
		wrapper.style.minHeight = '2000px';
		document.body.style.minHeight = '3000px';
		window.scrollTo(0, 400);
		const headerStyle = window.getComputedStyle(header);
		const wrapperStyle = window.getComputedStyle(wrapper);
		const headerRect = header.getBoundingClientRect();
		const topHeaderRect = topHeader.getBoundingClientRect();
		return {
			position: headerStyle.position,
			wrapperOverflow: wrapperStyle.overflow,
			headerTop: headerRect.top,
			topHeaderBottom: topHeaderRect.bottom
		};
	}`)
	if err != nil {
		t.Fatalf("Failed to evaluate sticky file header: %v", err)
	}

	data := res.(map[string]interface{})
	if data["position"] != "sticky" {
		t.Errorf("Expected .d2h-file-header position 'sticky', got %v", data["position"])
	}
	headerTop := toFloat64(data["headerTop"])
	topHeaderBottom := toFloat64(data["topHeaderBottom"])
	diff := headerTop - topHeaderBottom
	if diff < -1 || diff > 1 {
		t.Errorf("Expected sticky file header top (%v) to match top-header bottom (%v) when scrolled", headerTop, topHeaderBottom)
	}
}

func TestResizableFileExplorerSidebar(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()
	resetServerState(t, baseURL)

	page := newTestPage(t)

	if _, err := page.Goto(baseURL); err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	sidebar := page.Locator(".file-explorer-sidebar")
	if err := sidebar.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("Sidebar not visible: %v", err)
	}

	initialBox, err := sidebar.BoundingBox()
	if err != nil || initialBox == nil {
		t.Fatalf("Failed to get sidebar bounding box: %v", err)
	}
	if initialBox.Width < 289 || initialBox.Width > 291 {
		t.Fatalf("Expected initial sidebar width ~290px, got %v", initialBox.Width)
	}

	resizer := page.Locator(".sidebar-resizer")
	resizerBox, err := resizer.BoundingBox()
	if err != nil || resizerBox == nil {
		t.Fatalf("Failed to get resizer bounding box: %v", err)
	}

	startX := resizerBox.X + resizerBox.Width/2
	startY := resizerBox.Y + 100
	targetWidth := 420.0

	if err := page.Mouse().Move(startX, startY); err != nil {
		t.Fatalf("Mouse move failed: %v", err)
	}
	if err := page.Mouse().Down(); err != nil {
		t.Fatalf("Mouse down failed: %v", err)
	}
	if err := page.Mouse().Move(targetWidth, startY); err != nil {
		t.Fatalf("Mouse drag failed: %v", err)
	}
	if err := page.Mouse().Up(); err != nil {
		t.Fatalf("Mouse up failed: %v", err)
	}

	resizedBox, err := sidebar.BoundingBox()
	if err != nil || resizedBox == nil {
		t.Fatalf("Failed to get resized bounding box: %v", err)
	}
	if resizedBox.Width < 418 || resizedBox.Width > 422 {
		t.Fatalf("Expected resized sidebar width ~420px, got %v", resizedBox.Width)
	}

	time.Sleep(250 * time.Millisecond)

	if _, err := page.Goto(baseURL); err != nil {
		t.Fatalf("Failed to reload page: %v", err)
	}
	if err := sidebar.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("Sidebar not visible after reload: %v", err)
	}

	persistedBox, err := sidebar.BoundingBox()
	if err != nil || persistedBox == nil {
		t.Fatalf("Failed to get persisted bounding box: %v", err)
	}
	if persistedBox.Width < 418 || persistedBox.Width > 422 {
		t.Fatalf("Expected persisted sidebar width ~420px after reload, got %v", persistedBox.Width)
	}
}

func TestExpandDiffContext(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()
	resetServerState(t, baseURL)

	page := newTestPage(t)

	if _, err := page.Goto(baseURL); err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	expandBtn := page.Locator(".hunk-expand-btn").First()
	if err := expandBtn.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("Hunk expand button not visible: %v", err)
	}

	initialCntxCount, err := page.Locator("td.d2h-code-linenumber.d2h-cntx").Count()
	if err != nil {
		t.Fatalf("Failed to count initial context lines: %v", err)
	}

	if err := expandBtn.Click(); err != nil {
		t.Fatalf("Failed to click hunk expand button: %v", err)
	}

	expandedRow := page.Locator("tr.d2h-cntx-expanded").First()
	if err := expandedRow.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("Expanded context rows did not appear after clicking hunk expand button: %v", err)
	}

	afterHunkExpandCount, _ := page.Locator("td.d2h-code-linenumber.d2h-cntx").Count()
	if afterHunkExpandCount <= initialCntxCount {
		t.Fatalf("Expected context line count to increase after hunk expand (before=%d, after=%d)", initialCntxCount, afterHunkExpandCount)
	}

	contextSelect := page.Locator("select.context-select")
	if _, err := contextSelect.SelectOption(playwright.SelectOptionValues{Values: playwright.StringSlice("25")}); err != nil {
		t.Fatalf("Failed to select 25 context lines: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	widerCntxCount, _ := page.Locator("td.d2h-code-linenumber.d2h-cntx").Count()
	if widerCntxCount <= initialCntxCount {
		t.Fatalf("Expected more context lines with 25-line context setting (initial=%d, got=%d)", initialCntxCount, widerCntxCount)
	}

	if _, err := page.Goto(baseURL); err != nil {
		t.Fatalf("Failed to reload page: %v", err)
	}

	selectedVal, err := page.Locator("select.context-select").InputValue()
	if err != nil || selectedVal != "25" {
		t.Fatalf("Expected persisted contextLines '25' after reload, got %q (err: %v)", selectedVal, err)
	}
}

func TestRefComparisonFlow(t *testing.T) {
	baseURL, cleanup := startServer(t, "-ref", "HEAD~1")
	defer cleanup()
	resetServerState(t, baseURL)

	page := newTestPage(t)

	if _, err := page.Goto(baseURL); err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	refBadge := page.Locator(".ref-badge")
	if err := refBadge.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("Ref badge not visible when started with -ref HEAD~1: %v", err)
	}
	badgeText, _ := refBadge.TextContent()
	if !strings.Contains(badgeText, "HEAD~1") {
		t.Errorf("Expected ref badge to contain 'HEAD~1', got %q", badgeText)
	}

	refInput := page.Locator("input.ref-input")
	inputVal, err := refInput.InputValue()
	if err != nil || inputVal != "HEAD~1" {
		t.Errorf("Expected ref-input value 'HEAD~1', got %q (err: %v)", inputVal, err)
	}

	if err := page.Locator(".d2h-file-wrapper").First().WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("Expected diff files when comparing against HEAD~1: %v", err)
	}

	if err := refInput.Fill("HEAD~2"); err != nil {
		t.Fatalf("Failed to fill ref-input: %v", err)
	}
	if err := refInput.Press("Enter"); err != nil {
		t.Fatalf("Failed to press Enter on ref-input: %v", err)
	}

	time.Sleep(250 * time.Millisecond)
	updatedBadge, _ := refBadge.TextContent()
	if !strings.Contains(updatedBadge, "HEAD~2") {
		t.Errorf("Expected updated ref badge to contain 'HEAD~2', got %q", updatedBadge)
	}
}

func TestTreeAndDiffViewOrderConsistency(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()
	resetServerState(t, baseURL)

	page := newTestPage(t)

	if _, err := page.Goto(baseURL); err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	firstWrapper := page.Locator(".d2h-file-wrapper").First()
	if err := firstWrapper.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("File wrapper not visible: %v", err)
	}

	treeFileOrder, err := page.Evaluate(`() => {
		const app = window.Alpine && window.Alpine.$data ? window.Alpine.$data(document.querySelector('[x-data]')) : null;
		if (!app || !app.treeNodes) return [];
		return app.treeNodes.filter(n => !n.isFolder).map(n => n.path);
	}`)
	if err != nil {
		t.Fatalf("Failed to get tree file order: %v", err)
	}

	domFileOrder, err := page.Evaluate(`() => {
		return Array.from(document.querySelectorAll('.d2h-file-wrapper')).map(el => {
			return el.getAttribute('data-file-path') || el.querySelector('.d2h-file-name')?.textContent?.trim() || '';
		}).filter(Boolean);
	}`)
	if err != nil {
		t.Fatalf("Failed to get DOM file order: %v", err)
	}

	treeList, ok1 := treeFileOrder.([]interface{})
	domList, ok2 := domFileOrder.([]interface{})
	if !ok1 || !ok2 {
		t.Fatalf("Expected slices for file orders, got %T and %T", treeFileOrder, domFileOrder)
	}

	if len(treeList) == 0 {
		t.Fatalf("Expected at least one file in tree")
	}
	if len(treeList) != len(domList) {
		t.Fatalf("Mismatch in file count between tree (%d) and view (%d)", len(treeList), len(domList))
	}

	for i := range treeList {
		if treeList[i] != domList[i] {
			t.Errorf("Order mismatch at index %d: tree has %v, diff view has %v", i, treeList[i], domList[i])
		}
	}
}

func TestHunkExpandTopAndBottomButtons(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()
	resetServerState(t, baseURL)

	page := newTestPage(t)

	if _, err := page.Goto(baseURL); err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	firstBtn := page.Locator(".hunk-expand-btn").First()
	if err := firstBtn.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("No hunk expand buttons found: %v", err)
	}

	initialExpandedCount, _ := page.Locator("tr.d2h-cntx-expanded").Count()

	if err := firstBtn.Click(); err != nil {
		t.Fatalf("Failed to click first hunk expand button: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	afterClickExpandedCount, _ := page.Locator("tr.d2h-cntx-expanded").Count()
	if afterClickExpandedCount <= initialExpandedCount {
		t.Fatalf("Expected expanded rows to increase after clicking expand button (before=%d, after=%d)", initialExpandedCount, afterClickExpandedCount)
	}

	bottomExpander := page.Locator(".d2h-bottom-expander-row .hunk-expand-btn").First()
	bottomCount, _ := bottomExpander.Count()
	if bottomCount > 0 {
		prevTotal, _ := page.Locator("tr.d2h-cntx-expanded").Count()
		if err := bottomExpander.Click(); err != nil {
			t.Fatalf("Failed to click bottom expander button: %v", err)
		}
		time.Sleep(300 * time.Millisecond)
		newTotal, _ := page.Locator("tr.d2h-cntx-expanded").Count()
		if newTotal <= prevTotal {
			t.Fatalf("Expected bottom expansion to add expanded rows (prev=%d, new=%d)", prevTotal, newTotal)
		}
	}
}

func TestUnrecognizedLanguageDefaultHighlighting(t *testing.T) {
	baseURL, cleanup := startServer(t)
	defer cleanup()
	resetServerState(t, baseURL)

	page := newTestPage(t)

	if _, err := page.Goto(baseURL); err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	firstWrapper := page.Locator(".d2h-file-wrapper").First()
	if err := firstWrapper.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("File wrapper not visible: %v", err)
	}

	res, err := page.Evaluate(`() => {
		const app = window.Alpine && window.Alpine.$data ? window.Alpine.$data(document.querySelector('[x-data]')) : null;
		if (!app || !app.hljs) return { error: "no hljs on app" };

		const sampleCode = "pub fn calculate(val: usize) void { while (val > 0) { if (val == 42) return; } }";
		const outZig = app.highlightCode("sample.zig", sampleCode);
		const outGeneric = app.highlightCode("custom.unknown", sampleCode);

		return {
			zigHasKeyword: outZig.includes("hljs-keyword"),
			zigHasTitle: outZig.includes("hljs-title"),
			genericHasKeyword: outGeneric.includes("hljs-keyword"),
			genericHasTitle: outGeneric.includes("hljs-title"),
			outZig: outZig,
			outGeneric: outGeneric
		};
	}`)
	if err != nil {
		t.Fatalf("Failed to evaluate highlightCode: %v", err)
	}

	data, ok := res.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map result, got %T: %v", res, res)
	}
	if data["error"] != nil {
		t.Fatalf("Highlight evaluation error: %v", data["error"])
	}
	if data["zigHasKeyword"] != true {
		t.Errorf("Expected zig highlighting to contain hljs-keyword: %+v", data)
	}
	if data["zigHasTitle"] != true {
		t.Errorf("Expected zig highlighting to contain hljs-title: %+v", data)
	}
	if data["genericHasKeyword"] != true {
		t.Errorf("Expected unrecognized language highlighting to contain hljs-keyword: %+v", data)
	}
	if data["genericHasTitle"] != true {
		t.Errorf("Expected unrecognized language highlighting to contain hljs-title: %+v", data)
	}
}

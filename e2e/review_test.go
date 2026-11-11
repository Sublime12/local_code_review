package e2e

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"
)

var (
	pw      *playwright.Playwright
	browser playwright.Browser
)

func TestMain(m *testing.M) {
	buildCmd := exec.Command("go", "build", "-o", "../local-review", "../main.go")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build local-review: %v\nOutput: %s\n", err, string(out))
		os.Exit(1)
	}

	readmePath := "../README.md"
	originalReadme, readErr := os.ReadFile(readmePath)
	if readErr == nil {
		_ = os.WriteFile(readmePath, append(originalReadme, []byte("\n<!-- test-diff-marker -->\n")...), 0644)
	}

	var err error
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

	_ = os.Remove("../.review.md")
	_ = os.Remove("../.local-review.json")

	os.Exit(code)
}

func startServer(t *testing.T) (string, func()) {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to allocate port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("Failed to get repo root: %v", err)
	}

	cmd := exec.Command("./local-review", "-no-browser", "-port", strconv.Itoa(port))
	cmd.Dir = repoRoot
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start local-review server: %v", err)
	}

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	ready := false
	for i := 0; i < 30; i++ {
		resp, err := http.Get(baseURL + "/api/status")
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !ready {
		_ = cmd.Process.Kill()
		t.Fatalf("Server timed out starting at %s", baseURL)
	}

	cleanup := func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}

	return baseURL, cleanup
}

func resetServerState(t *testing.T, baseURL string) {
	t.Helper()
	resp, err := http.Post(baseURL+"/api/clear", "application/json", bytes.NewBuffer([]byte("{}")))
	if err == nil {
		_ = resp.Body.Close()
	}
	_ = os.Remove("../.review.md")
	_ = os.Remove("../.local-review.json")
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

	if _, err := os.Stat("../.local-review.json"); os.IsNotExist(err) {
		t.Fatalf("Expected .local-review.json to exist on disk")
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

	content, err := os.ReadFile("../.review.md")
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

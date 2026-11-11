# Local Code Review

A local, GitHub PR-style code review tool that runs completely offline as a **single Go binary**.

Built specifically for AI coding agent workflows (Claude Code, Cursor, Codex, local agents, etc.). Review code modifications locally, click on any line to add inline review comments, and copy/export all feedback cleanly formatted with file paths, line numbers, and code context.

---

## Features

- **GitHub PR-Style Interface:** Side-by-side (split) and line-by-line (unified) views powered by `diff2html` and `Alpine.js`.
- **Changed Files Tree Explorer:** Collapsible sidebar with directory tree hierarchy, file status badges (`A`, `M`, `D`, `R`), additions/deletions stats (`+N / -N`), real-time filename search filter, and draggable width resizer.
- **Synchronized File Ordering:** 1:1 file order synchronization between the sidebar tree and the diff view.
- **Sticky File Headers:** File headers stay pinned cleanly beneath the top navigation bar as you scroll through long diffs.
- **GitHub-Style Hunk Expansion:** Expand context lines upward (`▲`), downward (`▼`), between hunks, and at the bottom of the file in 5–7 line increments, or select default context sizes (3, 10, 25, 50 lines).
- **Default Syntax Highlighting:** Universal syntax highlighter for languages not bundled in standard Highlight.js (Zig, Odin, V, Nim, Gleam, etc.), covering conditionals (`if`, `else`, `switch`), loops (`for`, `while`), function calls, types, constants, properties, and comments.
- **Inline Commenting:** Click on any line number (including dynamically expanded context lines) to attach review comments (`Ctrl+Enter` to save).
- **AI Agent Prompt Formatter:** Formats all comments with target line numbers and surrounding code context (e.g. 2 lines before and after).
- **Single-Click Copy & Save:** Copy formatted agent prompts directly to clipboard or export to `.review.md`.
- **Git Ref & Branch Comparison:** Compare working directory changes against any branch or commit using the `-ref` CLI flag or the live top-bar input.
- **Clean OS User Config Storage:** Preferences (view mode, diff mode, sidebar width, context lines) are stored in standard OS user config directories (`~/.config/review/config.json` on Linux, `~/Library/Application Support/review/` on macOS, `%AppData%\review\` on Windows) without polluting your repository.
- **100% Offline & Zero Runtime Dependencies:** All web assets (`Alpine.js`, `diff2html`, dark themes) are embedded inside the Go executable using `//go:embed`.

---

## Quick Start

### Installation

Clone and build the binary:

```bash
git clone https://github.com/Sublime12/local_code_review.git
cd local_code_review
make all # builds review and review-e2e
```

Move `review` to your `$PATH` (e.g. `~/.local/bin` or `/usr/local/bin`):

```bash
mv review ~/.local/bin/
```

### Usage

Run inside any git repository:

```bash
review
```

The review server will start on a free local port and display the access URL.

#### CLI Flags

```bash
# Run on a specific port
review -port 8080

# Open your browser automatically on startup
review --browser

# Compare working tree against a specific git ref or branch
review -ref main
review -ref HEAD~1
```

---

## The AI Agent Review Loop

1. **Ask your agent** to implement a feature or refactor code.
2. **Launch review:**
   ```bash
   review
   ```
3. **Review & Comment:**
   - Click the line number on any change or expanded context line.
   - Type your feedback (e.g., *"Handle the nil pointer case here before dereferencing"*).
   - Press `Ctrl+Enter` or click **Add Comment**.
4. **Export:** Click **"Copy for Agent"**.
5. **Paste back to agent:** The generated prompt format looks like:

```text
review: you should do this per example instead of doing this
file_path/a/b/c.js:89
87 content
88 content
89 content
90 content
91 content
```

---

## Tech Stack

- **Backend & CLI:** Go 1.21+ (`net/http`, `os/exec`, `//go:embed`)
- **Frontend State:** [Alpine.js v3](https://alpinejs.dev/)
- **Diff Engine:** [diff2html](https://diff2html.xyz/)
- **Styling:** Custom GitHub Dark theme

---

## Running Tests

### Unit Tests
```bash
make test
# or: go test . ./git ./server -v
```

### Playwright E2E Functional Tests (Pure Go, Zero npm)
You can run the full browser automation suite directly using standard Go:
```bash
make test-e2e
# or: make review-e2e && ./review-e2e -test.v
```

---

## License

MIT

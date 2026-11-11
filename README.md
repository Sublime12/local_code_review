# Local Code Review

A local, GitHub PR-style code review tool that runs completely offline as a **single Go binary**.

Built specifically for AI coding agent workflows (Claude Code, Cursor, Codex, local agents, etc.). Review code modifications locally, click on any line to add inline review comments, and copy/export all feedback cleanly formatted with file paths, line numbers, and code context.

---

## Features

- **GitHub PR-Style Interface:** Side-by-side (split) and line-by-line (unified) views powered by `diff2html` and `Alpine.js`.
- **Inline Commenting:** Click on any line number to attach review comments.
- **AI Agent Prompt Formatter:** Formats all comments with target line numbers and surrounding code context (e.g. 2 lines before and after).
- **Single-Click Copy & Save:** Copy the prompt to clipboard or save directly to `.review.md`.
- **100% Offline & Zero Runtime Dependencies:** All HTML, CSS, and JS libraries (`Alpine.js`, `diff2html`) are embedded inside the Go executable using `//go:embed`.
- **Flexible Diff Modes:** Switch between uncommitted working tree changes, staged changes only, or compare against branches.

---

## Quick Start

### Installation

Clone and build the binary:

```bash
git clone https://github.com/Sublime12/local_code_review.git
cd local_code_review
make all # builds local-review and review-e2e
```

Move it to your `$PATH` (e.g. `~/.local/bin` or `/usr/local/bin`):

```bash
mv local-review ~/.local/bin/
```

### Usage

Run inside any git repository:

```bash
local-review
```

The server starts on a local port and prints the URL to open in your browser.

#### CLI Flags

```bash
# Run on a specific port
local-review -port 8080

# Open the default browser automatically on startup
local-review --browser

# Compare against a specific git ref/branch
local-review -ref main
```

---

## The AI Agent Review Loop

1. **Ask your agent** to implement a feature or refactor code.
2. **Launch review:**
   ```bash
   local-review
   ```
3. **Review & Comment:**
   - Click the line number on any change.
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
- **Styling:** Custom dark GitHub theme

---

## Running Tests

### Unit Tests
```bash
go test ./git ./server -v
```

### Playwright E2E Functional Tests (Pure Go, Zero npm)
You can run the full browser automation suite directly using standard Go:
```bash
go test -v ./e2e/...
```

Or compile and run the dedicated test binary:
```bash
go build -o review-e2e ./cmd/review-e2e
./review-e2e
```

---

## License

MIT

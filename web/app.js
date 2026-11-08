function reviewApp() {
  return {
    branch: 'HEAD',
    repoRoot: '',
    diffMode: 'all',
    viewMode: 'side-by-side', // 'side-by-side' or 'line-by-line'
    diffText: '',
    comments: [],
    loading: true,
    empty: false,
    toastMessage: '',
    toastVisible: false,

    async init() {
      await this.loadStatus();
      await this.loadComments();
      await this.loadDiff();
    },

    showToast(message) {
      this.toastMessage = message;
      this.toastVisible = true;
      setTimeout(() => {
        this.toastVisible = false;
      }, 3000);
    },

    async loadStatus() {
      try {
        const res = await fetch('/api/status');
        const data = await res.json();
        this.branch = data.branch || 'HEAD';
        this.repoRoot = data.repoRoot || '';
      } catch (err) {
        console.error('Error fetching status:', err);
      }
    },

    async loadComments() {
      try {
        const res = await fetch('/api/comments');
        this.comments = await res.json();
      } catch (err) {
        console.error('Error fetching comments:', err);
      }
    },

    async loadDiff() {
      this.loading = true;
      try {
        const res = await fetch(`/api/diff?mode=${this.diffMode}`);
        const data = await res.json();
        this.diffText = data.diff || '';
        this.empty = !this.diffText.trim();
        this.renderDiff();
      } catch (err) {
        console.error('Error fetching diff:', err);
        this.empty = true;
      } finally {
        this.loading = false;
      }
    },

    switchView(mode) {
      this.viewMode = mode;
      this.renderDiff();
    },

    switchDiffMode(mode) {
      this.diffMode = mode;
      this.loadDiff();
    },

    renderDiff() {
      const container = document.getElementById('diff-viewer');
      if (!container) return;

      if (this.empty) {
        container.innerHTML = `
          <div class="empty-state">
            <h2>No changes detected</h2>
            <p>Your working tree is clean. Modify some files or switch the diff mode in the top bar.</p>
          </div>
        `;
        return;
      }

      const diffHtml = Diff2Html.html(this.diffText, {
        drawFileList: true,
        matching: 'lines',
        outputFormat: this.viewMode,
        rawTemplates: {
          'generic-file-path': '<span class="d2h-file-name">{{{file.newName}}}</span>'
        }
      });

      container.innerHTML = diffHtml;
      this.attachLineListeners();
      this.renderExistingComments();
    },

    attachLineListeners() {
      const fileWrappers = document.querySelectorAll('.d2h-file-wrapper');
      fileWrappers.forEach(fileEl => {
        const fileNameEl = fileEl.querySelector('.d2h-file-name');
        if (!fileNameEl) return;
        const filePath = fileNameEl.textContent.trim();

        const lineNumbers = fileEl.querySelectorAll('.d2h-code-linenumber');
        lineNumbers.forEach(lineEl => {
          lineEl.title = "Click to add a review comment";
          lineEl.addEventListener('click', (e) => {
            e.stopPropagation();
            const rawLine = lineEl.textContent.replace(/\+/g, '').replace(/-/g, '').trim();
            const lineNum = parseInt(rawLine, 10);
            if (!lineNum || isNaN(lineNum)) return;

            const row = lineEl.closest('tr');
            if (row) {
              this.showCommentInput(filePath, lineNum, row);
            }
          });
        });
      });
    },

    showCommentInput(filePath, lineNumber, rowElement) {
      // Check if an input is already open for this line
      const nextSibling = rowElement.nextElementSibling;
      if (nextSibling && nextSibling.classList.contains('inline-comment-row')) {
        const textarea = nextSibling.querySelector('textarea');
        if (textarea) textarea.focus();
        return;
      }

      const commentRow = document.createElement('tr');
      commentRow.className = 'inline-comment-row';
      const colSpan = rowElement.children.length || 2;

      commentRow.innerHTML = `
        <td colspan="${colSpan}">
          <div class="inline-comment-box">
            <div class="comment-box-header">
              <span><strong>Add Comment</strong> on <code>${filePath}:${lineNumber}</code></span>
              <span style="font-size: 0.75rem;">Press Ctrl+Enter to save</span>
            </div>
            <textarea class="comment-textarea" placeholder="Leave a review comment for the agent (e.g., 'You should handle errors here instead of ignoring them')..."></textarea>
            <div class="comment-actions">
              <button type="button" class="btn btn-secondary btn-cancel">Cancel</button>
              <button type="button" class="btn btn-primary btn-save">Add Comment</button>
            </div>
          </div>
        </td>
      `;

      rowElement.parentNode.insertBefore(commentRow, rowElement.nextSibling);

      const textarea = commentRow.querySelector('textarea');
      const saveBtn = commentRow.querySelector('.btn-save');
      const cancelBtn = commentRow.querySelector('.btn-cancel');

      textarea.focus();

      const save = async () => {
        const text = textarea.value.trim();
        if (!text) return;
        await this.addComment(filePath, lineNumber, text);
        commentRow.remove();
        this.renderExistingComments();
      };

      saveBtn.addEventListener('click', save);
      cancelBtn.addEventListener('click', () => commentRow.remove());
      textarea.addEventListener('keydown', (e) => {
        if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
          save();
        }
      });
    },

    async addComment(filePath, line, text) {
      try {
        const res = await fetch('/api/comments', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ filePath, line, text })
        });
        const savedComment = await res.json();
        this.comments.push(savedComment);
        this.showToast(`Comment added to ${filePath}:${line}`);
      } catch (err) {
        console.error('Error saving comment:', err);
      }
    },

    async deleteComment(id) {
      try {
        await fetch(`/api/comments?id=${encodeURIComponent(id)}`, { method: 'DELETE' });
        this.comments = this.comments.filter(c => c.id !== id);
        this.renderExistingComments();
        this.showToast('Comment deleted');
      } catch (err) {
        console.error('Error deleting comment:', err);
      }
    },

    renderExistingComments() {
      // Remove all currently rendered comment display cards in the table
      document.querySelectorAll('.rendered-comment-row').forEach(el => el.remove());

      const fileWrappers = document.querySelectorAll('.d2h-file-wrapper');
      fileWrappers.forEach(fileEl => {
        const fileNameEl = fileEl.querySelector('.d2h-file-name');
        if (!fileNameEl) return;
        const filePath = fileNameEl.textContent.trim();

        const fileComments = this.comments.filter(c => c.filePath === filePath);
        fileComments.forEach(comment => {
          // Find the row with matching line number
          const lineElements = fileEl.querySelectorAll('.d2h-code-linenumber');
          for (let el of lineElements) {
            const num = parseInt(el.textContent.replace(/\+/g, '').replace(/-/g, '').trim(), 10);
            if (num === comment.line) {
              const row = el.closest('tr');
              if (row) {
                this.insertCommentCard(row, comment);
                break;
              }
            }
          }
        });
      });
    },

    insertCommentCard(rowElement, comment) {
      const commentRow = document.createElement('tr');
      commentRow.className = 'rendered-comment-row';
      const colSpan = rowElement.children.length || 2;

      commentRow.innerHTML = `
        <td colspan="${colSpan}">
          <div class="existing-comment-card">
            <div class="existing-comment-header">
              <span>💬 <strong>Line ${comment.line}</strong></span>
              <button class="btn btn-danger" style="padding: 2px 8px; font-size: 0.75rem;" data-id="${comment.id}">Delete</button>
            </div>
            <div class="existing-comment-body">${escapeHtml(comment.text)}</div>
          </div>
        </td>
      `;

      rowElement.parentNode.insertBefore(commentRow, rowElement.nextSibling);

      const delBtn = commentRow.querySelector('button');
      if (delBtn) {
        delBtn.addEventListener('click', () => this.deleteComment(comment.id));
      }
    },

    async copyPrompt() {
      try {
        const res = await fetch('/api/export', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ saveToFile: false })
        });
        const data = await res.json();
        await navigator.clipboard.writeText(data.prompt);
        this.showToast('📋 Copied formatted prompt to clipboard!');
      } catch (err) {
        console.error('Error copying prompt:', err);
        alert('Failed to copy to clipboard.');
      }
    },

    async saveToFile() {
      try {
        const res = await fetch('/api/export', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ saveToFile: true, filename: '.review.md' })
        });
        await res.json();
        this.showToast('💾 Saved review to .review.md in repository root!');
      } catch (err) {
        console.error('Error saving review file:', err);
      }
    }
  };
}

function escapeHtml(str) {
  return str.replace(/&/g, "&amp;")
            .replace(/</g, "&lt;")
            .replace(/>/g, "&gt;")
            .replace(/"/g, "&quot;")
            .replace(/'/g, "&#039;");
}

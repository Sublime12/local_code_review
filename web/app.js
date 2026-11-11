function reviewApp() {
  return {
    branch: 'HEAD',
    repoRoot: '',
    ref: '',
    diffMode: 'all',
    viewMode: 'line-by-line',
    contextLines: 3,
    diffText: '',
    comments: [],
    loading: true,
    empty: false,
    toastMessage: '',
    toastVisible: false,
    sidebarOpen: true,
    sidebarWidth: 290,
    isResizingSidebar: false,
    fileFilter: '',
    activeFile: '',
    filesList: [],
    treeNodes: [],
    scrollObserver: null,

    async init() {
      this.setupHeaderObserver();
      await this.loadConfig();
      await this.loadStatus();
      await this.loadComments();
      await this.loadDiff();
    },

    setupHeaderObserver() {
      const header = document.querySelector('.top-header');
      if (!header) return;
      const updateHeight = () => {
        const h = header.getBoundingClientRect().height || header.offsetHeight;
        document.documentElement.style.setProperty('--top-header-height', `${h}px`);
      };
      updateHeight();
      if (typeof ResizeObserver !== 'undefined') {
        const ro = new ResizeObserver(updateHeight);
        ro.observe(header);
      }
    },

    showToast(message) {
      this.toastMessage = message;
      this.toastVisible = true;
      setTimeout(() => {
        this.toastVisible = false;
      }, 3000);
    },

    async loadConfig() {
      try {
        const res = await fetch('/api/config');
        if (res.ok) {
          const cfg = await res.json();
          if (cfg.viewMode) this.viewMode = cfg.viewMode;
          if (cfg.diffMode) this.diffMode = cfg.diffMode;
          if (typeof cfg.sidebarOpen === 'boolean') this.sidebarOpen = cfg.sidebarOpen;
          if (typeof cfg.sidebarWidth === 'number' && cfg.sidebarWidth >= 160 && cfg.sidebarWidth <= 800) {
            this.sidebarWidth = cfg.sidebarWidth;
          }
          if (typeof cfg.contextLines === 'number' && cfg.contextLines > 0) {
            this.contextLines = cfg.contextLines;
          }
        }
      } catch (err) {
        console.error('Error loading config:', err);
      }
    },

    async saveConfig() {
      try {
        await fetch('/api/config', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            viewMode: this.viewMode,
            diffMode: this.diffMode,
            sidebarOpen: this.sidebarOpen,
            sidebarWidth: this.sidebarWidth,
            contextLines: Number(this.contextLines) || 3
          })
        });
      } catch (err) {
        console.error('Error saving config:', err);
      }
    },

    toggleSidebar() {
      this.sidebarOpen = !this.sidebarOpen;
      this.saveConfig();
    },

    startSidebarResize() {
      this.isResizingSidebar = true;
      document.body.classList.add('resizing-sidebar');
      const onMouseMove = (moveEvent) => {
        if (!this.isResizingSidebar) return;
        const clamped = Math.min(800, Math.max(160, Math.round(moveEvent.clientX)));
        this.sidebarWidth = clamped;
      };
      const onMouseUp = () => {
        if (!this.isResizingSidebar) return;
        this.isResizingSidebar = false;
        document.body.classList.remove('resizing-sidebar');
        window.removeEventListener('mousemove', onMouseMove);
        window.removeEventListener('mouseup', onMouseUp);
        this.saveConfig();
      };
      window.addEventListener('mousemove', onMouseMove);
      window.addEventListener('mouseup', onMouseUp);
    },

    buildTree() {
      const root = { path: '', childrenDirs: new Map(), files: [] };
      this.filesList.forEach(f => {
        const parts = f.path.split('/');
        let curr = root;
        for (let i = 0; i < parts.length - 1; i++) {
          const dirName = parts[i];
          const dirPath = parts.slice(0, i + 1).join('/');
          if (!curr.childrenDirs.has(dirName)) {
            curr.childrenDirs.set(dirName, {
              name: dirName,
              path: dirPath,
              childrenDirs: new Map(),
              files: []
            });
          }
          curr = curr.childrenDirs.get(dirName);
        }
        curr.files.push(f);
      });

      const existingExpanded = new Map();
      if (this.treeNodes) {
        for (const node of this.treeNodes) {
          if (node.isFolder) existingExpanded.set(node.path, node.expanded);
        }
      }

      const nodes = [];
      function traverse(dir, depth) {
        const dirNames = Array.from(dir.childrenDirs.keys()).sort();
        for (const dName of dirNames) {
          const childDir = dir.childrenDirs.get(dName);
          const isExpanded = existingExpanded.has(childDir.path) ? existingExpanded.get(childDir.path) : true;
          nodes.push({
            id: 'dir:' + childDir.path,
            name: childDir.name,
            path: childDir.path,
            isFolder: true,
            depth: depth,
            expanded: isExpanded
          });
          traverse(childDir, depth + 1);
        }
        dir.files.sort((a, b) => a.name.localeCompare(b.name));
        for (const file of dir.files) {
          nodes.push({
            id: 'file:' + file.path,
            name: file.name,
            path: file.path,
            isFolder: false,
            depth: depth,
            status: file.status,
            addedLines: file.addedLines,
            deletedLines: file.deletedLines
          });
        }
      }

      traverse(root, 0);
      this.treeNodes = nodes;
    },

    get visibleTreeItems() {
      if (!this.fileFilter) {
        const collapsed = new Set(this.treeNodes.filter(n => n.isFolder && !n.expanded).map(n => n.path));
        return this.treeNodes.filter(n => {
          const parts = n.path.split('/');
          let prefix = parts[0];
          for (let i = 0; i < parts.length - 1; i++) {
            if (collapsed.has(prefix)) return false;
            prefix += '/' + parts[i + 1];
          }
          return true;
        });
      }

      const query = this.fileFilter.toLowerCase().trim();
      const matchingFilePaths = new Set();
      const neededDirPaths = new Set();

      for (const n of this.treeNodes) {
        if (!n.isFolder && n.path.toLowerCase().includes(query)) {
          matchingFilePaths.add(n.path);
          const parts = n.path.split('/');
          let prefix = parts[0];
          for (let i = 0; i < parts.length - 1; i++) {
            neededDirPaths.add(prefix);
            prefix += '/' + parts[i + 1];
          }
        }
      }

      return this.treeNodes.filter(n => {
        if (n.isFolder) return neededDirPaths.has(n.path);
        return matchingFilePaths.has(n.path);
      });
    },

    toggleFolder(node) {
      node.expanded = !node.expanded;
    },

    get allFoldersExpanded() {
      const folders = this.treeNodes.filter(n => n.isFolder);
      if (!folders.length) return true;
      return folders.every(f => f.expanded);
    },

    toggleAllFolders() {
      const target = !this.allFoldersExpanded;
      this.treeNodes.forEach(n => {
        if (n.isFolder) n.expanded = target;
      });
    },

    scrollToFile(filePath) {
      this.activeFile = filePath;
      const target = document.querySelector(`[data-file-path="${CSS.escape(filePath)}"]`);
      if (target) {
        target.scrollIntoView({ behavior: 'smooth', block: 'start' });
      }
    },

    setupScrollSpy() {
      if (this.scrollObserver) {
        this.scrollObserver.disconnect();
      }
      const wrappers = document.querySelectorAll('.d2h-file-wrapper');
      if (!wrappers.length || typeof IntersectionObserver === 'undefined') return;

      const callback = (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            const filePath = entry.target.getAttribute('data-file-path');
            if (filePath) {
              this.activeFile = filePath;
            }
          }
        }
      };

      this.scrollObserver = new IntersectionObserver(callback, {
        rootMargin: '-70px 0px -70% 0px',
        threshold: 0
      });

      wrappers.forEach(w => this.scrollObserver.observe(w));
    },

    getStatusLabel(status) {
      switch (status) {
        case 'A': return 'Added';
        case 'D': return 'Deleted';
        case 'R': return 'Renamed';
        case 'M': return 'Modified';
        default: return status;
      }
    },

    getFileCommentsCount(filePath) {
      if (!this.comments || !this.comments.length) return 0;
      return this.comments.filter(c => c.filePath === filePath).length;
    },

    async loadStatus() {
      try {
        const res = await fetch('/api/status');
        const data = await res.json();
        this.branch = data.branch || 'HEAD';
        this.repoRoot = data.repoRoot || '';
        if (data.ref) {
          this.ref = data.ref;
        }
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
        const refParam = this.ref ? `&ref=${encodeURIComponent(this.ref.trim())}` : '&ref=';
        const res = await fetch(`/api/diff?mode=${this.diffMode}&context=${this.contextLines}${refParam}`);
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

    applyRef() {
      this.ref = this.ref.trim();
      this.loadDiff();
    },

    switchView(mode) {
      this.viewMode = mode;
      this.saveConfig();
      this.renderDiff();
    },

    switchDiffMode(mode) {
      this.diffMode = mode;
      this.saveConfig();
      this.loadDiff();
    },

    switchContextLines(lines) {
      this.contextLines = parseInt(lines, 10) || 3;
      this.saveConfig();
      this.loadDiff();
    },

    renderDiff() {
      const container = document.getElementById('diff-viewer');
      if (!container) return;

      if (this.empty) {
        this.filesList = [];
        this.treeNodes = [];
        container.innerHTML = `
          <div class="empty-state">
            <h2>No changes detected</h2>
            <p>Your working tree is clean. Modify some files or switch the diff mode in the top bar.</p>
          </div>
        `;
        return;
      }

      if (typeof Diff2HtmlUI !== 'undefined') {
        const diff2htmlUi = new Diff2HtmlUI(container, this.diffText, {
          drawFileList: false,
          matching: 'none',
          outputFormat: this.viewMode,
          highlight: true
        });
        diff2htmlUi.draw();
      } else if (typeof Diff2Html !== 'undefined') {
        const diffHtml = Diff2Html.html(this.diffText, {
          drawFileList: false,
          matching: 'none',
          outputFormat: this.viewMode
        });
        container.innerHTML = diffHtml;
      }

      let parsedFiles = [];
      if (typeof Diff2Html !== 'undefined' && typeof Diff2Html.parse === 'function') {
        try {
          parsedFiles = Diff2Html.parse(this.diffText);
        } catch (e) {
          console.error('Error parsing diff with Diff2Html:', e);
        }
      }

      if (parsedFiles.length > 0) {
        this.filesList = parsedFiles.map(f => {
          const filePath = (f.newName && f.newName !== '/dev/null' && f.newName !== 'dev/null') ? f.newName : f.oldName;
          let status = 'M';
          if (f.isNew) status = 'A';
          else if (f.isDeleted) status = 'D';
          else if (f.isRename) status = 'R';
          return {
            path: filePath,
            name: filePath.split('/').pop(),
            status: status,
            addedLines: f.addedLines || 0,
            deletedLines: f.deletedLines || 0
          };
        });
      } else {
        const wrappers = container.querySelectorAll('.d2h-file-wrapper');
        this.filesList = Array.from(wrappers).map(el => {
          const nameEl = el.querySelector('.d2h-file-name');
          const filePath = nameEl ? nameEl.textContent.trim() : 'unknown';
          let status = 'M';
          if (el.querySelector('.d2h-added-tag')) status = 'A';
          else if (el.querySelector('.d2h-deleted-tag')) status = 'D';
          else if (el.querySelector('.d2h-moved-tag')) status = 'R';
          const added = el.querySelectorAll('.d2h-ins').length;
          const deleted = el.querySelectorAll('.d2h-del').length;
          return {
            path: filePath,
            name: filePath.split('/').pop(),
            status: status,
            addedLines: added,
            deletedLines: deleted
          };
        });
      }
      this.buildTree();
      if (!this.activeFile && this.filesList.length > 0) {
        this.activeFile = this.filesList[0].path;
      }

      this.attachLineListeners();
      this.attachHunkExpanders();
      this.renderExistingComments();
      this.setupScrollSpy();
    },

    attachHunkExpanders() {
      const fileWrappers = document.querySelectorAll('.d2h-file-wrapper');
      fileWrappers.forEach(fileEl => {
        const fileNameEl = fileEl.querySelector('.d2h-file-name');
        if (!fileNameEl) return;
        const filePath = fileNameEl.textContent.trim();
        if (!filePath) return;

        const infoCells = fileEl.querySelectorAll('td.d2h-code-linenumber.d2h-info');
        infoCells.forEach(gutterCell => {
          const infoRow = gutterCell.closest('tr');
          if (!infoRow) return;
          const codeLineEl = infoRow.querySelector('.d2h-code-line');
          if (!codeLineEl) return;
          const headerText = codeLineEl.textContent.trim();
          const match = headerText.match(/^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$/);
          if (!match) return;

          let oldStart = parseInt(match[1], 10);
          let oldCount = match[2] !== undefined ? parseInt(match[2], 10) : 1;
          let newStart = parseInt(match[3], 10);
          let newCount = match[4] !== undefined ? parseInt(match[4], 10) : 1;
          const suffix = match[5] || '';

          if (newStart <= 1) return;

          const btn = document.createElement('button');
          btn.type = 'button';
          btn.className = 'hunk-expand-btn';
          btn.title = 'Expand hidden context lines';
          btn.textContent = '↕ Expand';
          gutterCell.innerHTML = '';
          gutterCell.appendChild(btn);

          btn.addEventListener('click', async (e) => {
            e.stopPropagation();
            let minNew = 1;
            const prevRow = infoRow.previousElementSibling;
            if (prevRow) {
              const prevNum2 = prevRow.querySelector('.line-num2');
              if (prevNum2 && prevNum2.textContent.trim()) {
                const parsed = parseInt(prevNum2.textContent.trim(), 10);
                if (!isNaN(parsed)) minNew = parsed + 1;
              }
            }

            const endNew = newStart - 1;
            if (endNew < minNew) {
              infoRow.remove();
              return;
            }

            const step = Math.min(20, endNew - minNew + 1);
            const startNew = endNew - step + 1;
            const startOld = Math.max(1, oldStart - step);

            try {
              const res = await fetch(`/api/context?file=${encodeURIComponent(filePath)}&start=${startNew}&end=${endNew}`);
              if (!res.ok) return;
              const data = await res.json();
              const lines = data.context || [];
              if (!lines.length) return;

              const fragment = document.createDocumentFragment();
              lines.forEach((item, idx) => {
                const tr = document.createElement('tr');
                tr.className = 'd2h-cntx-expanded';
                const oldNum = startOld + idx;
                const newNum = item.lineNumber;
                tr.innerHTML = `
                  <td class="d2h-code-linenumber d2h-cntx" title="Click to add a review comment" style="cursor: pointer;">
                    <div class="line-num1">${oldNum}</div>
                    <div class="line-num2">${newNum}</div>
                  </td>
                  <td class="d2h-cntx">
                    <div class="d2h-code-line">
                      <span class="d2h-code-line-prefix">&nbsp;</span>
                      <span class="d2h-code-line-ctn">${escapeHtml(item.content)}</span>
                    </div>
                  </td>
                `;
                const lineCell = tr.querySelector('.d2h-code-linenumber');
                lineCell.addEventListener('click', (ev) => {
                  ev.stopPropagation();
                  this.showCommentInput(filePath, newNum, tr);
                });
                fragment.appendChild(tr);
              });

              infoRow.parentNode.insertBefore(fragment, infoRow.nextSibling);

              newStart = startNew;
              oldStart = startOld;
              newCount += lines.length;
              oldCount += lines.length;

              if (startNew <= minNew) {
                infoRow.remove();
              } else {
                codeLineEl.textContent = `@@ -${oldStart},${oldCount} +${newStart},${newCount} @@${suffix}`;
              }
            } catch (err) {
              console.error('Error expanding hunk:', err);
            }
          });
        });
      });
    },

    attachLineListeners() {
      const fileWrappers = document.querySelectorAll('.d2h-file-wrapper');
      fileWrappers.forEach(fileEl => {
        const fileNameEl = fileEl.querySelector('.d2h-file-name');
        if (!fileNameEl) return;
        const filePath = fileNameEl.textContent.trim();
        if (!filePath) return;

        fileEl.setAttribute('data-file-path', filePath);
        fileEl.id = 'diff-file-' + encodeURIComponent(filePath);

        const lineNumbers = fileEl.querySelectorAll('.d2h-code-linenumber, .d2h-code-side-linenumber');
        lineNumbers.forEach(lineEl => {
          if (lineEl.classList.contains('d2h-info') ||
              lineEl.classList.contains('d2h-emptyplaceholder') ||
              lineEl.classList.contains('d2h-code-side-emptyplaceholder')) {
            return;
          }
          lineEl.title = "Click to add a review comment";
          lineEl.style.cursor = "pointer";
          lineEl.addEventListener('click', (e) => {
            e.stopPropagation();
            const lineNum = extractLineNumber(lineEl);
            if (!lineNum) return;

            const row = lineEl.closest('tr');
            if (row) {
              this.showCommentInput(filePath, lineNum, row);
            }
          });
        });
      });
    },

    showCommentInput(filePath, lineNumber, rowElement) {
      const nextSibling = rowElement.nextElementSibling;
      if (nextSibling && nextSibling.classList.contains('inline-comment-row')) {
        const textarea = nextSibling.querySelector('textarea');
        if (textarea) textarea.focus();
        return;
      }

      const currentPane = rowElement.closest('.d2h-file-side-diff');
      let spacerRow = null;
      let ro = null;

      if (currentPane) {
        const panes = currentPane.parentElement.querySelectorAll('.d2h-file-side-diff');
        const otherPane = panes[0] === currentPane ? panes[1] : panes[0];
        if (otherPane) {
          const currentTbody = rowElement.closest('tbody');
          const rowIndex = Array.from(currentTbody.children).indexOf(rowElement);
          const otherTbody = otherPane.querySelector('tbody');
          if (otherTbody) {
            const otherRow = otherTbody.children[rowIndex];
            if (otherRow) {
              spacerRow = document.createElement('tr');
              spacerRow.className = 'inline-comment-spacer-row';
              spacerRow.innerHTML = `<td colspan="${otherRow.children.length || 2}" class="inline-comment-spacer"></td>`;
              otherRow.parentNode.insertBefore(spacerRow, otherRow.nextSibling);
            }
          }
        }
      }

      const commentRow = document.createElement('tr');
      commentRow.className = 'inline-comment-row';
      const colSpan = rowElement.children.length || 2;

      commentRow.innerHTML = `
        <td colspan="${colSpan}">
          <div class="inline-comment-box">
            <div class="comment-box-header">
              <span><strong>Add Comment</strong> on <code>${escapeHtml(filePath)}:${lineNumber}</code></span>
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

      if (spacerRow) {
        ro = new ResizeObserver(() => {
          if (spacerRow && spacerRow.isConnected && commentRow.isConnected) {
            spacerRow.style.height = `${commentRow.offsetHeight}px`;
          }
        });
        ro.observe(commentRow);
      }

      const textarea = commentRow.querySelector('textarea');
      const saveBtn = commentRow.querySelector('.btn-save');
      const cancelBtn = commentRow.querySelector('.btn-cancel');

      textarea.focus();

      const cleanup = () => {
        if (ro) ro.disconnect();
        if (spacerRow) spacerRow.remove();
        commentRow.remove();
      };

      const save = async () => {
        const text = textarea.value.trim();
        if (!text) return;
        await this.addComment(filePath, lineNumber, text);
        cleanup();
        this.renderExistingComments();
      };

      saveBtn.addEventListener('click', save);
      cancelBtn.addEventListener('click', cleanup);
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
      document.querySelectorAll('.rendered-comment-row, .inline-comment-spacer-row').forEach(el => el.remove());

      const fileWrappers = document.querySelectorAll('.d2h-file-wrapper');
      fileWrappers.forEach(fileEl => {
        const fileNameEl = fileEl.querySelector('.d2h-file-name');
        if (!fileNameEl) return;
        const filePath = fileNameEl.textContent.trim();

        const fileComments = this.comments.filter(c => c.filePath === filePath);
        fileComments.forEach(comment => {
          let targetRow = null;

          const sideDiffs = fileEl.querySelectorAll('.d2h-file-side-diff');
          if (sideDiffs.length >= 2) {
            const rightPane = sideDiffs[1];
            const leftPane = sideDiffs[0];

            for (const pane of [rightPane, leftPane]) {
              const lines = pane.querySelectorAll('.d2h-code-side-linenumber');
              for (const el of lines) {
                if (extractLineNumber(el) === comment.line) {
                  targetRow = el.closest('tr');
                  break;
                }
              }
              if (targetRow) break;
            }
          } else {
            const lines = fileEl.querySelectorAll('.d2h-code-linenumber');
            for (const el of lines) {
              if (extractLineNumber(el) === comment.line) {
                targetRow = el.closest('tr');
                break;
              }
            }
          }

          if (targetRow) {
            this.insertCommentCard(targetRow, comment);
          }
        });
      });
    },

    insertCommentCard(rowElement, comment) {
      const currentPane = rowElement.closest('.d2h-file-side-diff');
      let spacerRow = null;
      let ro = null;

      if (currentPane) {
        const panes = currentPane.parentElement.querySelectorAll('.d2h-file-side-diff');
        const otherPane = panes[0] === currentPane ? panes[1] : panes[0];
        if (otherPane) {
          const currentTbody = rowElement.closest('tbody');
          const rowIndex = Array.from(currentTbody.children).indexOf(rowElement);
          const otherTbody = otherPane.querySelector('tbody');
          if (otherTbody) {
            const otherRow = otherTbody.children[rowIndex];
            if (otherRow) {
              spacerRow = document.createElement('tr');
              spacerRow.className = 'inline-comment-spacer-row';
              spacerRow.innerHTML = `<td colspan="${otherRow.children.length || 2}" class="inline-comment-spacer"></td>`;
              otherRow.parentNode.insertBefore(spacerRow, otherRow.nextSibling);
            }
          }
        }
      }

      const commentRow = document.createElement('tr');
      commentRow.className = 'rendered-comment-row';
      const colSpan = rowElement.children.length || 2;

      commentRow.innerHTML = `
        <td colspan="${colSpan}">
          <div class="existing-comment-card">
            <div class="existing-comment-header">
              <span><strong>Line ${comment.line}</strong></span>
              <button class="btn btn-danger" style="padding: 2px 8px; font-size: 0.75rem;" data-id="${escapeHtml(comment.id)}">Delete</button>
            </div>
            <div class="existing-comment-body">${escapeHtml(comment.text)}</div>
          </div>
        </td>
      `;

      rowElement.parentNode.insertBefore(commentRow, rowElement.nextSibling);

      if (spacerRow) {
        ro = new ResizeObserver(() => {
          if (spacerRow && spacerRow.isConnected && commentRow.isConnected) {
            spacerRow.style.height = `${commentRow.offsetHeight}px`;
          }
        });
        ro.observe(commentRow);
      }

      const delBtn = commentRow.querySelector('button');
      if (delBtn) {
        delBtn.addEventListener('click', async () => {
          if (ro) ro.disconnect();
          if (spacerRow) spacerRow.remove();
          commentRow.remove();
          await this.deleteComment(comment.id);
        });
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
        const text = data.prompt || '';

        let copied = false;
        if (navigator.clipboard && navigator.clipboard.writeText) {
          try {
            await navigator.clipboard.writeText(text);
            copied = true;
          } catch (_) {}
        }

        if (!copied) {
          const ta = document.createElement('textarea');
          ta.value = text;
          ta.style.position = 'fixed';
          ta.style.opacity = '0';
          document.body.appendChild(ta);
          ta.focus();
          ta.select();
          try {
            document.execCommand('copy');
            copied = true;
          } catch (_) {}
          ta.remove();
        }

        this.showToast('Copied formatted prompt to clipboard!');
      } catch (err) {
        console.error('Error copying prompt:', err);
        this.showToast('Failed to copy prompt');
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
        this.showToast('Saved review to .review.md in repository root!');
      } catch (err) {
        console.error('Error saving review file:', err);
      }
    }
  };
}

function extractLineNumber(lineEl) {
  if (!lineEl || lineEl.classList.contains('d2h-info') || 
      lineEl.classList.contains('d2h-emptyplaceholder') || 
      lineEl.classList.contains('d2h-code-side-emptyplaceholder')) {
    return null;
  }
  const lineNum2 = lineEl.querySelector('.line-num2');
  if (lineNum2 && lineNum2.textContent.trim()) {
    const val = parseInt(lineNum2.textContent.trim(), 10);
    if (!isNaN(val) && val > 0) return val;
  }
  const lineNum1 = lineEl.querySelector('.line-num1');
  if (lineNum1 && lineNum1.textContent.trim()) {
    const val = parseInt(lineNum1.textContent.trim(), 10);
    if (!isNaN(val) && val > 0) return val;
  }
  const clean = lineEl.textContent.replace(/[^0-9]/g, '').trim();
  const val = parseInt(clean, 10);
  return (!isNaN(val) && val > 0) ? val : null;
}

function escapeHtml(str) {
  if (!str) return '';
  return String(str).replace(/&/g, "&amp;")
                    .replace(/</g, "&lt;")
                    .replace(/>/g, "&gt;")
                    .replace(/"/g, "&quot;")
                    .replace(/'/g, "&#039;");
}

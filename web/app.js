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
    fileTotalLines: {},
    scrollObserver: null,
    hljs: null,

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
        this.fileTotalLines = data.fileLines || {};
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
        registerGenericCodeGrammar(diff2htmlUi.hljs);
        this.hljs = diff2htmlUi.hljs;
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
      this.orderDiffWrappers();
      if (!this.activeFile && this.filesList.length > 0) {
        this.activeFile = this.filesList[0].path;
      }

      this.attachLineListeners();
      this.attachHunkExpanders();
      this.renderExistingComments();
      this.setupScrollSpy();
    },

    orderDiffWrappers() {
      const d2hWrapper = document.querySelector('.d2h-wrapper') || document.getElementById('diff-viewer');
      if (!d2hWrapper) return;

      const treeFilePaths = this.treeNodes.filter(n => !n.isFolder).map(n => n.path);
      const wrappers = Array.from(d2hWrapper.querySelectorAll('.d2h-file-wrapper'));
      if (!wrappers.length) return;

      const wrapperMap = new Map();
      wrappers.forEach(w => {
        const nameEl = w.querySelector('.d2h-file-name');
        const path = nameEl ? nameEl.textContent.trim() : '';
        if (path) {
          w.setAttribute('data-file-path', path);
          w.id = 'diff-file-' + encodeURIComponent(path);
          wrapperMap.set(path, w);
        }
      });

      const parent = wrappers[0]?.parentNode;
      if (!parent) return;

      treeFilePaths.forEach(path => {
        const w = wrapperMap.get(path);
        if (w) {
          parent.appendChild(w);
        }
      });

      const fileMap = new Map(this.filesList.map(f => [f.path, f]));
      this.filesList = treeFilePaths.map(p => fileMap.get(p)).filter(Boolean);
    },

    async getFileTotalLines(filePath) {
      if (this.fileTotalLines && typeof this.fileTotalLines[filePath] === 'number') {
        return this.fileTotalLines[filePath];
      }
      try {
        const res = await fetch(`/api/context?file=${encodeURIComponent(filePath)}`);
        if (res.ok) {
          const data = await res.json();
          const count = data.totalLines || 0;
          this.fileTotalLines = this.fileTotalLines || {};
          this.fileTotalLines[filePath] = count;
          return count;
        }
      } catch (err) {
        console.error('Error fetching file total lines:', err);
      }
      return 0;
    },

    async attachHunkExpanders() {
      const EXPAND_STEP = 6;
      const fileWrappers = document.querySelectorAll('.d2h-file-wrapper');

      for (const fileEl of Array.from(fileWrappers)) {
        const fileNameEl = fileEl.querySelector('.d2h-file-name');
        if (!fileNameEl) continue;
        const filePath = fileNameEl.textContent.trim();
        if (!filePath) continue;

        const isSplit = !!fileEl.querySelector('.d2h-files-diff');
        const totalLines = await this.getFileTotalLines(filePath);

        if (!isSplit) {
          this.attachUnifiedHunkExpanders(fileEl, filePath, totalLines, EXPAND_STEP);
        } else {
          this.attachSplitHunkExpanders(fileEl, filePath, totalLines, EXPAND_STEP);
        }
      }
    },

    attachUnifiedHunkExpanders(fileEl, filePath, totalLines, step) {
      const tbody = fileEl.querySelector('table.d2h-diff-table tbody');
      if (!tbody) return;

      const hunkRows = Array.from(tbody.querySelectorAll('tr')).filter(r => r.querySelector('td.d2h-code-linenumber.d2h-info'));

      hunkRows.forEach((infoRow) => {
        const gutterCell = infoRow.querySelector('td.d2h-code-linenumber.d2h-info');
        const codeLineEl = infoRow.querySelector('.d2h-code-line');
        if (!gutterCell || !codeLineEl) return;

        const headerText = codeLineEl.textContent.trim();
        const match = headerText.match(/^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$/);
        if (!match) return;

        let oldStart = parseInt(match[1], 10);
        let oldCount = match[2] !== undefined ? parseInt(match[2], 10) : 1;
        let newStart = parseInt(match[3], 10);
        let newCount = match[4] !== undefined ? parseInt(match[4], 10) : 1;
        const suffix = match[5] || '';

        let prevNew = 0;
        let prevOld = 0;
        let hasAboveHunk = false;

        const prevRow = infoRow.previousElementSibling;
        if (prevRow) {
          const num2El = prevRow.querySelector('.line-num2');
          const num1El = prevRow.querySelector('.line-num1');
          if (num2El && num2El.textContent.trim()) {
            const p2 = parseInt(num2El.textContent.trim(), 10);
            if (!isNaN(p2)) {
              prevNew = p2;
              hasAboveHunk = true;
            }
          }
          if (num1El && num1El.textContent.trim()) {
            const p1 = parseInt(num1El.textContent.trim(), 10);
            if (!isNaN(p1)) prevOld = p1;
          }
        }

        let gapStartNew = hasAboveHunk ? prevNew + 1 : 1;
        let gapEndNew = newStart - 1;
        let gapStartOld = hasAboveHunk ? (prevOld > 0 ? prevOld + 1 : 1) : 1;
        let gapEndOld = oldStart - 1;

        if (gapStartNew > gapEndNew) return;

        gutterCell.innerHTML = '';
        const actionsContainer = document.createElement('div');
        actionsContainer.className = 'hunk-expand-actions';

        let btnUp = null;
        let btnDown = null;

        if (hasAboveHunk) {
          btnUp = document.createElement('button');
          btnUp.type = 'button';
          btnUp.className = 'hunk-expand-btn expand-up';
          btnUp.title = 'Expand up (5-7 lines)';
          btnUp.textContent = '▲';

          btnDown = document.createElement('button');
          btnDown.type = 'button';
          btnDown.className = 'hunk-expand-btn expand-down';
          btnDown.title = 'Expand down (5-7 lines)';
          btnDown.textContent = '▼';

          actionsContainer.appendChild(btnUp);
          actionsContainer.appendChild(btnDown);
        } else {
          btnUp = document.createElement('button');
          btnUp.type = 'button';
          btnUp.className = 'hunk-expand-btn expand-single expand-up';
          btnUp.title = 'Expand up (5-7 lines)';
          btnUp.textContent = '▲';
          actionsContainer.appendChild(btnUp);
        }
        gutterCell.appendChild(actionsContainer);

        const expandUp = async () => {
          if (gapStartNew > gapEndNew) return;
          const count = Math.min(step, gapEndNew - gapStartNew + 1);
          const startNew = gapEndNew - count + 1;
          const endNew = gapEndNew;
          const startOld = Math.max(1, gapEndOld - count + 1);

          try {
            const res = await fetch(`/api/context?file=${encodeURIComponent(filePath)}&start=${startNew}&end=${endNew}`);
            if (!res.ok) return;
            const data = await res.json();
            const lines = data.context || [];
            if (!lines.length) return;

            const fragment = document.createDocumentFragment();
            lines.forEach((item, idx) => {
              const oldNum = startOld + idx;
              const newNum = item.lineNumber;
              const tr = this.createUnifiedContextRow(filePath, oldNum, newNum, item.content);
              fragment.appendChild(tr);
            });

            infoRow.parentNode.insertBefore(fragment, infoRow.nextSibling);

            gapEndNew = startNew - 1;
            gapEndOld = startOld - 1;
            newStart = startNew;
            newCount += lines.length;
            oldStart = startOld;
            oldCount += lines.length;

            if (gapStartNew > gapEndNew) {
              infoRow.remove();
            } else {
              codeLineEl.textContent = `@@ -${oldStart},${oldCount} +${newStart},${newCount} @@${suffix}`;
            }
            this.renderExistingComments();
          } catch (err) {
            console.error('Error expanding up:', err);
          }
        };

        const expandDown = async () => {
          if (gapStartNew > gapEndNew) return;
          const count = Math.min(step, gapEndNew - gapStartNew + 1);
          const startNew = gapStartNew;
          const endNew = gapStartNew + count - 1;
          const startOld = gapStartOld;

          try {
            const res = await fetch(`/api/context?file=${encodeURIComponent(filePath)}&start=${startNew}&end=${endNew}`);
            if (!res.ok) return;
            const data = await res.json();
            const lines = data.context || [];
            if (!lines.length) return;

            const fragment = document.createDocumentFragment();
            lines.forEach((item, idx) => {
              const oldNum = startOld + idx;
              const newNum = item.lineNumber;
              const tr = this.createUnifiedContextRow(filePath, oldNum, newNum, item.content);
              fragment.appendChild(tr);
            });

            infoRow.parentNode.insertBefore(fragment, infoRow);

            gapStartNew = endNew + 1;
            gapStartOld = startOld + lines.length;

            if (gapStartNew > gapEndNew) {
              infoRow.remove();
            } else {
              codeLineEl.textContent = `@@ -${oldStart},${oldCount} +${newStart},${newCount} @@${suffix}`;
            }
            this.renderExistingComments();
          } catch (err) {
            console.error('Error expanding down:', err);
          }
        };

        if (btnUp) {
          btnUp.addEventListener('click', (e) => {
            e.stopPropagation();
            expandUp();
          });
        }
        if (btnDown) {
          btnDown.addEventListener('click', (e) => {
            e.stopPropagation();
            expandDown();
          });
        }
      });

      if (totalLines > 0) {
        const rows = Array.from(tbody.querySelectorAll('tr')).filter(r => !r.classList.contains('inline-comment-row') && !r.classList.contains('inline-comment-spacer-row') && !r.classList.contains('d2h-bottom-expander-row'));
        const lastRow = rows[rows.length - 1];
        if (!lastRow) return;

        let lastNew = 0;
        let lastOld = 0;
        const num2El = lastRow.querySelector('.line-num2');
        const num1El = lastRow.querySelector('.line-num1');
        if (num2El && num2El.textContent.trim()) {
          const parsed = parseInt(num2El.textContent.trim(), 10);
          if (!isNaN(parsed)) lastNew = parsed;
        }
        if (num1El && num1El.textContent.trim()) {
          const parsed = parseInt(num1El.textContent.trim(), 10);
          if (!isNaN(parsed)) lastOld = parsed;
        }

        if (lastNew > 0 && totalLines > lastNew) {
          let bottomStartNew = lastNew + 1;
          let bottomEndNew = totalLines;
          let bottomStartOld = lastOld > 0 ? lastOld + 1 : bottomStartNew;

          const bottomRow = document.createElement('tr');
          bottomRow.className = 'd2h-bottom-expander-row';
          bottomRow.innerHTML = `
            <td class="d2h-code-linenumber d2h-info">
              <div class="hunk-expand-actions">
                <button type="button" class="hunk-expand-btn expand-single expand-down" title="Expand down (5-7 lines)">▼</button>
              </div>
            </td>
            <td class="d2h-info">
              <div class="d2h-code-line">...</div>
            </td>
          `;

          const btnBottom = bottomRow.querySelector('.hunk-expand-btn');
          btnBottom.addEventListener('click', async (e) => {
            e.stopPropagation();
            if (bottomStartNew > bottomEndNew) return;
            const count = Math.min(step, bottomEndNew - bottomStartNew + 1);
            const startNew = bottomStartNew;
            const endNew = bottomStartNew + count - 1;
            const startOld = bottomStartOld;

            try {
              const res = await fetch(`/api/context?file=${encodeURIComponent(filePath)}&start=${startNew}&end=${endNew}`);
              if (!res.ok) return;
              const data = await res.json();
              const lines = data.context || [];
              if (!lines.length) return;

              const fragment = document.createDocumentFragment();
              lines.forEach((item, idx) => {
                const oldNum = startOld + idx;
                const newNum = item.lineNumber;
                const tr = this.createUnifiedContextRow(filePath, oldNum, newNum, item.content);
                fragment.appendChild(tr);
              });

              bottomRow.parentNode.insertBefore(fragment, bottomRow);

              bottomStartNew = endNew + 1;
              bottomStartOld = startOld + lines.length;

              if (bottomStartNew > bottomEndNew) {
                bottomRow.remove();
              }
              this.renderExistingComments();
            } catch (err) {
              console.error('Error expanding bottom:', err);
            }
          });

          tbody.appendChild(bottomRow);
        }
      }
    },

    attachSplitHunkExpanders(fileEl, filePath, totalLines, step) {
      const sideDiffs = fileEl.querySelectorAll('.d2h-file-side-diff');
      if (sideDiffs.length < 2) return;
      const leftTbody = sideDiffs[0].querySelector('tbody');
      const rightTbody = sideDiffs[1].querySelector('tbody');
      if (!leftTbody || !rightTbody) return;

      const rightHunkRows = Array.from(rightTbody.querySelectorAll('tr')).filter(r => r.querySelector('td.d2h-code-side-linenumber.d2h-info'));

      rightHunkRows.forEach((rightInfoRow) => {
        const rightGutter = rightInfoRow.querySelector('td.d2h-code-side-linenumber.d2h-info');
        const rowIndex = Array.from(rightTbody.children).indexOf(rightInfoRow);
        const leftInfoRow = leftTbody.children[rowIndex];
        if (!rightGutter || !leftInfoRow) return;

        const leftCodeLine = leftInfoRow.querySelector('.d2h-code-side-line');
        const headerText = leftCodeLine ? leftCodeLine.textContent.trim() : '';
        const match = headerText.match(/^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$/);
        if (!match) return;

        let oldStart = parseInt(match[1], 10);
        let oldCount = match[2] !== undefined ? parseInt(match[2], 10) : 1;
        let newStart = parseInt(match[3], 10);
        let newCount = match[4] !== undefined ? parseInt(match[4], 10) : 1;
        const suffix = match[5] || '';

        let prevNew = 0;
        let prevOld = 0;
        let hasAboveHunk = false;

        const prevRightRow = rightInfoRow.previousElementSibling;
        const prevLeftRow = leftInfoRow.previousElementSibling;
        if (prevRightRow) {
          const num2El = prevRightRow.querySelector('.d2h-code-side-linenumber');
          if (num2El && num2El.textContent.trim()) {
            const p2 = parseInt(num2El.textContent.trim(), 10);
            if (!isNaN(p2)) {
              prevNew = p2;
              hasAboveHunk = true;
            }
          }
        }
        if (prevLeftRow) {
          const num1El = prevLeftRow.querySelector('.d2h-code-side-linenumber');
          if (num1El && num1El.textContent.trim()) {
            const p1 = parseInt(num1El.textContent.trim(), 10);
            if (!isNaN(p1)) prevOld = p1;
          }
        }

        let gapStartNew = hasAboveHunk ? prevNew + 1 : 1;
        let gapEndNew = newStart - 1;
        let gapStartOld = hasAboveHunk ? (prevOld > 0 ? prevOld + 1 : 1) : 1;
        let gapEndOld = oldStart - 1;

        if (gapStartNew > gapEndNew) return;

        rightGutter.innerHTML = '';
        const actionsContainer = document.createElement('div');
        actionsContainer.className = 'hunk-expand-actions';

        let btnUp = null;
        let btnDown = null;

        if (hasAboveHunk) {
          btnUp = document.createElement('button');
          btnUp.type = 'button';
          btnUp.className = 'hunk-expand-btn expand-up';
          btnUp.title = 'Expand up (5-7 lines)';
          btnUp.textContent = '▲';

          btnDown = document.createElement('button');
          btnDown.type = 'button';
          btnDown.className = 'hunk-expand-btn expand-down';
          btnDown.title = 'Expand down (5-7 lines)';
          btnDown.textContent = '▼';

          actionsContainer.appendChild(btnUp);
          actionsContainer.appendChild(btnDown);
        } else {
          btnUp = document.createElement('button');
          btnUp.type = 'button';
          btnUp.className = 'hunk-expand-btn expand-single expand-up';
          btnUp.title = 'Expand up (5-7 lines)';
          btnUp.textContent = '▲';
          actionsContainer.appendChild(btnUp);
        }
        rightGutter.appendChild(actionsContainer);

        const expandUp = async () => {
          if (gapStartNew > gapEndNew) return;
          const count = Math.min(step, gapEndNew - gapStartNew + 1);
          const startNew = gapEndNew - count + 1;
          const endNew = gapEndNew;
          const startOld = Math.max(1, gapEndOld - count + 1);

          try {
            const res = await fetch(`/api/context?file=${encodeURIComponent(filePath)}&start=${startNew}&end=${endNew}`);
            if (!res.ok) return;
            const data = await res.json();
            const lines = data.context || [];
            if (!lines.length) return;

            const leftFrag = document.createDocumentFragment();
            const rightFrag = document.createDocumentFragment();

            lines.forEach((item, idx) => {
              const oldNum = startOld + idx;
              const newNum = item.lineNumber;
              const { leftTr, rightTr } = this.createSplitContextRows(filePath, oldNum, newNum, item.content);
              leftFrag.appendChild(leftTr);
              rightFrag.appendChild(rightTr);
            });

            leftInfoRow.parentNode.insertBefore(leftFrag, leftInfoRow.nextSibling);
            rightInfoRow.parentNode.insertBefore(rightFrag, rightInfoRow.nextSibling);

            gapEndNew = startNew - 1;
            gapEndOld = startOld - 1;
            newStart = startNew;
            newCount += lines.length;
            oldStart = startOld;
            oldCount += lines.length;

            if (gapStartNew > gapEndNew) {
              leftInfoRow.remove();
              rightInfoRow.remove();
            } else if (leftCodeLine) {
              leftCodeLine.textContent = `@@ -${oldStart},${oldCount} +${newStart},${newCount} @@${suffix}`;
            }
            this.renderExistingComments();
          } catch (err) {
            console.error('Error expanding up in split view:', err);
          }
        };

        const expandDown = async () => {
          if (gapStartNew > gapEndNew) return;
          const count = Math.min(step, gapEndNew - gapStartNew + 1);
          const startNew = gapStartNew;
          const endNew = gapStartNew + count - 1;
          const startOld = gapStartOld;

          try {
            const res = await fetch(`/api/context?file=${encodeURIComponent(filePath)}&start=${startNew}&end=${endNew}`);
            if (!res.ok) return;
            const data = await res.json();
            const lines = data.context || [];
            if (!lines.length) return;

            const leftFrag = document.createDocumentFragment();
            const rightFrag = document.createDocumentFragment();

            lines.forEach((item, idx) => {
              const oldNum = startOld + idx;
              const newNum = item.lineNumber;
              const { leftTr, rightTr } = this.createSplitContextRows(filePath, oldNum, newNum, item.content);
              leftFrag.appendChild(leftTr);
              rightFrag.appendChild(rightTr);
            });

            leftInfoRow.parentNode.insertBefore(leftFrag, leftInfoRow);
            rightInfoRow.parentNode.insertBefore(rightFrag, rightInfoRow);

            gapStartNew = endNew + 1;
            gapStartOld = startOld + lines.length;

            if (gapStartNew > gapEndNew) {
              leftInfoRow.remove();
              rightInfoRow.remove();
            } else if (leftCodeLine) {
              leftCodeLine.textContent = `@@ -${oldStart},${oldCount} +${newStart},${newCount} @@${suffix}`;
            }
            this.renderExistingComments();
          } catch (err) {
            console.error('Error expanding down in split view:', err);
          }
        };

        if (btnUp) {
          btnUp.addEventListener('click', (e) => {
            e.stopPropagation();
            expandUp();
          });
        }
        if (btnDown) {
          btnDown.addEventListener('click', (e) => {
            e.stopPropagation();
            expandDown();
          });
        }
      });

      if (totalLines > 0) {
        const rightRows = Array.from(rightTbody.querySelectorAll('tr')).filter(r => !r.classList.contains('inline-comment-row') && !r.classList.contains('inline-comment-spacer-row') && !r.classList.contains('d2h-bottom-expander-row'));
        const leftRows = Array.from(leftTbody.querySelectorAll('tr')).filter(r => !r.classList.contains('inline-comment-row') && !r.classList.contains('inline-comment-spacer-row') && !r.classList.contains('d2h-bottom-expander-row'));
        const lastRightRow = rightRows[rightRows.length - 1];
        const lastLeftRow = leftRows[leftRows.length - 1];
        if (!lastRightRow || !lastLeftRow) return;

        let lastNew = 0;
        let lastOld = 0;
        const num2El = lastRightRow.querySelector('.d2h-code-side-linenumber');
        const num1El = lastLeftRow.querySelector('.d2h-code-side-linenumber');
        if (num2El && num2El.textContent.trim()) {
          const parsed = parseInt(num2El.textContent.trim(), 10);
          if (!isNaN(parsed)) lastNew = parsed;
        }
        if (num1El && num1El.textContent.trim()) {
          const parsed = parseInt(num1El.textContent.trim(), 10);
          if (!isNaN(parsed)) lastOld = parsed;
        }

        if (lastNew > 0 && totalLines > lastNew) {
          let bottomStartNew = lastNew + 1;
          let bottomEndNew = totalLines;
          let bottomStartOld = lastOld > 0 ? lastOld + 1 : bottomStartNew;

          const leftBottomRow = document.createElement('tr');
          leftBottomRow.className = 'd2h-bottom-expander-row';
          leftBottomRow.innerHTML = `
            <td class="d2h-code-side-linenumber d2h-info"></td>
            <td class="d2h-info"><div class="d2h-code-side-line">&nbsp;</div></td>
          `;

          const rightBottomRow = document.createElement('tr');
          rightBottomRow.className = 'd2h-bottom-expander-row';
          rightBottomRow.innerHTML = `
            <td class="d2h-code-side-linenumber d2h-info">
              <div class="hunk-expand-actions">
                <button type="button" class="hunk-expand-btn expand-single expand-down" title="Expand down (5-7 lines)">▼</button>
              </div>
            </td>
            <td class="d2h-info">
              <div class="d2h-code-side-line">...</div>
            </td>
          `;

          const btnBottom = rightBottomRow.querySelector('.hunk-expand-btn');
          btnBottom.addEventListener('click', async (e) => {
            e.stopPropagation();
            if (bottomStartNew > bottomEndNew) return;
            const count = Math.min(step, bottomEndNew - bottomStartNew + 1);
            const startNew = bottomStartNew;
            const endNew = bottomStartNew + count - 1;
            const startOld = bottomStartOld;

            try {
              const res = await fetch(`/api/context?file=${encodeURIComponent(filePath)}&start=${startNew}&end=${endNew}`);
              if (!res.ok) return;
              const data = await res.json();
              const lines = data.context || [];
              if (!lines.length) return;

              const leftFrag = document.createDocumentFragment();
              const rightFrag = document.createDocumentFragment();

              lines.forEach((item, idx) => {
                const oldNum = startOld + idx;
                const newNum = item.lineNumber;
                const { leftTr, rightTr } = this.createSplitContextRows(filePath, oldNum, newNum, item.content);
                leftFrag.appendChild(leftTr);
                rightFrag.appendChild(rightTr);
              });

              leftBottomRow.parentNode.insertBefore(leftFrag, leftBottomRow);
              rightBottomRow.parentNode.insertBefore(rightFrag, rightBottomRow);

              bottomStartNew = endNew + 1;
              bottomStartOld = startOld + lines.length;

              if (bottomStartNew > bottomEndNew) {
                leftBottomRow.remove();
                rightBottomRow.remove();
              }
              this.renderExistingComments();
            } catch (err) {
              console.error('Error expanding bottom in split view:', err);
            }
          });

          leftTbody.appendChild(leftBottomRow);
          rightTbody.appendChild(rightBottomRow);
        }
      }
    },

    highlightCode(filePath, content) {
      if (!content) return '';
      if (!this.hljs) return escapeHtml(content);
      const ext = (filePath.split('.').pop() || '').toLowerCase();
      const lang = this.hljs.getLanguage(ext) ? ext : 'plaintext';
      try {
        const res = this.hljs.highlight(content, { language: lang, ignoreIllegals: true });
        return res.value;
      } catch (e) {
        return escapeHtml(content);
      }
    },

    createUnifiedContextRow(filePath, oldNum, newNum, content) {
      const tr = document.createElement('tr');
      tr.className = 'd2h-cntx-expanded';
      const codeHtml = this.highlightCode(filePath, content);
      tr.innerHTML = `
        <td class="d2h-code-linenumber d2h-cntx" title="Click to add a review comment" style="cursor: pointer;">
          <div class="line-num1">${oldNum > 0 ? oldNum : ''}</div>
          <div class="line-num2">${newNum > 0 ? newNum : ''}</div>
        </td>
        <td class="d2h-cntx">
          <div class="d2h-code-line">
            <span class="d2h-code-line-prefix">&nbsp;</span>
            <span class="d2h-code-line-ctn hljs">${codeHtml}</span>
          </div>
        </td>
      `;
      const lineCell = tr.querySelector('.d2h-code-linenumber');
      lineCell.addEventListener('click', (ev) => {
        ev.stopPropagation();
        this.showCommentInput(filePath, newNum, tr);
      });
      return tr;
    },

    createSplitContextRows(filePath, oldNum, newNum, content) {
      const codeHtml = this.highlightCode(filePath, content);
      const leftTr = document.createElement('tr');
      leftTr.className = 'd2h-cntx-expanded';
      leftTr.innerHTML = `
        <td class="d2h-code-side-linenumber d2h-cntx" title="Click to add a review comment" style="cursor: pointer;">${oldNum > 0 ? oldNum : ''}</td>
        <td class="d2h-cntx">
          <div class="d2h-code-side-line">
            <span class="d2h-code-line-prefix">&nbsp;</span>
            <span class="d2h-code-line-ctn hljs">${codeHtml}</span>
          </div>
        </td>
      `;

      const rightTr = document.createElement('tr');
      rightTr.className = 'd2h-cntx-expanded';
      rightTr.innerHTML = `
        <td class="d2h-code-side-linenumber d2h-cntx" title="Click to add a review comment" style="cursor: pointer;">${newNum > 0 ? newNum : ''}</td>
        <td class="d2h-cntx">
          <div class="d2h-code-side-line">
            <span class="d2h-code-line-prefix">&nbsp;</span>
            <span class="d2h-code-line-ctn hljs">${codeHtml}</span>
          </div>
        </td>
      `;

      const leftCell = leftTr.querySelector('.d2h-code-side-linenumber');
      leftCell.addEventListener('click', (ev) => {
        ev.stopPropagation();
        this.showCommentInput(filePath, newNum, leftTr);
      });

      const rightCell = rightTr.querySelector('.d2h-code-side-linenumber');
      rightCell.addEventListener('click', (ev) => {
        ev.stopPropagation();
        this.showCommentInput(filePath, newNum, rightTr);
      });

      return { leftTr, rightTr };
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

function registerGenericCodeGrammar(hljs) {
  if (!hljs || typeof hljs.registerLanguage !== 'function') return;

  function genericLanguage(hljsInstance) {
    const KEYWORDS = {
      keyword: [
        // Conditionals & branches ("ifs")
        "if", "else", "elif", "elsif", "then", "endif", "switch", "case", "select", "match", "default", "when", "guard",
        // Loops
        "for", "while", "do", "loop", "repeat", "until", "break", "continue", "in", "each", "of",
        // Functions & declarations
        "fn", "func", "function", "def", "sub", "proc", "method", "macro",
        "let", "var", "const", "mut", "pub", "priv", "package", "import", "export", "from", "as", "include", "require", "use", "extern",
        // Control flow & errors
        "return", "yield", "defer", "errdefer", "try", "catch", "finally", "throw", "raise", "panic", "assert",
        // Types & structures
        "struct", "class", "interface", "enum", "type", "union", "trait", "impl", "new", "delete",
        // Modifiers & concurrency
        "async", "await", "comptime", "inline", "volatile", "atomic",
        // Logical
        "and", "or", "not", "xor", "is", "with"
      ],
      literal: [
        "true", "false", "null", "nil", "undefined", "none", "None", "Some", "Ok", "Err", "NaN", "Infinity", "self", "this"
      ],
      type: [
        "int", "i8", "i16", "i32", "i64", "i128", "isize",
        "u8", "u16", "u32", "u64", "u128", "usize",
        "f16", "f32", "f64", "f80", "f128", "float", "double",
        "bool", "boolean", "char", "str", "string", "void", "any", "anytype", "anyerror",
        "byte", "comptime_int", "comptime_float"
      ]
    };

    return {
      name: "Generic Code",
      keywords: KEYWORDS,
      contains: [
        hljsInstance.C_LINE_COMMENT_MODE,
        hljsInstance.HASH_COMMENT_MODE,
        hljsInstance.C_BLOCK_COMMENT_MODE,
        hljsInstance.QUOTE_STRING_MODE,
        hljsInstance.APOS_STRING_MODE,
        { scope: "string", begin: "\\\\\\\\.*$" },
        hljsInstance.C_NUMBER_MODE,
        {
          scope: "title.function.invoke",
          begin: "\\b(?!(?:if|else|elif|elsif|for|while|switch|case|match|catch|return|defer|errdefer|yield)\\b)[a-zA-Z_]\\w*(?=\\s*\\()"
        },
        {
          scope: "variable.constant",
          begin: "\\b[A-Z][A-Z0-9_]{2,}\\b"
        },
        {
          scope: "type",
          begin: "\\b[A-Z][a-zA-Z0-9_]*\\b"
        },
        {
          scope: "property",
          begin: "(?<=\\.)[a-zA-Z_]\\w*\\b"
        }
      ]
    };
  }

  const commonLangs = [
    "plaintext", "generic", "zig", "odin", "v", "nim", "gleam", "mojo",
    "sol", "solidity", "proto", "vue", "svelte", "astro", "hcl", "tf"
  ];
  commonLangs.forEach(lang => {
    try {
      hljs.registerLanguage(lang, genericLanguage);
    } catch (e) {
      // Ignore if already registered
    }
  });
}

# sleeper — A "look busy" TUI for macOS

## Context

工具用途：當你需要離開螢幕但不想被察覺 AFK 時，在螢幕上播放看起來很真實的「正在工作」畫面。同時防止螢幕鎖定/睡眠。如果有人靠近，可以一鍵把假裝編輯的檔案用真的 `$EDITOR` 開起來無縫接管。

**實際行為**：
- 子程序跑 `caffeinate -dimsu` 防鎖屏，程式退出時自動殺掉
- 隨機在 fake-vim 介面打開使用者指定項目 (`--project ~/work/repo`) 的真實檔案，動畫式假打字
- 旁邊或切換出 fake-shell 跑「真的但無害」的 read-only 指令 (`ls`, `git status`, `git log`...)
- 旁邊或切換出 fake-AI chat 演示 debug/feature/refactor 對話（模板填入真實檔名/函式名）
- 熱鍵立即用 `$EDITOR` 打開當前顯示中的真實檔案，TUI 暫停、`$EDITOR` 退出後恢復
- 「panic key」：立刻切到最不顯眼的場景（shell 顯示 `git status`）再優雅退出

**技術選擇**（已和使用者確認）：
- Go 1.22+ + Bubble Tea v1（單一 binary、Charm 生態做動畫最強）
- macOS only（caffeinate / iTerm2 + Terminal.app）
- 項目路徑透過 CLI flag 傳入，預設 `cwd`
- AI 對話用 `//go:embed` 內建模板 + 隨機填入真實 symbol，不呼叫外部 API

## Architecture

```
sleeper/
├─ cmd/sleeper/
│   └─ main.go              # cobra-style flags、signal handler、program.Run + panic recovery
├─ internal/
│   ├─ caffeinate/          # exec.Command 包裝，Setpgid=true，process-group kill
│   ├─ scanner/             # git ls-files -z 優先，filepath.Walk fallback；隱私黑名單
│   ├─ fakevim/             # bubbletea Model：status line、行號、chroma terminal256 高亮
│   ├─ fakeshell/           # bubbletea Model：viewport scrollback；安全指令 allowlist + 2s timeout
│   ├─ fakeai/              # bubbletea Model：chat bubbles；模板渲染器（embed FS）
│   ├─ scene/               # 上層 Model：lipgloss.JoinHorizontal 切版、tea.WindowSizeMsg 路由
│   └─ handover/            # tea.ExecProcess($EDITOR file) 暫停/恢復 TUI
├─ assets/
│   └─ ai_templates/        # *.tmpl debug/feature/refactor 對話腳本（embed）
├─ go.mod
└─ README.md
```

**關鍵設計決定**（來自 Plan agent 驗證）：
- **Per-pane tick，不要 global frame**：每個 pane 自己 `Update` 回傳 `tea.Tick(jitter, ...)` 維持自己的節奏；跨 pane 協調用 `PaneEventMsg`。漂移是好事，看起來更像真人。
- **Chroma 用 `terminal256`**，不要 `terminal16m`（避免和 lipgloss/termenv 重複塗色 + 寬度計算錯誤）。
- **Caffeinate**：`SysProcAttr{Setpgid: true}` + main goroutine `signal.Notify(SIGINT/SIGTERM/SIGHUP)` 同時呼叫 `program.Quit()` 和 `syscall.Kill(-pgid, SIGTERM)`。`defer cmd.Process.Kill()` 兜底。
- **`tea.ExecProcess` 注意事項**：自己建 `*exec.Cmd` 並設定 `Stdin/Stdout/Stderr = os.Stdin/...`、callback 裡發 `tea.ClearScreen` + 自訂 `RedrawMsg` 避免 iTerm2 殘影、caffeinate 不受影響（不在同一 controlling TTY）。
- **Split layout**：先算 child 寬度（扣掉 border/padding 內邊距），再 `child.Update(tea.WindowSizeMsg)` 傳內部寬度給 child；viewport 高度要扣掉 pane 內的 status line。
- **Panic recovery**：`defer program.ReleaseTerminal()` + recover，避免 crash 留 raw mode。
- **隱私黑名單**：scanner 排除 `.env*`、`*.pem`、`id_rsa*`、binary（`http.DetectContentType` 判定）、>200KB 大檔。

## Modules — 詳細職責

### `cmd/sleeper/main.go`
- Flag 定義（用標準 `flag` 即可，免引 cobra 依賴）：
  - `--project <path>` 預設 `cwd`
  - `--scene <vim|shell|ai|vim+shell|vim+ai>` 預設 `vim+shell`
  - `--tick <duration>` 預設 `150ms`
  - `--no-caffeinate` debug 用
  - `--editor <cmd>` 覆蓋 `$EDITOR`
  - `--seed <int>` deterministic 測試
  - `--ai-style <debug|feature|refactor|mixed>` 預設 `mixed`
- 啟動 caffeinate（除非 `--no-caffeinate`）
- 啟動 signal handler goroutine
- `tea.NewProgram(rootModel, tea.WithAltScreen(), tea.WithMouseCellMotion())`
- `defer` 清理：caffeinate kill、`program.ReleaseTerminal()`
- panic recover：`ReleaseTerminal()` 後 re-panic

### `internal/caffeinate/`
```go
type Manager struct { cmd *exec.Cmd; pgid int }
func Start(ctx context.Context) (*Manager, error)  // exec "caffeinate -dimsu", Setpgid=true
func (m *Manager) Stop() error                      // syscall.Kill(-pgid, SIGTERM); fallback Kill
```

### `internal/scanner/`
```go
type File struct { Path string; Lang string; Size int64 }
type Scanner struct { root string; files []File; rng *rand.Rand }
func New(root string, seed int64) (*Scanner, error)
func (s *Scanner) Pick() File                       // weighted by ext (.go/.py/.ts > .md/.json)
func (s *Scanner) Symbols(f File) []string          // grep func/class names for AI templates
```
- 偵測 `.git/` → 用 `git ls-files -z` (PIPE 讀取)；否則 `filepath.WalkDir` 加 ignore 清單（`.git`、`node_modules`、`vendor`、`dist`、`build`、`.venv`）
- 黑名單比對檔名 glob 後再讀首 512 bytes 用 `http.DetectContentType` 排除 binary

### `internal/fakevim/`
```go
type Model struct {
    file scanner.File
    lines []string
    cursor struct{ row, col int }
    typingBuf []rune
    state vimState  // normal | insert | command
    rng *rand.Rand
}
func New(f scanner.File, w, h int) Model
func (m Model) Update(tea.Msg) (Model, tea.Cmd)     // 自己的 tea.Tick(jitter)
func (m Model) View() string                         // chroma terminal256 + lipgloss border
```
- 動畫狀態機：選一段 line range → cursor 移動 → 進入 insert mode → 一個個字打出來 → ESC → `:w` 假裝存（但**永遠不真存檔**）→ 切下一檔
- jitter：每次 tick 隨機 50–250ms，模擬真人

### `internal/fakeshell/`
```go
var SafeCommands = []SafeCmd{
    {"ls", []string{"-la"}},
    {"git", []string{"status"}},
    {"git", []string{"log", "--oneline", "-20"}},
    {"wc", []string{"-l"}},  // 加當前檔
    {"head", []string{"-30"}},
    {"find", []string{".", "-name", "*.go", "-type", "f"}},
}
type Model struct { vp viewport.Model; rng *rand.Rand; cwd string }
func (m Model) Update(tea.Msg) (Model, tea.Cmd)     // tea.Tick 後挑一個 SafeCmd 跑
```
- 強制 argv[0] 比對 + `exec.LookPath` + `context.WithTimeout(2*time.Second)`
- 永不從模板字串拼指令；永不 `sh -c`
- 偶爾插入純假輸出（`Running tests... ✓ 142 passed`）增加多樣性

### `internal/fakeai/`
```go
type Template struct { Style string; Steps []ChatStep }
type ChatStep struct { Role string; Body string }  // body 含 {{.File}} {{.Func}} 佔位符
//go:embed assets/ai_templates/*.tmpl
var templatesFS embed.FS
type Model struct { messages []ChatStep; rng *rand.Rand; symbols []string; vp viewport.Model }
```
- 啟動讀模板 → 隨機選一段 → text/template 填入 scanner 給的 file/symbol → 一字一字打出來再加下一則
- 每則「ai 訊息」之間 1.5–4s 模擬思考

### `internal/scene/`
```go
type Layout int  // VimOnly | ShellOnly | AIOnly | VimShell | VimAI
type Model struct {
    layout Layout
    vim    fakevim.Model
    shell  fakeshell.Model
    ai     fakeai.Model
    width, height int
    paused bool
}
func (m Model) Update(tea.Msg) (tea.Model, tea.Cmd)  // 路由 + 熱鍵 + WindowSizeMsg 切版
func (m Model) View() string                          // lipgloss.JoinHorizontal/Vertical
```
- 熱鍵：
  - `Tab` 循環 layout
  - `n` 強制 vim 切下一檔
  - `space` 暫停/恢復所有動畫
  - `e` `$EDITOR` 接管當前 vim 檔（送 `handover.Msg`）
  - `?` help overlay
  - `q` 普通退出
  - `Esc Esc` **panic key**：立刻切 `ShellOnly` + 跑 `git status` + 1.5s 後 `tea.Quit`
  - `Ctrl+C` 立即硬退出（caffeinate 仍會被 defer 清掉）

### `internal/handover/`
```go
func ExecEditor(editor string, file string) tea.Cmd  // tea.ExecProcess wrapper
type RedrawMsg struct{}                              // callback 發出讓 scene 重畫
```
- 建 `*exec.Cmd` 自己接 `os.Stdin/Stdout/Stderr`
- callback 回傳 `tea.Batch(tea.ClearScreen, func() tea.Msg { return RedrawMsg{} })`

## Implementation Order

1. **Skeleton + caffeinate** — `main.go` + `caffeinate/`，無 TUI，純驗證 caffeinate 啟停 + signal 處理
2. **Scanner** — 列檔、過濾、symbol 抽取；寫 `_test.go` 用本 repo 自身 dogfood
3. **fakevim 最小可動** — 讀檔、純文字渲染、固定 jitter typing；scene 用 VimOnly
4. **fakeshell** — viewport + safe-cmd allowlist + timeout
5. **scene split + 熱鍵** — JoinHorizontal、Tab 切換、`e` 接管 (`handover/`)
6. **fakeai** — 模板渲染、symbol 注入
7. **chroma 高亮** — 加進 fakevim
8. **panic key + polish** — `Esc Esc`、help overlay、panic recovery、ReleaseTerminal
9. **README + `go install` 路徑驗證**

## Critical Files (to be created)
- `cmd/sleeper/main.go`
- `internal/caffeinate/caffeinate.go` + `_test.go`
- `internal/scanner/scanner.go` + `_test.go`
- `internal/fakevim/model.go`
- `internal/fakeshell/model.go` + `safe_commands.go`
- `internal/fakeai/model.go` + `templates.go`
- `internal/scene/scene.go`
- `internal/handover/handover.go`
- `assets/ai_templates/debug.tmpl` `feature.tmpl` `refactor.tmpl`
- `go.mod` + `README.md`

## Reused / External Deps
- `github.com/charmbracelet/bubbletea` — TUI runtime
- `github.com/charmbracelet/bubbles/viewport` — scrollback for shell/ai
- `github.com/charmbracelet/lipgloss` — borders / split layout
- `github.com/alecthomas/chroma/v2` + `formatters/terminal256` — vim 語法高亮
- 標準庫：`flag`, `os/exec`, `os/signal`, `syscall`, `embed`, `text/template`, `context`, `math/rand/v2`

## Verification

**手動驗證**：
1. `go install ./cmd/sleeper` → `sleeper --project .` 在本 repo 跑起來
2. 看 vim 跑、看 shell 跑 `git status`，按 Tab 切版面
3. 按 `e` → 確認真的 `$EDITOR` 開啟、退出後 TUI 完整恢復、caffeinate 仍在跑（`pgrep caffeinate`）
4. 按 `Esc Esc` → 立刻切 shell scene 後退出
5. `Ctrl+C` → 確認 `pgrep caffeinate` 為空
6. 螢幕保護程式設 1 分鐘，跑 5 分鐘不動鍵盤滑鼠 → 不該鎖屏
7. `--no-caffeinate` → caffeinate 不啟動、其他功能正常
8. `--project ~/somewhere/with-secrets` 含 `.env` → 確認 fake-vim 從不顯示這檔

**自動測試**：
- `caffeinate_test.go`：start/stop、process group kill 驗證
- `scanner_test.go`：本 repo dogfood、確認排除 `.git`、binary 偵測、隱私黑名單
- `fakeshell/safe_commands_test.go`：allowlist 拒絕未知指令、timeout 行為

**安全自審**：
- grep `os.Create|os.WriteFile|ioutil.WriteFile` 確認沒有任何寫入路徑（fake-vim 永遠不存檔）
- grep `sh -c|/bin/sh` 確認沒有 shell 拼接

---

# Round 2 — Bug Fixes (2026-04-28)

## Context

兩個顯眼問題：
1. **Top border 經常被擠掉** — fakevim 進入 INSERT 模式後，pane 頂端的 `╭───╮` 邊框消失；vim+shell 兩 pane 都受影響。
2. **隨機編輯把代碼插在原本正常代碼中間，formatting 完全壞掉** — 例如 `package log.Printf("debug: %+v\n", v)return fmt.Errorf("foo: %w", err)main` 出現在螢幕上。看就知道是假的。

使用者問是否要用 LSP 解決位置問題。**結論：不用**。LSP 需要為每個語言類型 spawn 一個 language server (gopls/pylsp/...)，處理 async response 的生命週期，啟動成本和維護負擔對「假裝編輯」這個玩具完全不划算。Heuristic + 「永遠在行尾換行後插入」就能拿到 90% 的逼真度。

另外使用者要求把先前 RAM 爆炸相關的 bug 記錄到 `pitfalls/`，當作將來避坑用。

## Root cause 分析

### Bug 1 — Top border eaten
`internal/fakevim/model.go` 的 `truncatePlain` 用 `len(s)`（byte count）截斷每行內容到 `bodyW`：
```go
func truncatePlain(s string, w int) string {
    if len(s) > w { return s[:w] }
    ...
}
```
但 Go 檔案用 **TAB** 縮排。一個 `\t` byte 在大多數 terminal 顯示為 8 格（或 lipgloss 視為 1 格 — 不一致）。所以一行 byte 長 60、實際顯示 100 格的內容被「截斷到 60 byte」後，丟給 chroma，再丟給 lipgloss，lipgloss 用 `cellbuf.Wrap(str, width-padding, "")` 偵測到視覺寬度超出，**自動換行**。每個被換的 row 多 1 行，`scene.paneStyle.Height(h-2)` 是 **minimum** 不是 max — 內容超量就直接撐高 pane，最終整個 view 比 terminal 還高，top border 被推出視野。

### Bug 2 — Insertion 破壞 formatting
`fakevim.stepNormal` 在隨機 row、隨機 col 直接進 INSERT 模式打多行 phrase（`"if err != nil {\n\treturn err\n}"`）。這樣會：
- 把現有的 `import (` 中間插入新代碼
- 把多行 phrase 一個一個字打進去，混入其他現有 token，產生視覺垃圾
- 完全沒有縮排匹配

## 修復方案

### Fix 1 — Tab expansion + hard cap on pane size

**File**: `internal/fakevim/model.go`

1. 加 `expandTabs(s string, tabSize int) string` helper：把 `\t` 換成空格直到下一個 `tabSize` 倍數欄位（**精確的 tab stop**，不是 naive 4-space replace）。
2. 在 View 的非 cursor 行：
   ```go
   ln := expandTabs(m.lines[row], 4)
   b.WriteString(highlightLine(m.file.Lang, truncatePlain(ln, bodyW)))
   ```
3. cursor 行（`renderCursorLine`）：傳入展開過的 line，cursor col 用 byte 不變但 visible 對齊。
4. status line 的 `gap` 計算用 `lipgloss.Width()` 已經是對的；確認沒有溢位。

**File**: `internal/scene/scene.go`

5. `paneStyle` 加 `MaxWidth(w)` 和 `MaxHeight(h)`（外框 hard cap），防止任何子 model 的失誤把 pane 撐大：
   ```go
   return lipgloss.NewStyle().
       Border(lipgloss.RoundedBorder()).
       BorderForeground(...).
       Padding(0, 1).
       Width(w-2).Height(h-2).
       MaxWidth(w).MaxHeight(h)
   ```

### Fix 2 — Insert 用 vim `o` 模式 + indent 匹配

**File**: `internal/fakevim/model.go`

1. 重寫 `stepNormal` 的 case `roll < 90`：
   - **Pick a "safe row"**：從 cursor 附近找一個「結尾乾淨」的 row（line 結尾是 `}`, `)`, `;`, 空行，或符合 `^\s*$`）。Fallback 到任意 row。
   - **跳到該 row 的行尾**（cursorCol = len(line)）
   - 進 INSERT mode，但 `insertBuf` 的內容變成：`"\n" + indent + singleLinePhrase`
2. `pickInsertPhrase` 改成只回 **單行** phrase（拆掉現有 `"if err != nil {\n\treturn err\n}"` 等多行 entry，改成 `"_ = handleErr(err)"`、`"// TODO: revisit"` 之類不破結構的 statement）。
3. 加 `detectIndent(lines []string, row int) string` — 從 row 往上找第一個非空白行，回傳它的 leading whitespace。新插入的行用一樣的縮排。
4. 把 cursor mid-line random insert 路徑（`roll < 90`）完全替換掉。原本 `roll < 60` 的「在 line 內隨機跳 col」也應該收斂，否則 cursor 會停在 token 中間看起來怪。

新分布：
```
roll <  20: row 跳動（cursor 上下移）
roll <  40: 在 line 內 word-wise 跳（不在 token 中間）
roll <  85: open 新行 + 縮排 + 單行 phrase（`o` 模式）
roll < 100: 思考停頓
```

新的 single-line `insertBank`：
```go
"go": {
    "_ = ctx.Err()",
    "// TODO: revisit boundary case",
    "log.Printf(\"trace: %+v\", payload)",
    "return fmt.Errorf(\"wrap: %w\", err)",
    "metrics.Inc(\"events.processed\")",
},
"python": {
    "logger.debug(\"trace=%s\", payload)",
    "# TODO: revisit boundary case",
    "raise ValueError(\"unexpected state\")",
},
"typescript": {
    "logger.debug({ payload });",
    "// TODO: revisit boundary case",
},
"_default": {
    "// TODO",
    "// fixme: revisit",
},
```

### Fix 3 — Pitfalls knowledge base (separate task)

使用者要求把先前的 RAM 爆炸 bug 記錄起來。建立 `pitfalls/` 目錄，每個 pitfall 一個 .md，依「症狀 / 原因 / 偵測 / 修法」結構：
- `pitfalls/urandom-readfile-oom.md` — `os.ReadFile("/dev/urandom")` 無限分配
- `pitfalls/caffeinate-u-flag-silent-exit.md` — `-u` 沒搭 `-t` 預設 5s 退出
- `pitfalls/lipgloss-truecolor-double-styling.md` — termenv 自動偵測 + chroma 重複塗色
- `pitfalls/bubbletea-quit-deadlock.md` — `prog.Quit()` 不保證即時返回，需 hard-fallback deadline
- `pitfalls/fakevim-tab-truncation.md` — byte-len 截斷 vs cell-width，tab 是地雷

也加 `TODO.md` 索引記錄已知限制（例如 LSP 不做、未來可考慮 tree-sitter）。`backlog/` 暫不建（沒有大的 design 待決定）。

這個任務獨立於 Bug 1/2，可以並行做。

## Critical files

- `internal/fakevim/model.go` — 改 View 的 tab 展開、改 stepNormal 的插入邏輯、改 pickInsertPhrase / insertBank
- `internal/scene/scene.go` — paneStyle 加 MaxWidth/MaxHeight
- `pitfalls/*.md`（new） + `TODO.md`（new）

## Verification

**手動**：
```bash
go install ./cmd/sleeper
sleeper --project ~/work/some-go-repo  # 任何含 tabs 的 Go 項目
```
- 持續看 5 分鐘，top border 應該永遠在
- 進 INSERT mode 後新代碼應該出現在「新的下一行」，縮排對齊
- 切 layout (`Tab`) 各種組合都不破

**自動**：
- 加 `internal/fakevim/expand_tabs_test.go` — table-test `expandTabs` 各種輸入
- 加 `internal/fakevim/insert_test.go` — 驗證 `detectIndent` 與「永遠不在 mid-line 插入」的 invariant

**煙霧**：
- `python3 /tmp/sleeper-pty.py 10` 跑 10 秒，RSS 應穩定 < 30MB（已經是了）

**Pitfalls**：
- `ls pitfalls/` 應該有 5 個檔
- 每個檔開頭有 symptom / cause / detect / fix 段落


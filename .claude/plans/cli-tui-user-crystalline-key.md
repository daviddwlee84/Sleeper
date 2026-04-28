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

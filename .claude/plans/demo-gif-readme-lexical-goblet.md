# Demo 動畫：方案調查 + 實際錄製 + README 嵌入

## Context

`sleeper` 是 Charmbracelet bubbletea 寫的高動態 TUI，核心賣點（多 pane 切換、fake vim 游標移動、syntax highlighting、panic key、AI chat bubble）都是「要看到才會懂」的東西。但目前 `README.md` 只有純文字描述，新使用者得 `go install` 跑起來才知道效果，這對一個「人家路過你電腦時看一秒」的工具來說落差很大。

這次要做：
1. 在 `docs/` 補一份方案調查文件，比較幾個常見的終端動畫錄製工具，讓未來想換工具或重錄時有依據（不只是「我當時隨便挑了一個」）。
2. 實際錄一支 demo（不限 GIF，只要是會動的格式都算）。
3. 嵌入 README 顯眼位置。

`docs/` 目前不存在，要新建。

## 調查到的方案 + 推薦

| 工具 | 產出格式 | 可重現？ | 適不適合 sleeper |
| --- | --- | --- | --- |
| **VHS** (charmbracelet/vhs) | GIF / MP4 / WebM | **Yes** — `.tape` 腳本進 git | **首選**：跟 bubbletea 同一家（Charmbracelet），headless 渲染所以幀數穩定，腳本可重跑 |
| asciinema + `agg` | `.cast` (JSON) → GIF | 部分 | 真實終端錄影，asciinema.org 可嵌；但 GitHub README 不會 auto-embed `.cast`，要走外部連結 |
| termtosvg | 動畫 SVG | 部分 | 體積最小、GitHub `<img>` inline 顯示；但對高頻 redraw 的 bubbletea 渲染品質不穩 |
| terminalizer | GIF / web player | Yes (`.yml`) | VHS 的舊式替代品，社群活躍度與 VHS 差距明顯 |
| svg-term-cli | 動畫 SVG（從 asciicast） | 部分 | 跟 termtosvg 類似，需先有 asciicast |
| Kap.app / QuickTime + ffmpeg | MP4 → GIF | No | 真實但純手動，每次重錄都不一樣，不適合放 repo |

**推薦：VHS** — 三個理由：
1. **同家族**：sleeper 用 bubbletea / lipgloss / chroma 都是 Charmbracelet，VHS 對這套 redraw 模式有特化（headless 終端 + 精準的幀計時），不會像螢幕錄影抓到撕裂或尺寸跳動。
2. **可重現**：腳本（`.tape`）進 git，未來改 UI 重跑一行 `vhs docs/demo.tape` 就能更新 demo，不用重新拍。
3. **GitHub 友善**：直接吐 GIF，README 用 `<img src="docs/demo.gif">` 就會 inline 動起來，不必登入或外連。

次要建議：另外提供 `asciinema rec` 指令（在文件中），讓使用者想互動式看終端時有 asciinema.org 連結可走。

## 要新增 / 修改的檔案

1. **`docs/demo-recording.md`**（新檔）— **這份不是純 how-to，而是「決策紀錄 + 各方案完整比較 + 操作手冊」三合一**。章節結構：

   1. **TL;DR**（3–5 行）— 「我們選 VHS、原因 X / Y / Z、要重錄跑這行指令」。
   2. **為什麼需要這份文件** — 講動機：未來想換工具、想理解當初取捨、或想了解每個方案的實際限制時，不用重新從零調研。
   3. **候選方案完整介紹**（每個方案一個小節，*不只列名字*）：
      - **VHS** — 是什麼、誰維護、輸入（`.tape` DSL）、輸出（GIF/MP4/WebM）、底層怎麼跑（headless `ttyd` + `ffmpeg`）、安裝方式、典型用法。
      - **asciinema + `agg`** — 同上：concept（錄事件流不錄畫素）、`.cast` 格式、怎麼播（asciinema.org / asciinema-player.js / 轉 GIF via `agg`）。
      - **termtosvg** — 概念、SVG 動畫如何運作、GitHub 怎麼 inline 渲染。
      - **terminalizer** — Node-based、`.yml` 設定檔、為何曾經是首選、現在處境。
      - **svg-term-cli** — asciicast → SVG 的轉換管道。
      - **Kap.app / QuickTime + ffmpeg** — 螢幕錄影派。
   4. **横向比較表**（一張完整表）— 欄位：產出格式 / 是否可重現 / 體積級距 / GitHub README inline 支援 / 互動播放 / 維護活躍度 / 對 bubbletea 高頻 redraw 的友善度 / 學習曲線。
   5. **每個方案的優缺點清單**（每個方案一張 Pros/Cons bullet 對照），優缺點都要具體：例：「VHS Pros: 同 Charmbracelet stack…；Cons: shell 的 PATH 不繼承 `~/.zshrc`、字型回退要手動指定」。
   6. **我們為什麼選 VHS**（決策紀錄章節，獨立一節，不只 TL;DR 一句話）— 列出 sleeper 專案的具體 constraint（bubbletea TUI、要 inline 在 README 動、要可重錄），把每個 constraint 對應到候選方案的勝負原因，並明列「在什麼情況下我們會改選別的」（例：之後想做互動式 demo → 改 asciinema；想要超小體積 → 試 termtosvg）。
   7. **錄製步驟**（推薦路徑：VHS）— 安裝、跑、產出位置、git 該追蹤哪些檔。
   8. **`.tape` 腳本逐行解釋** — 把 `docs/demo.tape` 每個指令的意圖說清楚，未來要改場景時知道改哪一行。
   9. **替代路徑的最小操作示範**（不是只說「也可以用 asciinema」就帶過）— 各給一段可直接複製的指令：
      - asciinema：`asciinema rec docs/demo.cast` → `agg docs/demo.cast docs/demo.gif`
      - termtosvg：`termtosvg docs/demo.svg`
      - QuickTime + ffmpeg → palette → GIF 的指令鏈
   10. **檔案大小優化** — `gifsicle -O3 --colors 128`、`ffmpeg` 改 fps 等實際指令。
   11. **常見問題 / 踩過的坑** — VHS PATH 不繼承、字型 box-drawing 缺 glyph、theme 不一致、GIF 太大。
2. **`docs/demo.tape`**（新檔）— VHS 腳本，可直接 `vhs docs/demo.tape` 重跑。內容大致：
   ```
   Output docs/demo.gif
   Output docs/demo.webm
   Set Shell "zsh"
   Set FontSize 14
   Set Width 1100
   Set Height 700
   Set Theme "Catppuccin Mocha"
   Hide
   Type "cd /Volumes/Data/Program/tries/2026-04-28-sleep-helper && go build -o /tmp/sleeper ./cmd/sleeper"
   Enter
   Sleep 4s
   Show
   Type "/tmp/sleeper --project . --seed 42 --tick 180ms"
   Enter
   Sleep 7s   # 看 default vim+shell scene
   Tab        # → vim+ai
   Sleep 6s
   Tab        # → vim
   Sleep 6s
   Tab        # → shell
   Sleep 5s
   Escape     # panic 第一下
   Escape     # panic 第二下，乾淨退出
   Sleep 1s
   ```
   設計重點：用 `--seed 42` 讓每次錄出來的版面一致、用 `--tick 180ms`（比 default 150ms 慢一點）讓觀眾跟得上動畫、用 `Hide`/`Show` 把 build 步驟藏起來、結尾示範 panic key 也是這個工具的 selling point。
3. **`docs/demo.gif`**（產生物）— 由 `vhs docs/demo.tape` 產出，目標 < 3 MB（GitHub README inline 體驗的甜蜜點）。`docs/demo.webm` 同步產出，當作高畫質備援連結。
4. **`README.md`**（修改）— 在 line 8（警告 blockquote）和 line 10（`## Features`）之間插入：
   ```markdown
   ![sleeper demo](docs/demo.gif)
   
   <sub>Recording produced by [`docs/demo.tape`](docs/demo.tape) — see
   [`docs/demo-recording.md`](docs/demo-recording.md) to re-record.</sub>
   ```

## 重用既有設計

- `--seed`（README:113，`cmd/sleeper/main.go`）— 設成非 0 就是 deterministic 動畫，拍出來每次一樣。
- `--tick`（README:115）— 控制動畫節奏，不用改原始碼就能放慢給觀眾看。
- 用 repo 自己當作 `--project .` 的目標 — 它是 git repo、有 `.go` 文字檔，符合 `internal/scanner` 的 privacy filter 規則，不會掃到敏感檔。
- README 本身已有 `pitfalls/` 連結模式，加 `docs/` 連結符合既有風格。

## 執行步驟（核准後才做）

1. **裝 VHS**：`brew install vhs`（會一併拉 `ttyd` + `ffmpeg`）。
2. **建立 docs/ 目錄與兩個文字檔**：`docs/demo-recording.md` 和 `docs/demo.tape`。
3. **錄製**：`vhs docs/demo.tape`。預期產出 `docs/demo.gif` + `docs/demo.webm`。
4. **檢查體積**：`du -h docs/demo.gif`。如果 > 3 MB，用 `gifsicle -O3 --colors 128` 壓一輪；步驟記在 `docs/demo-recording.md` 的優化章節。
5. **編 README**：插入上面的圖片區塊。
6. **本地預覽**：`open docs/demo.gif`（Quick Look）跟 `grip README.md` 或直接在 GitHub 看 raw markdown。

## Verification

- `vhs docs/demo.tape` 結束時 exit code 0、產出兩個檔案。
- `file docs/demo.gif` 顯示 `GIF image data`、`du -h` 在 3 MB 以下。
- 用 macOS Quick Look 開 `docs/demo.gif`，動畫從 `vim+shell` 起跳、走過三次 Tab、最後 Esc Esc 收尾。
- `grep -n 'demo.gif' README.md` 命中 1 次、相對路徑正確。
- （手動）push 之後在 GitHub 網頁開 README，圖片正常 inline 動起來。
- 重跑驗證：`rm docs/demo.gif && vhs docs/demo.tape` 應該產出位元組長度相近的同樣 GIF（驗 seed 有效）。

## 風險 / 已知 gotcha

- **VHS shell 的 PATH**：VHS 起的 zsh 不一定會載 `~/.zshrc`，所以 `sleeper` 可能不在 PATH。`.tape` 腳本用 `go build -o /tmp/sleeper` + 絕對路徑呼叫，繞過這個坑（也順便文件化在 `docs/demo-recording.md` 的 troubleshooting）。
- **Theme 與字型**：VHS 預設字型可能不含某些 box-drawing glyph。先用 default 跑，若 vim pane 邊框有破字再切 `Set Font "JetBrains Mono"` 或 `"FiraCode Nerd Font"`，把這個 fallback 也寫進文件。
- **GIF 體積**：bubbletea 高頻 redraw 容易把 GIF 撐到 5–10 MB。先量，有需要再加 `gifsicle` 步驟。
- **`docs/demo.webm` 不是必要檔**：如果 webm 體積太大或 GitHub 不認，可以從 `Output` 拿掉，只留 GIF。

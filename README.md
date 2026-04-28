# sleeper

A "look busy" TUI for macOS. Plays a believable fake-coding session on your
screen so you can step away without your laptop locking, your status bouncing
to "away," or your screen filling with screensaver fish.

> ⚠️ This is a joke / personal-productivity toy. It does not edit, write to,
> or transmit any of your project files.

## Features

- **Keeps the screen awake** — runs `caffeinate -dimsu` as a child process and
  cleans it up on exit (verified by `pgrep`-based unit tests).
- **Fake vim** — opens real files from a project you point it at, animates a
  cursor, fakes insert-mode keystrokes, switches files. Never persists edits.
- **Fake shell** — runs a tiny allowlist of *real* read-only commands
  (`ls -la`, `git status`, `git log --oneline -20`, etc.) interleaved with
  hard-coded fake output lines.
- **Fake AI chat** — embedded debug/feature/refactor conversation templates,
  filled with real symbol names lifted from the target project.
- **Instant handover** — press `e` to suspend the TUI and open the currently
  displayed file in `$EDITOR`. Walk back to your desk, type for a few seconds
  in a real editor, exit, the TUI resumes.
- **Panic key** — double-tap `Esc` to immediately switch to a plain
  `git status`-looking shell scene, then quietly quit.

## Install

### `go install` (default)

```bash
go install github.com/daviddwlee84/sleeper/cmd/sleeper@latest
```

Or from a clone:

```bash
go install ./cmd/sleeper
```

The binary lands in `$GOPATH/bin` (typically `~/go/bin`). If `which sleeper`
returns nothing afterwards, that directory isn't on your `PATH`:

```bash
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc && exec zsh
```

### Via [`mise`](https://mise.jdx.dev) (optional)

If you already manage Go with mise and want sleeper to live alongside it
(no extra `PATH` tweak), use the `go:` backend — mise will install + shim it
into its own bin path:

```bash
mise use -g "go:github.com/daviddwlee84/sleeper/cmd/sleeper@latest"
sleeper --help    # resolves via ~/.local/share/mise/shims/sleeper
```

Upgrade later with `mise upgrade sleeper`. This requires the module to be
fetchable by `go install` (i.e. pushed to GitHub with a tag); for a
local-only checkout, fall back to `go install ./cmd/sleeper` from the repo.

> Heads-up: if `which go` returns nothing on a mise host, mise's shell
> activation isn't installed. Add `eval "$(mise activate zsh)"` to your
> `~/.zshrc`, then `mise use -g go@latest`. mise's own
> [getting-started guide](https://mise.jdx.dev/getting-started.html) has
> the full per-shell setup.

## Usage

```bash
sleeper --project ~/work/my-real-repo
```

Flags:

| Flag | Default | Notes |
| --- | --- | --- |
| `--project` | `cwd` | Project to fake-edit. Must contain readable text files. |
| `--scene` | `vim+shell` | Initial layout: `vim`, `shell`, `vim+shell`, `vim+ai` |
| `--ai-style` | `mixed` | Filter AI templates: `debug`, `feature`, `refactor`, `mixed` |
| `--editor` | `$EDITOR` | Override the editor used by the `e` handover key |
| `--seed` | `0` | Deterministic RNG seed (0 = real random). Useful for reproducing layouts. |
| `--tick` | `150ms` | Base animation tick (panes add jitter on top) |
| `--no-caffeinate` | `false` | Skip `caffeinate` (debug). |
| `--no-tui` | `false` | Just hold `caffeinate` until `Ctrl+C`. |

## Hotkeys

| Key | Action |
| --- | --- |
| `Tab` | Cycle layout (vim+shell → vim+ai → vim → shell) |
| `n` | Force vim to switch to a new fake file |
| `Space` | Pause / resume animation |
| `e` | Open the current file in `$EDITOR` (handover) |
| `?` | Toggle help overlay |
| `q` | Quit |
| `Ctrl+C` | Hard quit (caffeinate still cleaned up) |
| `Esc Esc` | **Panic** — switch to shell scene + quit |

## Safety

- **Never writes to disk.** `grep -r 'os.WriteFile\|os.Create' internal/ cmd/`
  shows zero matches outside `t.TempDir()` test fixtures.
- **No shell invocation.** Real commands go through an exact-match `SafeCmd`
  allowlist + `exec.LookPath`. The allowlist is checked again inside `Run()`.
  No template string is ever passed to `sh -c`.
- **Privacy filter on the project scanner.** Skips `.env*`, `*.pem`, `*.key`,
  `id_rsa*`, `id_ed25519*`, `*.p12`, `*.pfx`, `credentials*`, `*.kdbx`. Also
  rejects binary files (via `http.DetectContentType`) and anything > 200 KB.
- **Respects `.gitignore`.** When the project is a git repo, file discovery
  uses `git ls-files -co --exclude-standard` so ignored paths never appear.

## Project layout

```
cmd/sleeper/         CLI entrypoint
internal/
  caffeinate/        caffeinate child process manager (Setpgid + group kill)
  scanner/           project file discovery, privacy filter, symbol grep
  fakevim/           bubbletea pane: vim-like animation + chroma highlighting
  fakeshell/         bubbletea pane: viewport + safe-cmd allowlist
  fakeai/            bubbletea pane: chat-bubble template renderer
  scene/             top-level model: layout, hotkeys, message routing
  handover/          tea.ExecProcess wrapper for $EDITOR handover
```

## Running tests

```bash
go test ./...
```

`caffeinate_test.go` actually starts and stops `caffeinate`, so it'll fail on
non-macOS systems.

<!-- project-knowledge-harness:readme-roadmap -->
<!-- Snippet for project's README.md, placed near other meta sections like
     "Customization" or "Contributing". -->

## Roadmap & lessons learned

Forward-looking work — long-term ideas, deferred items, things needing
evaluation — lives in [`TODO.md`](TODO.md), prioritised P1 → P3 with effort
estimates (S/M/L/XL). Items with accompanying research, design notes, or paused
troubleshooting link to a corresponding [`backlog/<slug>.md`](backlog/) doc.

Backward-looking knowledge — past traps and non-obvious debugging — lives in
[`pitfalls/`](pitfalls/), titled by symptom so future-you can grep the error
message and land on the root cause + workaround instead of re-debugging from
scratch.
<!-- project-knowledge-harness:readme-roadmap --> (end)

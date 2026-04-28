# TODO

Long-term backlog for sleeper. See AGENTS.md
for the maintenance workflow that agents should follow.

> **For agents**: when the user surfaces an idea explicitly **not** being
> implemented this session (signals: "maybe later", "nice to have",
> "工程量太大需要再評估", "先記下來"), add it here with priority + effort tags.
> Do not create new `ROADMAP.md` / `IDEAS.md` / `BACKLOG.md` files —
> `TODO.md` is the single backlog index. Long-form research goes in
> [`backlog/<slug>.md`](backlog/).

<!-- Use the exact section order: P1, P2, P3, P?, Done.
     The bundled scripts/todo-kanban.sh validator only inspects top-level
     `- [ ]` and `- ✅` items inside these sections. Prose paragraphs,
     blockquotes, indented sub-bullets, HTML comments, and `---` rules are
     ignored — feel free to add inline guidance like this without breaking
     machine readability. -->

## P1

Likely next batch — items you'd reach for if you sat down to work today.

## P2

Worth doing, no rush.

- [ ] **[S] Integration test: SIGTERM cleanup invariant** — Add a Go test that spawns sleeper as a subprocess (with `--no-tui`), sends SIGTERM, asserts `pgrep caffeinate` is empty within 2 s. Pairs with the hard-fallback deadline in main; without the test, that path can regress unnoticed. See `pitfalls/bubbletea-quit-deadlock.md`.
- [ ] **[S] Bound the project scan: timeout + file-count cap + warning** — Today `scanner.New(--project)` runs synchronously before TUI launch, and `--project` defaults to `cwd`. Running sleeper from `$HOME` (or any huge tree) walks every reachable file before drawing the first frame — the user sees a black screen for 30+ s and 200+ MB RSS. Add a hard cap (e.g. 5 s wall-clock OR 50k files, whichever first) that aborts the walk and prints `[sleeper] project too large; pass --project <smaller-dir>` to stderr. Considered minor: also default `--project` to refuse `$HOME` / `/`.

## P3

Someday / nice-to-have.

- [ ] **[L] Smarter fake-edit placement (tree-sitter, not LSP)** — Current vim 'o' insertion uses end-of-line heuristic + isCleanEnd. Tree-sitter could give AST-aware insertion points (after a stmt, inside a fn body) without LSP's per-language server overhead. Defer until heuristic visibly fails.
- [ ] **[M] `goreleaser` + GitHub Actions release workflow** — Right now `go install` is the only distribution. Add `.goreleaser.yaml` + `.github/workflows/release.yml` to publish darwin/{amd64,arm64} + linux/{amd64,arm64} tarballs on tag. Stays optional — most users will install via `go install`.
- [ ] **[S] Per-language insert phrases — fill out the gaps** — `insertBank` covers go/python/ts/js/rust + default. Add java/kotlin/swift/c/cpp/ruby/php phrases when a real user complains the AI looks bored.
- [ ] **[M] GNOME/KDE screensaver D-Bus inhibitor** — `systemd-inhibit` blocks logind suspend but doesn't silence gnome-shell's own screen-lock idle timer (`org.gnome.ScreenSaver`). Either add a `godbus/dbus` dep or shell out to `gnome-session-inhibit`/`dbus-send` per DE. Defer until a real Ubuntu-desktop user reports the gap; for now the `gsettings lock-enabled false` workaround documented in `pitfalls/linux-systemd-inhibit-screensaver-gap.md` is good enough.

## P?

Needs a spike before committing to a real priority. Tag as `[?/Effort]`.

- [ ] **[?/M] Frame-rate cap for fakevim** — On ancient terminals chroma-styled output may still be slow even with ANSI256 pinned. Spike: measure terminal-side CPU for our typical View string at various sizes; if > 10% on baseline iTerm2, add `tea.WithFPS(30)` or coalesce ticks.

## Done

Recently shipped. When implementing an active item, in the same commit run:

```
scripts/promote-todo.sh --title "<substring>" --summary "<one-line shipped summary>"
```

This moves the entry here using the dated `Done` syntax and re-validates.

- ✅ [2026-04-28] [P1/M] Linux / Ubuntu support — `systemd-inhibit … cat` keep-awake under `//go:build linux`, build-tagged darwin/linux/other split of `internal/caffeinate`, `ErrUnsupported` soft-fallback to animated-only mode for sandboxes (Docker without `/run/systemd`, WSL1, BSDs, Windows). GNOME screen-lock gap documented as a pitfall + P3 follow-up.
- ✅ [2026-04-28] [P1/M] Round-2 fakevim fixes — tab-expanded loadFile + vim-`o` style insert + pane MaxWidth/MaxHeight cap; top border no longer eaten and inserts land after clean-end lines with matched indent.
- ✅ [2026-04-28] [P1/L] RAM-bomb fix — switched `randomSeed` from `os.ReadFile("/dev/urandom")` to `crypto/rand.Read`, plus mem cap, color profile pin, caffeinate flag fix, signal hard-fallback; 5 pitfalls documented.
- ✅ [2026-04-28] [P1/L] Initial sleeper build — Bubble Tea TUI with caffeinate keep-awake, fakevim/fakeshell/fakeai panes, `$EDITOR` handover, panic key.

<!-- Prune older entries into CHANGELOG.md once prior-year items appear here
     or this section grows past ~20 entries. -->

# Recording the README demo

This document is **a decision record**, not just a how-to. If you're reading
it because you want to (a) re-record the demo, (b) swap recording tools, or
(c) understand why we chose what we did, the answer is here.

## TL;DR

- We use **[VHS](https://github.com/charmbracelet/vhs)** to render
  `docs/demo.tape` → `docs/demo.gif` (embedded in README) + `docs/demo.webm`.
- To re-record: `brew install vhs && vhs docs/demo.tape`.
- Reasons VHS won the bake-off: same Charmbracelet stack as bubbletea (so it
  renders sleeper's redraw pattern cleanly), the recording is a checked-in
  script (`docs/demo.tape`), and GitHub renders the GIF inline in README.
- See [Why we picked VHS](#why-we-picked-vhs) for the full reasoning, and
  [Alternatives](#alternatives) for what to switch to if your constraints
  change.

## Why this document exists

Demo media in a README is small — one image, maybe two. The choice of *how*
to produce that image, however, is sticky: once a `.tape` / `.cast` /
QuickTime workflow is in place, nobody wants to re-discover the trade-offs
the next time the UI changes. This doc records:

1. Every tool we considered and what it actually does.
2. The pros and cons that mattered to **this project specifically** (a
   bubbletea TUI shipped on macOS + Linux, embedded in a GitHub README).
3. The decision and — importantly — when we'd revisit it.

If you're adding a new tool or swapping the current one, update this doc
in the same PR. Future-you will thank present-you.

## Tools considered

### 1. VHS (Charmbracelet)

- **What it is:** a CLI that renders terminal sessions from a `.tape` script.
  Written by Charmbracelet — the same team that maintains `bubbletea`,
  `lipgloss`, and `glamour`, all of which sleeper depends on.
- **Input:** a small DSL (`.tape` file) — one command per line: `Type`,
  `Enter`, `Tab`, `Sleep`, `Set FontSize`, `Set Theme`, `Hide`/`Show`, etc.
- **Output:** GIF, MP4, and/or WebM (you can declare multiple `Output`
  lines in one tape).
- **How it actually runs:** spins up a headless terminal via `ttyd`,
  drives it through your `.tape` script, and pipes the framebuffer to
  `ffmpeg`. Because the terminal is headless, the recording is independent
  of your physical screen size, font, or window-manager quirks.
- **Install:** `brew install vhs` on macOS, official binaries on Linux. The
  brew formula pulls `ttyd` and `ffmpeg` automatically.
- **Typical use:** `vhs path/to/script.tape`.

### 2. asciinema + `agg`

- **What it is:** asciinema records terminal sessions as a stream of
  *events* (keystrokes, redraws, timing) into a JSON file (`.cast`). It
  does **not** record pixels. `agg` (asciinema GIF generator) replays a
  `.cast` and rasterises it to GIF.
- **Input:** an interactive terminal session — you press keys, asciinema
  records them in real time.
- **Output:** `.cast` (text/JSON, ~tens of KB), and optionally GIF via
  `agg` or rendered SVG via `svg-term-cli`.
- **How it actually runs:** asciinema wraps your shell in a PTY and logs
  every read/write with a timestamp. Playback re-emits the writes with
  the same delays.
- **Install:** `brew install asciinema agg` on macOS,
  `apt install asciinema` on Ubuntu.
- **Typical use:** `asciinema rec demo.cast`, then either upload to
  [asciinema.org](https://asciinema.org) for an interactive web player,
  or `agg demo.cast demo.gif` for an embeddable GIF.

### 3. termtosvg

- **What it is:** a Python tool that records a terminal session and emits
  an animated SVG. Different from VHS/agg: SVG keeps the output as text
  glyphs in vector form, not rasterised pixels.
- **Input:** an interactive shell session (or an asciicast).
- **Output:** a single `.svg` file with embedded animation timing
  (CSS `animation` keyframes).
- **How it actually runs:** wraps a shell, captures the output stream
  (similar to asciinema), then renders glyphs as SVG `<text>` nodes
  whose `opacity` / `visibility` is animated over time.
- **Install:** `pipx install termtosvg`.
- **Typical use:** `termtosvg out.svg`, exit shell, embed via
  `<img src="out.svg">`.

### 4. terminalizer

- **What it is:** a Node.js tool that records a terminal session into a
  `.yml` file and renders it to GIF (or a hosted web player on
  terminalizer.com).
- **Input:** interactive session → `.yml` you can hand-edit afterwards.
- **Output:** GIF or shareable web player.
- **How it actually runs:** records via a Node-side PTY library, then
  rasterises frames using a headless Electron / Puppeteer pipeline.
- **Install:** `npm install -g terminalizer`.
- **Typical use:** `terminalizer record demo`, `terminalizer render demo`.

### 5. svg-term-cli

- **What it is:** an asciicast → animated SVG converter. Sister project
  to termtosvg, but pipeline-driven: you record with asciinema first,
  then transform.
- **Input:** an existing `.cast` file.
- **Output:** animated SVG.
- **Install:** `npm install -g svg-term-cli`.
- **Typical use:** `cat demo.cast | svg-term --out demo.svg`.

### 6. Screen recording (Kap.app, QuickTime, OBS) → ffmpeg → GIF

- **What it is:** the no-special-tooling option. Record the actual
  terminal window, then convert to GIF with `ffmpeg`.
- **Input:** your real screen.
- **Output:** MP4 → GIF.
- **How it actually runs:** OS-level screen capture, then `ffmpeg`
  palette-gen + GIF encoding.
- **Install:** `brew install --cask kap` or use the built-in QuickTime
  Player on macOS.
- **Typical use:** record window, then
  `ffmpeg -i in.mp4 -vf "fps=15,scale=1100:-1:flags=lanczos,palettegen" palette.png && ffmpeg -i in.mp4 -i palette.png -lavfi "fps=15,scale=1100:-1:flags=lanczos[x];[x][1:v]paletteuse" out.gif`.

## Side-by-side comparison

| Tool | Output | Reproducible from script? | Typical size (30s demo) | GitHub README inline | Interactive playback | Maintenance | bubbletea redraw friendliness | Learning curve |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **VHS** | GIF / MP4 / WebM | ✅ `.tape` in git | 1–3 MB GIF | ✅ | ❌ | Active, Charmbracelet | ✅✅ (built for it) | Low — small DSL |
| asciinema + agg | `.cast` → GIF | Partial (replay yes, deterministic re-record no) | 2–5 MB GIF | GIF yes, `.cast` no | ✅ on asciinema.org / via player | Active | ✅ | Low |
| termtosvg | Animated SVG | Partial | 100–500 KB | ✅ (raw SVG) | ❌ | Low (last release older) | ⚠️ — heavy redraw can stutter | Low |
| terminalizer | GIF / web player | ✅ `.yml` | 3–10 MB GIF | ✅ | ✅ on terminalizer.com | Slowing | ✅ | Medium |
| svg-term-cli | Animated SVG | Partial (driven by `.cast`) | 100–500 KB | ✅ | ❌ | Low | ⚠️ | Low |
| Kap / QuickTime + ffmpeg | MP4 → GIF | ❌ | 5–15 MB GIF | ✅ | ❌ | OS-level | ✅ | Manual fiddling each time |

## Pros / cons (the parts that mattered for sleeper)

### VHS

- **Pros**
  - Same maintainers as `bubbletea`/`lipgloss`/`chroma`. The team has
    explicitly tested it against their own TUIs, including ones with
    layout cycling and redraws on every tick — exactly sleeper's pattern.
  - Headless: no flicker, no captured macOS notifications, no window
    chrome leaking into the recording.
  - The `.tape` script is the source of truth. `git diff docs/demo.tape`
    is meaningful; `git diff docs/demo.gif` is not.
  - Multiple outputs (GIF + WebM/MP4) from one render.
- **Cons**
  - The shell VHS spawns does **not** auto-source `~/.zshrc`, so anything
    that depends on a user-level `PATH` mutation (mise shims, `~/go/bin`,
    asdf, etc.) won't be visible. Mitigation: build a binary to a known
    absolute path inside the tape (`go build -o /tmp/sleeper`).
  - Default font may not include all box-drawing glyphs; `Set Font
    "JetBrains Mono"` (or any Nerd Font) fixes it.
  - GIF dithering can be aggressive on syntax-highlighted code; if it
    matters, render to MP4/WebM and link instead of inlining.

### asciinema + agg

- **Pros**
  - Tiny `.cast` files (KB, not MB). Great for long demos where GIF
    would balloon.
  - Interactive playback on asciinema.org: pause, scrub, copy text out
    of the recording (it *is* text).
  - Authentic — you record yourself doing the thing, no DSL to learn.
- **Cons**
  - Not deterministic: two recordings of the same session will differ
    in keystroke timing.
  - GitHub README will not auto-render a `.cast` — you either link
    out to asciinema.org or commit a derived GIF (which loses the
    "tiny file" advantage).
  - `agg` GIFs tend to be larger than VHS GIFs of the same length
    because asciinema records every redraw event a TUI emits.

### termtosvg / svg-term-cli

- **Pros**
  - Smallest output files in the table (often <500 KB).
  - SVGs scale crisply to any DPI — the terminal text stays sharp on
    retina displays where rasterised GIFs blur.
- **Cons**
  - High-redraw TUIs (anything that repaints the whole screen on each
    tick — exactly bubbletea's mode) produce SVGs with thousands of
    keyframes, which can stutter or hit browser CPU limits.
  - Less actively maintained; release cadence is irregular.

### terminalizer

- **Pros**
  - Configurable via `.yml`, more granular than VHS's DSL in some areas
    (per-frame delays, prompt customisation).
- **Cons**
  - Larger GIFs than VHS for the same content in our tests.
  - Maintenance has slowed since VHS arrived — VHS now covers most of
    its use cases with a smaller install footprint.

### Screen recording → ffmpeg

- **Pros**
  - Works for *anything* visible on the screen — full-screen apps, GUIs,
    overlays — not just terminals.
- **Cons**
  - Manual, non-reproducible. Every re-record is "I hope I roughly
    remember the timing."
  - Captures real OS chrome: cursor, notifications, resize handles.
  - Largest output sizes in the table.

## Why we picked VHS

sleeper has three project-specific constraints that decide this:

1. **It's a bubbletea TUI with multi-pane redraws on every animation
   tick.** Anything that records the *terminal stream* (asciinema, termtosvg)
   captures a lot of redraw events; anything that records the *screen*
   (Kap, QuickTime) captures real-world flicker. VHS is the only option
   that drives a headless terminal AND uses a frame-buffer pipeline,
   which sidesteps both classes of artifact.
2. **The demo lives in the README and must inline-render on GitHub.**
   That rules out asciinema's `.cast` (links out only) and any plain
   MP4 (GitHub `<img>` doesn't autoplay video). GIF, WebP, or SVG are
   the candidates.
3. **The demo will need re-recording every time the UI changes.** If
   re-recording costs more than five minutes of human attention, it
   won't happen, and the README will go stale. Only VHS and terminalizer
   produce the recording from a checked-in script; of those two, VHS
   is more actively maintained and handles bubbletea redraws cleaner.

The tie-breaker is **same-vendor confidence**: VHS, bubbletea, lipgloss,
and chroma all live under the Charmbracelet umbrella. When VHS adds new
TUI-friendly features (e.g. better handling of cursor styles, theme
parity), they tend to land in the same release cycle as the things that
make sleeper render correctly.

### When we'd switch

| If this becomes true… | …switch to |
| --- | --- |
| The demo grows past ~45s and GIF size hits 5+ MB | asciinema + asciinema.org link |
| Readers want to copy code out of the recording | asciinema (interactive web player) |
| We want crisp rendering on 4K monitors and ≤500 KB | termtosvg, accepting some redraw stutter |
| We need a non-terminal demo (full-app screen recording) | Kap.app + ffmpeg |

## Recording with VHS (the recommended path)

### Install

```bash
# macOS
brew install vhs

# Linux (Ubuntu / Debian)
# https://github.com/charmbracelet/vhs/releases — grab the .deb
```

`brew install vhs` pulls `ttyd` and `ffmpeg` as dependencies. On Linux,
make sure `ffmpeg` is on your `PATH` (`apt install ffmpeg`).

### Render

From the repo root:

```bash
vhs docs/demo.tape
```

This produces `docs/demo.gif` and `docs/demo.webm`. Commit both — they
are part of the README's user experience.

### Anatomy of `docs/demo.tape`

Every line is one VHS instruction. The commented copy in the file
itself is the canonical version; the one-liner explanations below are a
quick reference:

| Line group | What it does |
| --- | --- |
| `Output docs/demo.gif` / `Output docs/demo.webm` | Declares two outputs from one render. |
| `Set Shell "zsh"` | Spawns a fresh `zsh` for the session. **Does not source `~/.zshrc`.** |
| `Set FontSize 14` / `Set Width 1100` / `Set Height 700` | Terminal dimensions. 1100 fits a typical README column. |
| `Set Theme "Catppuccin Mocha"` | Color scheme (any [Glamour theme](https://github.com/charmbracelet/glamour) name works). |
| `Set TypingSpeed 60ms` | How fast `Type` keystrokes appear. |
| `Hide` … `Show` | Bracket the off-camera build (`go build -o /tmp/sleeper`) so it doesn't show in the GIF. |
| `Type "…"` / `Enter` | Inputs to the headless shell. |
| `Sleep 7s` | Hold on this frame so viewers can read. |
| `Tab` | Cycles sleeper's layout. We do this three times to show every scene. |
| `Escape` `Escape` | Triggers sleeper's panic-key, which doubles as a clean exit. |

### Re-record after UI changes

```bash
rm docs/demo.gif docs/demo.webm
vhs docs/demo.tape
du -h docs/demo.gif         # sanity-check size
git add docs/demo.tape docs/demo.gif docs/demo.webm
```

If a layout was added or removed, edit `docs/demo.tape` (add/remove a
`Tab` block) before re-rendering.

## Alternative recipes (for the cases in the switch table above)

These produce different artifacts than VHS does, but each is a
copy-paste-and-go starting point.

### asciinema + agg → GIF + asciinema.org link

```bash
brew install asciinema agg
asciinema rec docs/demo.cast \
  --command "go run ./cmd/sleeper --project . --seed 42 --tick 180ms --no-caffeinate" \
  --idle-time-limit 2
# Press Esc Esc inside sleeper to exit; recording stops automatically.

agg docs/demo.cast docs/demo.gif

# Optional: upload for an interactive player
asciinema upload docs/demo.cast
```

### termtosvg → animated SVG

```bash
pipx install termtosvg
termtosvg docs/demo.svg \
  --command "go run ./cmd/sleeper --project . --seed 42 --tick 180ms --no-caffeinate"
# Exit sleeper with Esc Esc; SVG is finalised on shell exit.
```

Embed in README with `![](docs/demo.svg)` — GitHub renders inline SVG.
Watch the file size: if it crosses ~1 MB, fall back to GIF.

### Kap.app + ffmpeg → GIF (manual)

```bash
brew install --cask kap
brew install ffmpeg
# 1. Run sleeper in a terminal: ./sleeper --project . --seed 42
# 2. In Kap, capture just the terminal window. Export as MP4.
# 3. Convert MP4 → GIF with a palette pass (smaller, no banding):
ffmpeg -i kap.mp4 -vf "fps=15,scale=1100:-1:flags=lanczos,palettegen" /tmp/p.png
ffmpeg -i kap.mp4 -i /tmp/p.png -lavfi \
  "fps=15,scale=1100:-1:flags=lanczos[x];[x][1:v]paletteuse" docs/demo.gif
```

## Optimising file size

Order of preference:

1. **Trim the tape first.** Shorter is smaller. Aim for ≤30s.
2. **Drop FPS.** VHS defaults work for sleeper; if you used Kap, set
   `fps=10` in the `ffmpeg` palette filter.
3. **Reduce colours.** `gifsicle -O3 --colors 128 docs/demo.gif -o docs/demo.gif`.
   sleeper's TUI palette is small (lipgloss + a handful of chroma syntax
   colours); 128 is usually transparent.
4. **Last resort:** drop `docs/demo.gif` from README and embed
   `docs/demo.webm` via `<video>` — works on github.com once committed,
   but won't render on raw `cat README.md`.

Target: GIF ≤ 3 MB (GitHub renders larger GIFs but mobile users will
hate you).

## Troubleshooting

| Symptom | Cause | Fix |
| --- | --- | --- |
| `command not found: sleeper` inside VHS | VHS shell does not source `~/.zshrc`, so user `PATH` additions (`~/go/bin`, mise, asdf) are absent. | Already mitigated — `docs/demo.tape` builds to `/tmp/sleeper` and calls it by absolute path. |
| Box-drawing characters render as `?` | Default VHS font lacks the glyphs. | Add `Set Font "JetBrains Mono"` (or any Nerd Font) to `docs/demo.tape`. |
| Colours look off vs. running sleeper directly | `Set Theme` value mismatched, or your real terminal uses a different background. | Pick a theme close to your real terminal: `"Catppuccin Mocha"` (dark) or `"GitHub"` (light). |
| Generated GIF is 8+ MB | High redraw rate × long Sleep × big window. | Trim Sleep durations, drop `Set Width` / `Set Height`, run `gifsicle -O3 --colors 128`. |
| Animation looks choppy in browser | Browser is rate-limiting GIF playback (common in Chrome on battery). | Open the GIF directly (not in README preview), or render to MP4 instead. |
| Re-rendering produces a noticeably different GIF | sleeper was started without `--seed`, or with `--seed 0` (= real random). | `docs/demo.tape` already pins `--seed 42`; if you change it, pin a non-zero value. |

## Summary

- VHS is the recording tool of record for this repo.
- The recording itself is `docs/demo.tape` — that is the artifact to
  edit when the UI changes.
- The other tools in this doc aren't deprecated, just not chosen — keep
  them in mind if the constraints in [When we'd switch](#when-wed-switch)
  start applying.

# Bubble Tea + chroma TUI is unusably slow / heavy on iTerm2 truecolor terminals

**Symptoms** (grep this section): TUI lags, terminal CPU spike, ANSI escape blowup, `\x1b[38;2;`, `\x1b[48;2;`, lipgloss truecolor, termenv ANSI256, chroma terminal256, double styling, `lipgloss.SetColorProfile`, color profile auto-detect
**First seen**: 2026-04
**Affects**: any Bubble Tea / lipgloss app that also embeds chroma syntax-highlighted output, when run under truecolor-capable terminals (iTerm2, modern Alacritty, kitty, etc.)
**Status**: fixed by pinning the lipgloss color profile before any styles are constructed

## Symptom

Same Go program, same content, very different terminal behaviour:

- macOS Terminal.app (256-color): smooth, ~22 MB RSS, terminal CPU ≈ 0%.
- iTerm2 / Alacritty (24-bit truecolor): noticeably laggy redraws,
  terminal-process CPU ≥ 30%, occasional whole-frame stalls.

`tee` of the program's stdout shows lines like:

```
\x1b[38;2;0;255;0m...\x1b[48;5;0m\x1b[38;5;28m...\x1b[0m
```

Both 24-bit (`38;2;…`) and 256-color (`38;5;…`) escape codes wrapping
the same character — sometimes interleaved on the same line.

## Root cause

Two color systems cooperate in this stack:

1. **chroma** highlights source code. We requested the `terminal256`
   formatter, so its output is 256-color escape sequences (`\x1b[38;5;…m`).
2. **lipgloss / termenv** styles UI chrome (borders, status bars, text
   blocks). At first use, termenv runs an *auto-detection* — sniffs
   `TERM`, `COLORTERM`, queries the terminal — and picks the highest
   profile the terminal claims to support. On iTerm2 / Alacritty /
   kitty, that's `TrueColor`.

When lipgloss-styled content **wraps chroma-styled content** (e.g.
`paneStyle.Render(highlightLine(...))`), termenv re-emits its styles in
24-bit form alongside chroma's existing 256-color sequences. Each
character can pick up two SGR open + reset pairs. A 200-char line ends
up with hundreds of ANSI bytes, and per-render ANSI parsing inside the
terminal compounds into measurable lag and memory pressure on the
*terminal process*.

This is allowed by the spec (terminals tolerate redundant SGR), but it's
expensive to render. It's also a known footgun in the
`charmbracelet/lipgloss` issue tracker — search "color profile
detection".

## Workaround

Pin lipgloss's profile to ANSI256 *before* constructing any styles
(crucially, before calling any `lipgloss.NewStyle().Foreground(...)` —
the first call locks the profile via `sync.Once`):

```go
import (
    "github.com/charmbracelet/lipgloss"
    "github.com/muesli/termenv"
)

func main() {
    // Must run before any other lipgloss API touches a color.
    lipgloss.SetColorProfile(termenv.ANSI256)
    // ...rest of main
}
```

Now both layers emit 256-color codes; the terminal sees one consistent
encoding per glyph. No double-coding, no doubled ANSI byte count.

If you want truecolor for the UI chrome and 256 for chroma, you have to
use `lipgloss.NewRenderer(io.Writer)` per-pane with explicit profiles —
much more invasive. For most TUIs, ANSI256 across the board is fine.

## Prevention

- **Always pin the color profile** in any TUI that mixes lipgloss with
  another ANSI-emitting library (chroma, glamour, etc.).
- **Never pad styled output with literal spaces** — use lipgloss's
  `Width()` / `MaxWidth()`, which are ANSI-aware. Manual `s + strings.Repeat(" ", n)`
  on a chroma string is OK only because the spaces are unstyled; the
  moment you add a background style to the wrapper, the math breaks.
- **Avoid `terminal16m`** as the chroma formatter unless the entire
  pipeline is truecolor end-to-end. `terminal256` is the safer default.

## Related

- Sibling pitfalls: `pitfalls/fakevim-tab-truncation.md` (also a
  width-calculation footgun in the same render path).
- Source: `cmd/sleeper/main.go` calls `lipgloss.SetColorProfile(termenv.ANSI256)`
  inside `run()` before `scene.New`; commit `696ef00`.
- Upstream: `github.com/charmbracelet/lipgloss` `renderer.go::SetColorProfile`.

# TUI pane top border disappears when content has tabs — byte-len truncation

**Symptoms** (grep this section): top border missing, `╭───╮` gone, pane height grows, lipgloss word-wrap, content overflows pane, line wraps to next row, INSERT mode breaks layout, `truncatePlain`, `lipgloss.Width`, tab characters, `\t`, Go source formatting, cell width vs byte length
**First seen**: 2026-04
**Affects**: any lipgloss-rendered TUI pane that fixes child width by `len(string)` instead of `lipgloss.Width(string)` — bites hardest on Go source code (mandatory tabs)
**Status**: fixed by expanding tabs once at file load

## Symptom

Pane border renders cleanly when displaying a tab-free file
(`fakevim/highlight.go`, all spaces):

```
╭───────────────────╮
│ package fakevim   │
│ ...
```

Renders broken when displaying a tab-indented Go file
(`cmd/sleeper/main.go`):

```
package main          ← top border literally gone
import (
    "flag"
...
```

Toggling `Tab` between layouts fixes it temporarily, then it breaks
again next time INSERT mode adds a line.

## Root cause

`internal/fakevim/model.go::truncatePlain` was:

```go
func truncatePlain(s string, w int) string {
    if len(s) > w { return s[:w] }
    if len(s) < w { return s + strings.Repeat(" ", w-len(s)) }
    return s
}
```

`len(s)` counts **bytes**. Each `'\t'` is 1 byte. The terminal /
lipgloss treat `'\t'` as a tab-stop advance — usually 4 or 8 cells.
A line that's 60 bytes long ("\t\t\tfunc foo(x int) error {") might
display as 90+ cells.

Pipeline:
1. fakevim padded to "60 bytes" thinking it'd be 60 cells wide.
2. chroma highlights — visible width unchanged.
3. lipgloss `paneStyle.Render(content)` runs `cellbuf.Wrap(content, w-padding, "")`.
4. `cellbuf.Wrap` uses `lipgloss.Width()` (cell-aware), sees the line is
   90 cells wide, **wraps mid-line**.
5. Each wrapped line adds 1 to the rendered block height.
6. lipgloss's `Height(N)` is a *minimum*, not a maximum — content
   that's taller than N just makes the block taller.
7. The pane is now taller than the terminal viewport. Terminal scrolls
   down to show the bottom. The top border falls off-screen.

Bug 6 above (Height as minimum, not maximum) is the second part of the
issue and applies to *any* render that overflows for any reason; see
the prevention section.

## Workaround

Two complementary fixes:

**1. Expand tabs once on file load** so byte length matches cell width
for the rest of the model's life:

```go
// internal/fakevim/model.go
func (m *Model) loadFile(f scanner.File) {
    lines, _ := scanner.ReadLines(f)
    m.lines = make([]string, len(lines))
    for i, ln := range lines {
        m.lines[i] = expandTabs(ln, 4)  // tab-stop aware, not naive replace
    }
    // ...
}

// expandTabs replaces each '\t' with the number of spaces needed to
// reach the next column that is a multiple of tabSize.
func expandTabs(s string, tabSize int) string {
    if !strings.ContainsRune(s, '\t') { return s }
    var b strings.Builder
    col := 0
    for _, r := range s {
        if r == '\t' {
            n := tabSize - (col % tabSize)
            for i := 0; i < n; i++ { b.WriteByte(' ') }
            col += n
            continue
        }
        b.WriteRune(r)
        col++
    }
    return b.String()
}
```

Note: tab-stop aware. `\tab\t` is NOT `(4 spaces)ab(4 spaces)` — the
second tab fills only 2 spaces because we're at column 6.

**2. Add `MaxWidth` / `MaxHeight` as a hard cap** on the pane style so
that any *future* render bug can't push the border off-screen:

```go
// internal/scene/scene.go
return lipgloss.NewStyle().
    Border(lipgloss.RoundedBorder()).
    Padding(0, 1).
    Width(w-2).Height(h-2).
    MaxWidth(w).MaxHeight(h)
```

Belt-and-braces: the tab fix prevents the overflow; MaxHeight prevents
the symptom even if some other line overflows for some other reason
(zero-width chars, emoji, future bugs).

## Prevention

- **Hard rule**: in any TUI renderer, never use `len(string)` to size
  visible content. Always use `lipgloss.Width()` (or
  `runewidth.StringWidth`) which accounts for tabs, double-width CJK,
  emoji, and ANSI escapes.
- **For pane containers, always set both `Width(N)` and `MaxWidth(N)`**
  (and likewise Height/MaxHeight). lipgloss's `Width` is "minimum
  block width / wrap point"; `MaxWidth` is the hard cap. Without the
  cap, any oversized child silently expands the pane and breaks the
  layout.
- **Test on real source files with tabs.** A unit test that renders a
  tab-heavy line and asserts the resulting block height matches `Height`
  would have caught this immediately.

## Related

- Sibling pitfalls: `pitfalls/lipgloss-truecolor-double-styling.md`
  (also a width-calculation gotcha further up the same render path).
- Source: `internal/fakevim/model.go::expandTabs`,
  `internal/scene/scene.go::paneStyle`; commit pending in this branch.

package fakevim

import (
	"math/rand/v2"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/daviddwlee84/sleeper/internal/scanner"
)

// vimState drives the typing animation state machine.
type vimState int

const (
	stateNormal vimState = iota
	stateInsert
	stateCommand
)

// TickMsg is the per-pane animation tick.
type TickMsg time.Time

// FileSwitchedMsg is published when fakevim moves on to a new file. The scene
// can route this to other panes (e.g. AI says "let's look at X.go now").
type FileSwitchedMsg struct {
	File scanner.File
}

type Model struct {
	scan   *scanner.Scanner
	file   scanner.File
	lines  []string
	width  int
	height int

	cursorRow int
	cursorCol int

	state vimState
	// remaining keystrokes to inject before transitioning back to normal
	insertBuf  []rune
	cmdBuf     string
	burstLeft  int
	pausedTill time.Time

	rng    *rand.Rand
	paused bool

	// scrollOffset is the first body line we render, for poor-man scrolling.
	scrollOffset int
}

// New constructs a fakevim model. width/height are zero until the first
// WindowSizeMsg arrives.
func New(s *scanner.Scanner, seed uint64) Model {
	m := Model{
		scan:      s,
		state:     stateNormal,
		burstLeft: 8 + int(seed%6),
		rng:       rand.New(rand.NewPCG(seed|1, seed|2)),
	}
	m.loadFile(s.Pick())
	return m
}

func (m *Model) loadFile(f scanner.File) {
	m.file = f
	lines, err := scanner.ReadLines(f)
	if err != nil {
		m.lines = []string{"// (failed to read file)", "// " + err.Error()}
	} else if len(lines) == 0 {
		m.lines = []string{""}
	} else {
		m.lines = lines
	}
	m.cursorRow = m.rng.IntN(len(m.lines))
	m.cursorCol = 0
	m.scrollOffset = 0
	m.state = stateNormal
}

// CurrentFile returns the path of the file we're pretending to edit.
func (m Model) CurrentFile() scanner.File { return m.file }

func (m Model) Init() tea.Cmd { return tick(120 * time.Millisecond) }

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return TickMsg(t) })
}

// SetSize is called by the scene when the layout changes.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// SetPaused toggles whether the animation produces new keystrokes.
func (m *Model) SetPaused(p bool) { m.paused = p }

// NextFile forces an immediate file switch.
func (m *Model) NextFile() tea.Cmd {
	old := m.file
	for i := 0; i < 8; i++ {
		f := m.scan.Pick()
		if f.Path != old.Path {
			m.loadFile(f)
			break
		}
	}
	return func() tea.Msg { return FileSwitchedMsg{File: m.file} }
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return m, nil
	case TickMsg:
		if m.paused {
			return m, tick(400 * time.Millisecond)
		}
		next, cmd := m.step()
		return m, tea.Batch(cmd, tick(next))
	}
	return m, nil
}

// step advances the animation by one frame and returns the delay until the
// next frame plus any messages to publish.
func (m *Model) step() (time.Duration, tea.Cmd) {
	if !m.pausedTill.IsZero() && time.Now().Before(m.pausedTill) {
		return 200 * time.Millisecond, nil
	}
	switch m.state {
	case stateNormal:
		return m.stepNormal()
	case stateInsert:
		return m.stepInsert()
	case stateCommand:
		return m.stepCommand()
	}
	return 200 * time.Millisecond, nil
}

func (m *Model) stepNormal() (time.Duration, tea.Cmd) {
	if m.burstLeft <= 0 {
		// finish this file: type :w, then switch
		m.state = stateCommand
		m.cmdBuf = ":w"
		return 350 * time.Millisecond, nil
	}
	roll := m.rng.IntN(100)
	switch {
	case roll < 30: // jump to a new line within range
		m.cursorRow = clamp(m.cursorRow+m.rng.IntN(11)-5, 0, len(m.lines)-1)
		m.cursorCol = clampCol(m.lines, m.cursorRow, m.cursorCol)
		m.adjustScroll()
		return jitter(m.rng, 80, 220), nil
	case roll < 60: // move within the line
		ln := m.lines[m.cursorRow]
		if len(ln) > 0 {
			m.cursorCol = m.rng.IntN(len(ln) + 1)
		}
		return jitter(m.rng, 60, 180), nil
	case roll < 90: // enter insert mode and type a small phrase
		m.state = stateInsert
		m.insertBuf = []rune(pickInsertPhrase(m.rng, m.file))
		return jitter(m.rng, 200, 450), nil
	default: // longer pause "thinking"
		m.pausedTill = time.Now().Add(time.Duration(800+m.rng.IntN(1200)) * time.Millisecond)
		return 200 * time.Millisecond, nil
	}
}

func (m *Model) stepInsert() (time.Duration, tea.Cmd) {
	if len(m.insertBuf) == 0 {
		m.state = stateNormal
		m.burstLeft--
		return jitter(m.rng, 150, 400), nil
	}
	r := m.insertBuf[0]
	m.insertBuf = m.insertBuf[1:]
	m.injectChar(r)
	return jitter(m.rng, 35, 110), nil
}

func (m *Model) stepCommand() (time.Duration, tea.Cmd) {
	// after the :w "settles", switch to a new file
	m.cmdBuf = ""
	m.state = stateNormal
	m.burstLeft = 6 + m.rng.IntN(8)
	return 600 * time.Millisecond, m.NextFile()
}

// injectChar updates the *display* of the current line to look like the
// character was typed. We never persist these edits.
func (m *Model) injectChar(r rune) {
	if m.cursorRow >= len(m.lines) {
		return
	}
	if r == '\n' {
		// split the line so it looks like Enter was pressed
		ln := m.lines[m.cursorRow]
		if m.cursorCol > len(ln) {
			m.cursorCol = len(ln)
		}
		head, tail := ln[:m.cursorCol], ln[m.cursorCol:]
		m.lines = append(m.lines[:m.cursorRow], append([]string{head, tail}, m.lines[m.cursorRow+1:]...)...)
		m.cursorRow++
		m.cursorCol = 0
		m.adjustScroll()
		return
	}
	ln := m.lines[m.cursorRow]
	if m.cursorCol > len(ln) {
		m.cursorCol = len(ln)
	}
	m.lines[m.cursorRow] = ln[:m.cursorCol] + string(r) + ln[m.cursorCol:]
	m.cursorCol++
}

func (m *Model) adjustScroll() {
	bodyH := m.bodyHeight()
	if bodyH <= 0 {
		return
	}
	if m.cursorRow < m.scrollOffset {
		m.scrollOffset = m.cursorRow
	}
	if m.cursorRow >= m.scrollOffset+bodyH {
		m.scrollOffset = m.cursorRow - bodyH + 1
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

func (m Model) bodyHeight() int {
	// width/height are the "inner" dims given by the scene. We reserve 1 line
	// for the status line.
	return m.height - 1
}

// View renders the fake-vim pane. The scene wraps it in a border.
func (m Model) View() string {
	if m.height <= 1 || m.width <= 4 {
		return ""
	}
	bodyH := m.bodyHeight()
	gutterW := digitWidth(len(m.lines)) + 1
	bodyW := m.width - gutterW
	if bodyW < 4 {
		bodyW = 4
	}

	var b strings.Builder
	for i := 0; i < bodyH; i++ {
		row := m.scrollOffset + i
		if row >= len(m.lines) {
			b.WriteString(emptyGutter.Render("~"))
			b.WriteString(strings.Repeat(" ", bodyW))
			b.WriteByte('\n')
			continue
		}
		b.WriteString(gutter.Render(rightPad(itoa(row+1), gutterW-1) + " "))
		ln := m.lines[row]
		if row == m.cursorRow {
			b.WriteString(renderCursorLine(ln, m.cursorCol, bodyW, m.state == stateInsert))
		} else {
			// truncate plain content first to avoid slicing ANSI codes,
			// then hand to chroma.
			b.WriteString(highlightLine(m.file.Lang, truncatePlain(ln, bodyW)))
		}
		b.WriteByte('\n')
	}

	status := m.statusLine()
	return strings.TrimRight(b.String(), "\n") + "\n" + status
}

func (m Model) statusLine() string {
	mode := "NORMAL"
	switch m.state {
	case stateInsert:
		mode = "INSERT"
	case stateCommand:
		mode = "COMMAND"
	}
	left := statusModeStyle(m.state).Render(" " + mode + " ")
	mid := statusFile.Render(" " + filepath.Base(m.file.Rel) + " ")
	right := statusPos.Render(" " + itoa(m.cursorRow+1) + ":" + itoa(m.cursorCol+1) + " ")
	cmd := ""
	if m.state == stateCommand {
		cmd = statusCmd.Render(" " + m.cmdBuf)
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(mid) - lipgloss.Width(right) - lipgloss.Width(cmd)
	if gap < 0 {
		gap = 0
	}
	return left + mid + cmd + strings.Repeat(" ", gap) + right
}

// ---------- helpers ----------

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampCol(lines []string, row, col int) int {
	if row < 0 || row >= len(lines) {
		return 0
	}
	if col > len(lines[row]) {
		return len(lines[row])
	}
	if col < 0 {
		return 0
	}
	return col
}

func jitter(r *rand.Rand, minMs, maxMs int) time.Duration {
	return time.Duration(minMs+r.IntN(maxMs-minMs)) * time.Millisecond
}

func digitWidth(n int) int {
	if n <= 0 {
		return 1
	}
	w := 0
	for n > 0 {
		w++
		n /= 10
	}
	if w < 3 {
		return 3
	}
	return w
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func rightPad(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return strings.Repeat(" ", w-len(s)) + s
}

func truncate(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s + strings.Repeat(" ", w-lipgloss.Width(s))
	}
	// naive byte slice — fakevim sees plain text from disk so this is safe enough
	if len(s) <= w {
		return s
	}
	return s[:w]
}

// truncatePlain handles only PLAIN strings (no ANSI). Pads with spaces to w.
func truncatePlain(s string, w int) string {
	if len(s) > w {
		return s[:w]
	}
	if len(s) < w {
		return s + strings.Repeat(" ", w-len(s))
	}
	return s
}

func renderCursorLine(line string, col, width int, insert bool) string {
	if col > len(line) {
		col = len(line)
	}
	pre, mid, post := line[:col], "", line[col:]
	if len(post) > 0 {
		mid = string(post[0])
		post = post[1:]
	} else {
		mid = " "
	}
	style := cursorBlock
	if insert {
		style = cursorBar
	}
	rendered := pre + style.Render(mid) + post
	if lipgloss.Width(rendered) > width {
		// be safe; trimming with ANSI is risky — drop trailing
		if len(line) > width {
			line = line[:width]
		}
		return line
	}
	pad := width - lipgloss.Width(rendered)
	if pad > 0 {
		rendered += strings.Repeat(" ", pad)
	}
	return rendered
}

// pickInsertPhrase generates a plausible-looking string of characters to type.
// We never use the file's real symbol context as input to template strings —
// just static phrases that look like code.
func pickInsertPhrase(r *rand.Rand, f scanner.File) string {
	bank := insertBank[f.Lang]
	if len(bank) == 0 {
		bank = insertBank["_default"]
	}
	return bank[r.IntN(len(bank))]
}

var insertBank = map[string][]string{
	"go": {
		"if err != nil {\n\treturn err\n}",
		"// TODO: handle edge case",
		"ctx, cancel := context.WithTimeout(ctx, 2*time.Second)\ndefer cancel()",
		"log.Printf(\"debug: %+v\\n\", v)",
		"return fmt.Errorf(\"foo: %w\", err)",
	},
	"python": {
		"if not result:\n    raise ValueError(\"empty\")",
		"# TODO: refactor this branch",
		"logger.debug(\"value=%s\", value)",
		"with open(path, \"r\") as fh:\n    data = fh.read()",
	},
	"typescript": {
		"if (!data) throw new Error('missing');",
		"// TODO: tighten the type",
		"const next = await fetchNext(id);",
	},
	"_default": {
		"// TODO",
		"// fixme",
		"// note: review this later",
	},
}

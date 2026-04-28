package scene

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/daviddwlee84/sleeper/internal/fakeai"
	"github.com/daviddwlee84/sleeper/internal/fakeshell"
	"github.com/daviddwlee84/sleeper/internal/fakevim"
	"github.com/daviddwlee84/sleeper/internal/handover"
	"github.com/daviddwlee84/sleeper/internal/scanner"
)

// Layout chooses which panes to display and how to arrange them.
type Layout int

const (
	LayoutVimOnly Layout = iota
	LayoutShellOnly
	LayoutVimShell
	LayoutVimAI // ai pane wired in step 6
)

// quitTickMsg is used by the panic-key path to exit after one final repaint.
type quitTickMsg struct{}

// Options holds the runtime configuration the scene needs.
type Options struct {
	Editor        string
	InitialLayout Layout
	AIStyle       string
}

type Model struct {
	scan   *scanner.Scanner
	vim    fakevim.Model
	shell  fakeshell.Model
	ai     fakeai.Model
	layout Layout
	opts   Options

	width, height int
	paused        bool
	helpOpen      bool

	// panic key tracking
	lastEsc time.Time
}

// New constructs a Model. seed=0 means "use a real random seed".
func New(s *scanner.Scanner, opts Options, seed uint64) (Model, error) {
	if seed == 0 {
		seed = uint64(time.Now().UnixNano())
	}
	ai, err := fakeai.New(s, opts.AIStyle, seed^0xc3c3c3c3)
	if err != nil {
		return Model{}, err
	}
	return Model{
		scan:   s,
		vim:    fakevim.New(s, seed),
		shell:  fakeshell.New(s.Root, seed^0xa5a5a5a5),
		ai:     ai,
		layout: opts.InitialLayout,
		opts:   opts,
	}, nil
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.vim.Init(), m.shell.Init(), m.ai.Init())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.dispatchSize()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case handover.RedrawMsg:
		return m, tea.ClearScreen

	case quitTickMsg:
		return m, tea.Quit

	case fakevim.TickMsg:
		var cmd tea.Cmd
		m.vim, cmd = m.vim.Update(msg)
		return m, cmd

	case fakeshell.TickMsg:
		var cmd tea.Cmd
		m.shell, cmd = m.shell.Update(msg)
		return m, cmd

	case fakeai.TickMsg:
		var cmd tea.Cmd
		m.ai, cmd = m.ai.Update(msg)
		return m, cmd

	case fakevim.FileSwitchedMsg:
		// AI pane optionally re-themes around the new file
		var cmd tea.Cmd
		m.ai, cmd = m.ai.Update(fakeai.SwitchTopicMsg{File: msg.File})
		return m, cmd
	}

	// route everything else to all panes; they ignore msgs they don't care about
	var c1, c2, c3 tea.Cmd
	m.vim, c1 = m.vim.Update(msg)
	m.shell, c2 = m.shell.Update(msg)
	m.ai, c3 = m.ai.Update(msg)
	return m, tea.Batch(c1, c2, c3)
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		return m, tea.Quit
	case "tab":
		m.layout = nextLayout(m.layout)
		m.dispatchSize()
		return m, tea.ClearScreen
	case " ":
		m.paused = !m.paused
		m.vim.SetPaused(m.paused)
		m.shell.SetPaused(m.paused)
		m.ai.SetPaused(m.paused)
		return m, nil
	case "n":
		cmd := m.vim.NextFile()
		return m, cmd
	case "?":
		m.helpOpen = !m.helpOpen
		return m, tea.ClearScreen
	case "e":
		// $EDITOR handover on the currently displayed file
		f := m.vim.CurrentFile()
		return m, handover.ExecEditor(m.opts.Editor, f.Path)
	case "esc":
		// double-tap esc = panic key: switch to shell-only and quit
		now := time.Now()
		if !m.lastEsc.IsZero() && now.Sub(m.lastEsc) < 600*time.Millisecond {
			m.layout = LayoutShellOnly
			m.dispatchSize()
			return m, tea.Tick(900*time.Millisecond, func(time.Time) tea.Msg { return quitTickMsg{} })
		}
		m.lastEsc = now
		return m, nil
	}
	return m, nil
}

func nextLayout(l Layout) Layout {
	switch l {
	case LayoutVimShell:
		return LayoutVimAI
	case LayoutVimAI:
		return LayoutVimOnly
	case LayoutVimOnly:
		return LayoutShellOnly
	case LayoutShellOnly:
		return LayoutVimShell
	default:
		return LayoutVimShell
	}
}

func (m *Model) dispatchSize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	switch m.layout {
	case LayoutVimOnly:
		w, h := paneInner(m.width, m.height)
		m.vim.SetSize(w, h)
	case LayoutShellOnly:
		w, h := paneInner(m.width, m.height)
		m.shell.SetSize(w, h)
	case LayoutVimShell:
		// split horizontally; each child gets ~half width
		leftW := m.width / 2
		rightW := m.width - leftW
		lw, lh := paneInner(leftW, m.height)
		rw, rh := paneInner(rightW, m.height)
		m.vim.SetSize(lw, lh)
		m.shell.SetSize(rw, rh)
	case LayoutVimAI:
		leftW := m.width / 2
		rightW := m.width - leftW
		lw, lh := paneInner(leftW, m.height)
		rw, rh := paneInner(rightW, m.height)
		m.vim.SetSize(lw, lh)
		m.ai.SetSize(rw, rh)
	}
}

// paneInner subtracts border (1 each side) and padding (1 each side).
func paneInner(w, h int) (int, int) {
	iw := w - 4
	ih := h - 2
	if iw < 4 {
		iw = 4
	}
	if ih < 2 {
		ih = 2
	}
	return iw, ih
}

func (m Model) View() string {
	if m.helpOpen {
		return m.renderHelp()
	}
	switch m.layout {
	case LayoutVimOnly:
		return paneStyle(m.width, m.height, "vim").Render(m.vim.View())
	case LayoutShellOnly:
		return paneStyle(m.width, m.height, "shell").Render(m.shell.View())
	case LayoutVimShell:
		leftW := m.width / 2
		rightW := m.width - leftW
		left := paneStyle(leftW, m.height, "vim").Render(m.vim.View())
		right := paneStyle(rightW, m.height, "shell").Render(m.shell.View())
		return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	case LayoutVimAI:
		leftW := m.width / 2
		rightW := m.width - leftW
		left := paneStyle(leftW, m.height, "vim").Render(m.vim.View())
		right := paneStyle(rightW, m.height, "ai").Render(m.ai.View())
		return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	default:
		return paneStyle(m.width, m.height, "vim").Render(m.vim.View())
	}
}

func paneStyle(w, h int, label string) lipgloss.Style {
	color := "39"
	switch label {
	case "shell":
		color = "76"
	case "ai":
		color = "213"
	}
	// Width/Height set the inner block dims (lipgloss adds padding + border on
	// top). MaxWidth/MaxHeight enforce a hard cap: even if a child renders too
	// much content, the pane never grows beyond (w, h). Without the cap, an
	// over-tall child View pushed the whole scene below the terminal viewport
	// and the top border scrolled away.
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(color)).
		Padding(0, 1).
		Width(w - 2).
		Height(h - 2).
		MaxWidth(w).
		MaxHeight(h)
}

func (m Model) renderHelp() string {
	body := strings.Join([]string{
		"sleeper — keys",
		"",
		"  Tab     cycle layout (vim / shell / split)",
		"  n       next fake file",
		"  Space   pause/resume animation",
		"  e       open current file in $EDITOR (handover)",
		"  ?       toggle this help",
		"  q       quit",
		"  Ctrl+C  hard quit",
		"  Esc Esc PANIC — switch to shell scene then quit",
	}, "\n")
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		Padding(1, 2).
		Render(body)
}

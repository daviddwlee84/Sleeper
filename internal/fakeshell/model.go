package fakeshell

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TickMsg drives the per-pane animation.
type TickMsg time.Time

// outputMsg carries deferred command output back to the model.
type outputMsg struct {
	label  string
	output string
	err    error
}

// fake "running test suite..." style lines we sprinkle to add believability
// without ever touching disk or processes.
var fakeLines = []string{
	"PASS  internal/foo  (cached)",
	"PASS  internal/bar  0.42s",
	"linting...   ✓ no issues",
	"running 142 tests... 142 passed",
	"compiling... done in 0.81s",
	"$ make build  # nothing to do",
	"checking imports... ok",
	"INFO  reload: applied 0 changes",
}

type Model struct {
	vp      viewport.Model
	cwd     string
	rng     *rand.Rand
	width   int
	height  int
	paused  bool
	history []string
	prompt  string
}

// New returns a fakeshell tied to cwd.
func New(cwd string, seed uint64) Model {
	vp := viewport.New(0, 0)
	vp.MouseWheelEnabled = false
	return Model{
		vp:     vp,
		cwd:    cwd,
		rng:    rand.New(rand.NewPCG(seed|3, seed|4)),
		prompt: shellPrompt(cwd),
	}
}

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.vp.Width = w
	if h > 0 {
		m.vp.Height = h
	}
}

func (m *Model) SetPaused(p bool) { m.paused = p }

func (m Model) Init() tea.Cmd {
	return tick(800 * time.Millisecond)
}

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return TickMsg(t) })
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return m, nil
	case TickMsg:
		if m.paused {
			return m, tick(700 * time.Millisecond)
		}
		return m, m.nextStep()
	case outputMsg:
		m.appendBlock(msg.label, msg.output, msg.err)
		// schedule next step after a brief settle
		return m, tick(time.Duration(900+m.rng.IntN(1800)) * time.Millisecond)
	}
	return m, nil
}

// nextStep decides whether to print a fake line or run a real safe command.
func (m *Model) nextStep() tea.Cmd {
	if m.rng.IntN(100) < 35 {
		// fake line — instant
		line := fakeLines[m.rng.IntN(len(fakeLines))]
		m.appendLine(line)
		return tick(time.Duration(500+m.rng.IntN(900)) * time.Millisecond)
	}
	// real command — async so we don't block the TUI
	c := Allowlist[m.rng.IntN(len(Allowlist))]
	cwd := m.cwd
	return func() tea.Msg {
		out, err := Run(cwd, c)
		return outputMsg{label: c.Label, output: out, err: err}
	}
}

func (m *Model) appendLine(s string) {
	m.history = append(m.history, s)
	m.refreshViewport()
}

func (m *Model) appendBlock(label, out string, err error) {
	header := lipgloss.NewStyle().Foreground(lipgloss.Color("76")).Render(m.prompt) +
		" " + label
	m.history = append(m.history, header)
	body := strings.TrimRight(out, "\n")
	if body != "" {
		m.history = append(m.history, body)
	}
	if err != nil {
		m.history = append(m.history, lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Render(fmt.Sprintf("(exit: %v)", err)))
	}
	m.refreshViewport()
}

func (m *Model) refreshViewport() {
	// keep last ~80 lines to bound viewport work and memory
	const maxHistory = 80
	if len(m.history) > maxHistory {
		m.history = m.history[len(m.history)-maxHistory:]
	}
	m.vp.SetContent(strings.Join(m.history, "\n"))
	m.vp.GotoBottom()
}

func (m Model) View() string {
	if m.height <= 0 || m.width <= 0 {
		return ""
	}
	return m.vp.View()
}

func shellPrompt(cwd string) string {
	if cwd == "" {
		cwd = "~"
	}
	short := cwd
	if len(short) > 24 {
		short = "…" + short[len(short)-23:]
	}
	return "$"
}

// _ = rand.Rand keeps the import even before the rng path is exercised.
var _ = rand.NewPCG

package fakeai

import (
	"math/rand/v2"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/daviddwlee84/sleeper/internal/scanner"
)

// TickMsg drives the AI pane's typing animation.
type TickMsg time.Time

// SwitchTopicMsg is sent in (e.g. by the scene when fakevim moves on) to
// trigger a fresh conversation.
type SwitchTopicMsg struct {
	File scanner.File
}

type Model struct {
	scan   *scanner.Scanner
	convs  []rawConv
	style  string // "debug" | "feature" | "refactor" | "mixed"
	rng    *rand.Rand
	width  int
	height int
	paused bool

	current Conversation
	step    int    // index of currently-being-typed step
	typed   string // characters of current step displayed so far

	finished []ChatStep // already-completed steps
	vp       viewport.Model

	// timing: between steps we wait "thinking" time
	pausedTill time.Time
}

// New constructs the AI pane. Returns an error if the embedded templates
// can't be parsed (programmer error).
func New(s *scanner.Scanner, style string, seed uint64) (Model, error) {
	convs, err := LoadAll()
	if err != nil {
		return Model{}, err
	}
	vp := viewport.New(0, 0)
	vp.MouseWheelEnabled = false
	m := Model{
		scan:  s,
		convs: convs,
		style: style,
		rng:   rand.New(rand.NewPCG(seed|5, seed|6)),
		vp:    vp,
	}
	m.startConversation(s.Pick())
	return m, nil
}

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.vp.Width = w
	m.vp.Height = h
	m.refreshViewport()
}

func (m *Model) SetPaused(p bool) { m.paused = p }

func (m Model) Init() tea.Cmd {
	return tick(900 * time.Millisecond)
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
			return m, tick(800 * time.Millisecond)
		}
		next := m.advance()
		return m, tick(next)
	case SwitchTopicMsg:
		m.startConversation(msg.File)
		return m, nil
	}
	return m, nil
}

func (m *Model) startConversation(f scanner.File) {
	rc := m.pickConv()
	syms := m.scan.Symbols(f, 8)
	fn := "doSomething"
	if len(syms) > 0 {
		fn = syms[m.rng.IntN(len(syms))]
	}
	sub := Substitution{
		File: f.Rel,
		Func: fn,
		Line: 1 + m.rng.IntN(200),
	}
	conv, err := Render(rc, sub)
	if err != nil {
		// fall back to raw
		conv = Conversation{Style: rc.style}
		for _, s := range rc.steps {
			conv.Steps = append(conv.Steps, ChatStep{Role: s.role, Body: s.body})
		}
	}
	m.current = conv
	m.step = 0
	m.typed = ""
	m.finished = nil
	m.refreshViewport()
}

func (m *Model) pickConv() rawConv {
	pool := m.convs
	if m.style != "mixed" && m.style != "" {
		filtered := pool[:0:0]
		for _, c := range m.convs {
			if c.style == m.style {
				filtered = append(filtered, c)
			}
		}
		if len(filtered) > 0 {
			pool = filtered
		}
	}
	return pool[m.rng.IntN(len(pool))]
}

// advance progresses one tick of the typing animation. Returns next delay.
func (m *Model) advance() time.Duration {
	if !m.pausedTill.IsZero() && time.Now().Before(m.pausedTill) {
		return 200 * time.Millisecond
	}
	if m.step >= len(m.current.Steps) {
		// conversation done — pause then start a new one
		m.startConversation(m.scan.Pick())
		return time.Duration(2000+m.rng.IntN(2500)) * time.Millisecond
	}
	cur := m.current.Steps[m.step]
	if len(m.typed) >= len(cur.Body) {
		// finalize this step, move to next, "thinking" pause
		m.finished = append(m.finished, cur)
		m.step++
		m.typed = ""
		m.pausedTill = time.Now().Add(time.Duration(900+m.rng.IntN(2500)) * time.Millisecond)
		m.refreshViewport()
		return 250 * time.Millisecond
	}
	// type ~1-3 chars per tick to feel snappy but not instant
	chunk := 1 + m.rng.IntN(3)
	end := len(m.typed) + chunk
	if end > len(cur.Body) {
		end = len(cur.Body)
	}
	m.typed = cur.Body[:end]
	m.refreshViewport()
	return time.Duration(25+m.rng.IntN(60)) * time.Millisecond
}

func (m *Model) refreshViewport() {
	if m.width <= 0 {
		return
	}
	var b strings.Builder
	for _, s := range m.finished {
		b.WriteString(renderBubble(s.Role, s.Body, m.width))
		b.WriteByte('\n')
	}
	if m.step < len(m.current.Steps) {
		cur := m.current.Steps[m.step]
		body := m.typed
		if body == "" && cur.Role == "ai" {
			body = aiThinking
		}
		b.WriteString(renderBubble(cur.Role, body, m.width))
	}
	m.vp.SetContent(strings.TrimRight(b.String(), "\n"))
	m.vp.GotoBottom()
}

func (m Model) View() string {
	if m.height <= 0 || m.width <= 0 {
		return ""
	}
	return m.vp.View()
}

const aiThinking = "▌"

func renderBubble(role, body string, width int) string {
	style := userBubble
	tag := "you"
	if role == "ai" {
		style = aiBubble
		tag = "ai"
	}
	header := tagStyle(role).Render(" " + tag + " ")
	wrapped := wrap(body, width-2)
	return header + "\n" + style.Render(wrapped) + "\n"
}

var (
	userBubble = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			PaddingLeft(2)
	aiBubble = lipgloss.NewStyle().
			Foreground(lipgloss.Color("75")).
			PaddingLeft(2)
)

func tagStyle(role string) lipgloss.Style {
	if role == "ai" {
		return lipgloss.NewStyle().
			Background(lipgloss.Color("60")).
			Foreground(lipgloss.Color("231")).
			Bold(true)
	}
	return lipgloss.NewStyle().
		Background(lipgloss.Color("237")).
		Foreground(lipgloss.Color("252")).
		Bold(true)
}

// wrap breaks text into lines of at most width runes. Naive — splits on
// spaces, doesn't try to be clever with code blocks.
func wrap(s string, width int) string {
	if width < 8 {
		width = 8
	}
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if len(line) <= width {
			out = append(out, line)
			continue
		}
		// greedy word wrap
		words := strings.Fields(line)
		cur := ""
		for _, w := range words {
			if cur == "" {
				cur = w
				continue
			}
			if len(cur)+1+len(w) <= width {
				cur += " " + w
			} else {
				out = append(out, cur)
				cur = w
			}
		}
		if cur != "" {
			out = append(out, cur)
		}
	}
	return strings.Join(out, "\n")
}

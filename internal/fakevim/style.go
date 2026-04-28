package fakevim

import "github.com/charmbracelet/lipgloss"

var (
	gutter      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	emptyGutter = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	cursorBlock = lipgloss.NewStyle().Background(lipgloss.Color("250")).Foreground(lipgloss.Color("0"))
	cursorBar   = lipgloss.NewStyle().Background(lipgloss.Color("214")).Foreground(lipgloss.Color("0"))

	statusFile = lipgloss.NewStyle().
			Background(lipgloss.Color("238")).
			Foreground(lipgloss.Color("252"))
	statusPos = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("250"))
	statusCmd = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))
)

func statusModeStyle(s vimState) lipgloss.Style {
	switch s {
	case stateInsert:
		return lipgloss.NewStyle().
			Background(lipgloss.Color("28")).
			Foreground(lipgloss.Color("231")).
			Bold(true)
	case stateCommand:
		return lipgloss.NewStyle().
			Background(lipgloss.Color("130")).
			Foreground(lipgloss.Color("231")).
			Bold(true)
	default:
		return lipgloss.NewStyle().
			Background(lipgloss.Color("24")).
			Foreground(lipgloss.Color("231")).
			Bold(true)
	}
}

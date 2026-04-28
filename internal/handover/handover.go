package handover

import (
	"errors"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// RedrawMsg is dispatched after $EDITOR returns so the scene can repaint.
type RedrawMsg struct{ Err error }

// ExecEditor returns a tea.Cmd that suspends Bubble Tea, runs the user's
// editor on the given file in the current TTY, then sends RedrawMsg.
//
// editor is parsed shlex-lite (split on whitespace). The file path is the
// final argument. Falls back to "vi" if editor is empty.
func ExecEditor(editor, file string) tea.Cmd {
	if strings.TrimSpace(editor) == "" {
		editor = "vi"
	}
	parts := strings.Fields(editor)
	bin := parts[0]
	args := append(parts[1:], file)
	cmd := exec.Command(bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return RedrawMsg{Err: err}
	})
}

// ErrNoEditor signals there's no usable editor in the environment.
var ErrNoEditor = errors.New("no editor available; set $EDITOR or pass --editor")

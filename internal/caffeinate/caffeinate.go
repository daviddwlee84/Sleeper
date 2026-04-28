// Package caffeinate manages the OS-specific child process that keeps the
// machine awake while sleeper is running. The name is darwin-flavoured for
// historical reasons; the implementation underneath dispatches by GOOS.
//
//   - macOS: caffeinate(8)
//   - Linux: systemd-inhibit (when logind is reachable)
//   - elsewhere: ErrUnsupported, caller falls back to animated-only mode
package caffeinate

import (
	"errors"
	"os/exec"
)

// ErrUnsupported is returned by Start when no sleep-inhibitor mechanism is
// available on the current platform (or the expected one is missing /
// non-functional, e.g. systemd-inhibit on a non-systemd Linux host or inside
// a Docker container without /run/systemd). Callers should treat this as a
// soft warning and continue without an inhibitor — sleeper still works as a
// pure animated CLI in that case.
var ErrUnsupported = errors.New("no sleep inhibitor available on this platform")

type Manager struct {
	cmd  *exec.Cmd
	pgid int
}

func (m *Manager) PID() int {
	if m == nil || m.cmd == nil || m.cmd.Process == nil {
		return 0
	}
	return m.cmd.Process.Pid
}

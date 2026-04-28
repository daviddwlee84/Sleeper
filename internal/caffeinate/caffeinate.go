package caffeinate

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
)

type Manager struct {
	cmd  *exec.Cmd
	pgid int
}

// Start spawns `caffeinate -dims` in its own process group so we can clean
// up the whole tree. -d prevents display sleep, -i prevents idle sleep,
// -m prevents disk sleep, -s prevents system sleep on AC.
//
// We deliberately omit -u: per caffeinate(8), -u defaults to a 5-second
// timeout when no -t is supplied, which would silently exit our child. The
// other four flags are assertion-based and live as long as the process.
func Start() (*Manager, error) {
	cmd := exec.Command("caffeinate", "-dims")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start caffeinate: %w", err)
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		pgid = cmd.Process.Pid
	}
	return &Manager{cmd: cmd, pgid: pgid}, nil
}

func (m *Manager) Stop() error {
	if m == nil || m.cmd == nil || m.cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-m.pgid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		_ = m.cmd.Process.Kill()
	}
	_, _ = m.cmd.Process.Wait()
	return nil
}

func (m *Manager) PID() int {
	if m == nil || m.cmd == nil || m.cmd.Process == nil {
		return 0
	}
	return m.cmd.Process.Pid
}

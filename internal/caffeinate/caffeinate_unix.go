//go:build darwin || linux

package caffeinate

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
)

// startCmd is the shared spawn helper used by every POSIX platform's Start().
// It puts the child in its own process group so Stop() can group-kill the
// whole tree (matters on Linux where systemd-inhibit forks an inner cat).
func startCmd(cmd *exec.Cmd) (*Manager, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", cmd.Path, err)
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

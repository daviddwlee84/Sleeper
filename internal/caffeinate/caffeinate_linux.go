//go:build linux

package caffeinate

import (
	"fmt"
	"os/exec"
	"time"
)

// Start spawns `systemd-inhibit ... cat` in its own process group. The
// inhibitor lock lives as long as the inner `cat` blocks on its (empty)
// stdin, so killing the process group releases the lock cleanly.
//
//   - --what=idle:sleep is enough: `sleep` short-circuits logind's suspend
//     pipeline regardless of trigger (lid, suspend key, systemctl suspend,
//     idle timer). handle-lid-switch / handle-power-key are noisier and can
//     trip polkit on some distros.
//   - inner command is `cat` (universally available, blocks forever on
//     empty stdin). `sleep infinity` is GNU-coreutils-only.
//
// Returns ErrUnsupported when systemd-inhibit is missing OR exits within
// ~150ms (the symptom of a sandbox without a reachable logind: Docker
// without /run/systemd, WSL1, broken session). The caller should treat
// that as a soft fallback to animated-CLI mode, not a fatal error.
func Start() (*Manager, error) {
	if _, err := exec.LookPath("systemd-inhibit"); err != nil {
		return nil, fmt.Errorf("%w: systemd-inhibit not on $PATH", ErrUnsupported)
	}
	cmd := exec.Command("systemd-inhibit",
		"--what=idle:sleep",
		"--who=sleeper",
		"--why=look-busy TUI",
		"--mode=block",
		"cat",
	)
	m, err := startCmd(cmd)
	if err != nil {
		return nil, err
	}
	time.Sleep(150 * time.Millisecond)
	if m.cmd.ProcessState != nil && m.cmd.ProcessState.Exited() {
		_ = m.Stop()
		return nil, fmt.Errorf("%w: systemd-inhibit exited immediately (no logind?)", ErrUnsupported)
	}
	return m, nil
}

//go:build linux

package caffeinate

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestStartStop(t *testing.T) {
	if _, err := exec.LookPath("systemd-inhibit"); err != nil {
		t.Skip("systemd-inhibit not on $PATH; skipping")
	}
	m, err := Start()
	if err != nil {
		if errors.Is(err, ErrUnsupported) {
			t.Skipf("systemd-inhibit unusable in this env: %v", err)
		}
		t.Fatalf("start: %v", err)
	}
	pid := m.PID()
	if pid == 0 {
		t.Fatal("expected non-zero pid")
	}
	if !pgrepHas(t, "systemd-inhibit", pid) {
		t.Fatalf("systemd-inhibit pid %d not found via pgrep", pid)
	}
	if err := m.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !pgrepHas(t, "systemd-inhibit", pid) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("systemd-inhibit pid %d still alive after Stop", pid)
}

func TestStopNilSafe(t *testing.T) {
	var m *Manager
	if err := m.Stop(); err != nil {
		t.Fatalf("nil Stop should be safe, got %v", err)
	}
}

func pgrepHas(t *testing.T, name string, pid int) bool {
	t.Helper()
	out, err := exec.Command("pgrep", name).Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		p, err := strconv.Atoi(strings.TrimSpace(line))
		if err == nil && p == pid {
			return true
		}
	}
	return false
}

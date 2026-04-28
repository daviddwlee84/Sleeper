package caffeinate

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestStartStop(t *testing.T) {
	m, err := Start()
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	pid := m.PID()
	if pid == 0 {
		t.Fatal("expected non-zero pid")
	}
	if !pgrepHas(t, pid) {
		t.Fatalf("caffeinate pid %d not found via pgrep", pid)
	}
	if err := m.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !pgrepHas(t, pid) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("caffeinate pid %d still alive after Stop", pid)
}

func TestStopNilSafe(t *testing.T) {
	var m *Manager
	if err := m.Stop(); err != nil {
		t.Fatalf("nil Stop should be safe, got %v", err)
	}
}

func pgrepHas(t *testing.T, pid int) bool {
	t.Helper()
	out, err := exec.Command("pgrep", "caffeinate").Output()
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

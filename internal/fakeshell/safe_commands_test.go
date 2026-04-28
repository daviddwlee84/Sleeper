package fakeshell

import (
	"strings"
	"testing"
)

func TestRun_AllowedLs(t *testing.T) {
	out, err := Run(".", SafeCmd{Bin: "ls", Args: []string{"-la"}})
	if err != nil {
		t.Fatalf("ls -la failed: %v", err)
	}
	if !strings.Contains(out, ".") {
		t.Errorf("expected ls output to contain '.', got %q", out)
	}
}

func TestRun_RejectsNotInAllowlist(t *testing.T) {
	_, err := Run(".", SafeCmd{Bin: "ls", Args: []string{"-la", "/etc"}})
	if err == nil {
		t.Fatal("expected ErrNotInAllowlist for unknown args, got nil")
	}
	if err != ErrNotInAllowlist {
		t.Fatalf("expected ErrNotInAllowlist, got %v", err)
	}
}

func TestRun_RejectsArbitraryBin(t *testing.T) {
	_, err := Run(".", SafeCmd{Bin: "rm", Args: []string{"-rf", "/"}})
	if err != ErrNotInAllowlist {
		t.Fatalf("rm -rf must be rejected, got %v", err)
	}
}

func TestAllowlist_NoShellInvocation(t *testing.T) {
	for _, c := range Allowlist {
		if c.Bin == "sh" || c.Bin == "bash" || c.Bin == "zsh" {
			t.Errorf("allowlist contains a shell: %+v", c)
		}
		for _, a := range c.Args {
			if strings.Contains(a, ";") || strings.Contains(a, "|") || strings.Contains(a, "$(") {
				t.Errorf("allowlist arg has shell metachar: %+v", c)
			}
		}
	}
}

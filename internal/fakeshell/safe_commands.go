package fakeshell

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

// SafeCmd is an immutable description of a real command we are willing to run.
// We never let user input flow into Args. We never invoke a shell.
type SafeCmd struct {
	Bin   string   // exact argv[0]
	Args  []string // fixed argv[1:]
	Label string   // what to display in the prompt line
}

// Allowlist is the only set of real commands fakeshell will execute.
var Allowlist = []SafeCmd{
	{Bin: "ls", Args: []string{"-la"}, Label: "ls -la"},
	{Bin: "git", Args: []string{"status"}, Label: "git status"},
	{Bin: "git", Args: []string{"log", "--oneline", "-20"}, Label: "git log --oneline -20"},
	{Bin: "git", Args: []string{"diff", "--stat"}, Label: "git diff --stat"},
	{Bin: "git", Args: []string{"branch", "--show-current"}, Label: "git branch --show-current"},
	{Bin: "find", Args: []string{".", "-type", "f", "-name", "*.go"}, Label: "find . -name '*.go'"},
	{Bin: "wc", Args: []string{"-l"}, Label: "wc -l ./..."},
}

// ErrNotInAllowlist is returned by Run when the command is not part of the
// allowlist. Should be impossible to trigger with the public API.
var ErrNotInAllowlist = errors.New("command not in allowlist")

// Run executes the command in cwd with a 2s timeout. Output is captured and
// returned as a single string so the caller can stream it into a viewport.
func Run(cwd string, c SafeCmd) (string, error) {
	if !inAllowlist(c) {
		return "", ErrNotInAllowlist
	}
	bin, err := exec.LookPath(c.Bin)
	if err != nil {
		return "", fmt.Errorf("lookpath %s: %w", c.Bin, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, c.Args...)
	cmd.Dir = cwd
	cmd.Env = []string{
		"PATH=" + getenv("PATH"),
		"HOME=" + getenv("HOME"),
		// Force git to skip pagers; we don't have a TTY for it
		"GIT_PAGER=cat",
		"PAGER=cat",
		"TERM=dumb",
	}
	out, err := cmd.CombinedOutput()
	return capLines(string(out), 60), err
}

// capLines truncates output to at most maxLines lines so an over-eager
// `find` or `git log` doesn't dump thousands of lines into the viewport.
func capLines(s string, maxLines int) string {
	if maxLines <= 0 {
		return s
	}
	count := 0
	for i, r := range s {
		if r == '\n' {
			count++
			if count >= maxLines {
				return s[:i] + "\n... (truncated)"
			}
		}
	}
	return s
}

// inAllowlist verifies by exact pointer-or-value identity that the SafeCmd
// matches one of our hardcoded entries. This is the last line of defense
// against accidental code paths that build a SafeCmd from untrusted strings.
func inAllowlist(c SafeCmd) bool {
	for _, ok := range Allowlist {
		if ok.Bin != c.Bin || len(ok.Args) != len(c.Args) {
			continue
		}
		match := true
		for i := range ok.Args {
			if ok.Args[i] != c.Args[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func getenv(k string) string {
	v, _ := lookupEnv(k)
	return v
}

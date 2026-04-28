# Linux / Ubuntu support for sleeper

## Context

Today `sleeper` only runs on macOS because `internal/caffeinate` shells out to
`caffeinate(8)`. The user wants Linux support, with Ubuntu as the primary
target. Acceptable fallback: if keep-awake on Linux is too messy across
desktop / init-system variants, the binary should still run as a *pure
animated CLI* on Linux (no inhibitor, just the TUI).

The plan adds a real keep-awake on systemd-based Linux (Ubuntu, Debian,
Fedora, Arch — anywhere `systemd-inhibit` is on `$PATH` and logind is reachable),
and gracefully degrades to "animated CLI only" everywhere else (non-systemd
Linux, Docker without `/run/systemd`, WSL1, Windows, BSDs).

The public API of `internal/caffeinate` (`Start`, `Stop`, `PID`) does not
change, so the rest of the codebase is untouched apart from a softer error
path in `main.go`.

## Approach

Keep the `caffeinate` package name (the rest of the code references it; rename
would be churn for no functional gain). Use Go build tags to slot in a Linux
implementation that spawns `systemd-inhibit … cat`. Treat
`systemd-inhibit` missing **or** exiting too fast (Docker / sandboxed env)
as `ErrUnsupported` — soft warning, no abort.

### Why `systemd-inhibit … cat`?

- `systemd-inhibit --what=idle:sleep --mode=block --who=sleeper --why="look-busy TUI"`
  takes a logind inhibitor lock that lasts as long as the inner command runs.
  Killing the process group kills both `systemd-inhibit` and `cat`, releasing
  the lock cleanly — same shape as the macOS `caffeinate` lifecycle.
- `--what=idle:sleep` is sufficient: `sleep` already short-circuits logind's
  suspend pipeline regardless of trigger (lid, suspend key, `systemctl suspend`,
  idle-threshold). `handle-lid-switch` / `handle-power-key` only matter for the
  handler action, not the underlying suspend, and can prompt polkit on some
  distros. Don't add them unless real-world testing shows lid-close still
  suspends.
- Inner command is `cat` (no args, inherits empty stdin → blocks on read
  forever). `sleep infinity` is GNU-coreutils-only; `cat` is universal.

### Known v1 limitation: GNOME / Ubuntu desktop screen lock

`systemd-inhibit` blocks logind's idle/sleep hook but **does not** silence
gnome-shell's own screensaver, which runs an independent idle timer via
`org.gnome.ScreenSaver` (D-Bus). On Ubuntu desktop, the screen may still blank
and lock per *Settings → Privacy → Screen Lock*.

Workaround documented for users: set "Blank Screen" to *Never*, or
`gsettings set org.gnome.desktop.screensaver lock-enabled false`.

Going after the GNOME / KDE screensaver D-Bus APIs would require either a
new dep (`godbus/dbus`) or shelling out to `gnome-session-inhibit` /
`dbus-send` per DE. Not worth the surface area for v1 — track as P3 follow-up.

## Files

### Modified

**`internal/caffeinate/caffeinate.go`** — strip the macOS-specific `Start()`
out, leave the platform-agnostic pieces:
- `Manager` struct (already POSIX-correct: pgid + cmd)
- `Stop()` (already POSIX: `syscall.Kill(-pgid, SIGTERM)` works identically on
  Linux)
- `PID()` (unchanged)
- New: `var ErrUnsupported = errors.New("no sleep inhibitor available on this platform")`
- New unexported helper `startCmd(cmd *exec.Cmd) (*Manager, error)` that does
  the `Setpgid` + `cmd.Start()` + `Getpgid` boilerplate so each platform's
  `Start()` stays a one-liner. Both darwin and linux paths use it.

**`cmd/sleeper/main.go`** — soften the error path at lines 132–138. When
`caffeinate.Start()` returns `ErrUnsupported` (wrapped via `errors.Is`),
log `[sleeper] no sleep-inhibitor available on this platform; running as animated CLI only`
to stderr and continue with `caf == nil`. All other errors still abort. The
existing `defer caf.Stop()` is already nil-safe (caffeinate.go:36). No flag
or scene change.

**`internal/caffeinate/caffeinate_test.go`** — add `//go:build darwin` build tag
at the top so `go test ./...` on Linux doesn't try to spawn the `caffeinate`
binary that doesn't exist there.

**`README.md`** — three small edits:
1. "Keeps the screen awake" bullet → mention "macOS via `caffeinate -dims`,
   Linux via `systemd-inhibit`; falls back to animated-only mode where neither
   is available".
2. Add a *Linux / Ubuntu* subsection under Install — `go install` works
   identically; no extra prereq on Ubuntu (systemd-inhibit is part of systemd).
   Mention the GNOME screen-lock caveat with the `gsettings` workaround.
3. Update the test caveat at line 134: rename to mention both platform-tagged
   tests and that each runs only on its own GOOS.

**`TODO.md`** — when shipped, add a `Done` entry; also add a P3 entry:
*"[M] GNOME/KDE screensaver inhibitor via gnome-session-inhibit / D-Bus —
covers Ubuntu desktop screen-lock gap not addressed by systemd-inhibit. Defer
until a desktop-Ubuntu user reports the issue."*

### Added

**`internal/caffeinate/caffeinate_darwin.go`** — `//go:build darwin`. Just:

```go
func Start() (*Manager, error) {
    // Existing comment about omitting -u to avoid the 5s silent exit stays.
    return startCmd(exec.Command("caffeinate", "-dims"))
}
```

**`internal/caffeinate/caffeinate_linux.go`** — `//go:build linux`:

```go
func Start() (*Manager, error) {
    if _, err := exec.LookPath("systemd-inhibit"); err != nil {
        return nil, fmt.Errorf("%w: systemd-inhibit not found on $PATH", ErrUnsupported)
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
    // Sandboxed envs (Docker without /run/systemd, WSL1, broken logind)
    // let systemd-inhibit start but it exits within milliseconds with
    // "Failed to inhibit: Connection refused". Detect this and report
    // ErrUnsupported so main.go falls through to animated-only mode.
    time.Sleep(150 * time.Millisecond)
    if m.cmd.ProcessState != nil && m.cmd.ProcessState.Exited() {
        _ = m.Stop()
        return nil, fmt.Errorf("%w: systemd-inhibit exited immediately (no logind?)", ErrUnsupported)
    }
    return m, nil
}
```

**`internal/caffeinate/caffeinate_other.go`** — `//go:build !darwin && !linux`:

```go
func Start() (*Manager, error) { return nil, ErrUnsupported }
```

Lets the package compile on FreeBSD / Windows / etc. without breaking
`GOOS=… go build ./...`.

**`internal/caffeinate/caffeinate_linux_test.go`** — `//go:build linux`. Mirror
of `caffeinate_test.go` but:
- `t.Skip` if `exec.LookPath("systemd-inhibit")` fails (keeps non-systemd CI
  green).
- `pgrep systemd-inhibit` instead of `pgrep caffeinate`.
- Same start → verify pid via pgrep → stop → verify pid gone within 2 s
  invariant. Also asserts the `cat` child is reaped (no zombie).

**`pitfalls/linux-systemd-inhibit-screensaver-gap.md`** — symptom-grep doc:
*"Symptom: on Ubuntu desktop, sleeper is running and `systemd-inhibit --list`
shows my lock, but the screen still blanks/locks after a few minutes."*
Root cause: GNOME's `org.gnome.ScreenSaver` runs an independent idle timer.
Fix: disable in Settings or via `gsettings`. Out-of-scope reason: no D-Bus
dep in v1.

## Reuse — already in the codebase

- `Manager.Stop()` in `internal/caffeinate/caffeinate.go:35-44` is already
  POSIX-portable (`syscall.Kill(-pgid, SIGTERM)` semantics are identical on
  Linux and macOS). No rewrite.
- `syscall.SysProcAttr{Setpgid: true}` works the same on both GOOS.
- `cmd/sleeper/main.go:132-143` already nil-guards `caf` — the soft-degrade
  path slots in without restructuring.
- The `--no-caffeinate` flag at `cmd/sleeper/main.go:42` already implements
  the "skip the inhibitor" UX. No rename — the flag stays for backward compat
  and reads naturally on Linux too (it's the toggle, not the literal binary).

## Verification

End-to-end checks before declaring done:

1. **Cross-compile sanity**:
   `GOOS=linux GOARCH=amd64 go build ./...` and
   `GOOS=darwin GOARCH=arm64 go build ./...` both succeed. Also
   `GOOS=windows go build ./...` succeeds (`caffeinate_other.go` covers it).

2. **macOS regression**: `go test ./internal/caffeinate -run TestStartStop -v`
   continues to pass on the dev machine. The build tag on the existing test
   file means it now only runs under `GOOS=darwin`, which is what we want.

3. **Linux happy path** (in an Ubuntu container or VM —
   `docker run --rm -it --privileged -v "$PWD:/src" -w /src golang:1.25 bash`,
   then install systemd inside, OR a real Ubuntu VM):
   - `go test ./internal/caffeinate -v` runs the new linux test. Spawns
     `systemd-inhibit`, sees it in `pgrep systemd-inhibit`, kills it, watches
     it disappear within 2 s.
   - Manual: `./sleeper --no-tui &`, then in another shell
     `systemd-inhibit --list | grep sleeper` → one entry. Ctrl+C the sleeper
     → re-run the list command → no entry. `pgrep systemd-inhibit` → empty.
   - System suspend behavior: with sleeper running, `systemctl suspend`
     should refuse / wait (logind respects the inhibitor); confirmation that
     the lock is real, not just visible.

4. **Linux degraded path** (Docker without privilege, or
   `unshare -m -- env -i sh -c './sleeper --no-tui'`):
   - sleeper logs `[sleeper] no sleep-inhibitor available on this platform; running as animated CLI only`
     to stderr.
   - The TUI still runs (or `--no-tui` blocks until SIGINT).
   - `pgrep systemd-inhibit` shows nothing.

5. **GNOME caveat manual check** (Ubuntu desktop, optional — only if a
   tester has a real Ubuntu desktop): start sleeper with the default
   GNOME screen-lock-after-5-min setting; expect screen to still lock at
   the 5-minute mark. Confirm the README workaround (`gsettings ...`
   lock-enabled false) actually keeps it awake.

6. **Memory-cap regression**: `sleeper --no-tui --debug-log /tmp/mem.log`
   on Linux for 60 s, confirm heap stays flat (the existing memory-cap
   safety net at `cmd/sleeper/main.go:73` is GOOS-independent and should
   keep working).

## Out of scope

- KDE / GNOME screensaver D-Bus inhibitors — tracked as P3 follow-up.
- Wayland-specific inhibitors (`wayland-protocols/idle-inhibit`) — same
  reason, plus needs a Wayland client lib.
- Windows native `SetThreadExecutionState` — would need `golang.org/x/sys/windows`
  and a separate `caffeinate_windows.go`; defer until a Windows user asks.
- BSDs — covered by the `caffeinate_other.go` no-op stub returning
  `ErrUnsupported`. Animated-CLI mode works.
